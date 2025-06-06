package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"github.com/itchyny/gojq"
	"github.com/rs/cors"

	"github.com/delta10/filter-proxy/internal/config"
	"github.com/delta10/filter-proxy/internal/route"
	"github.com/delta10/filter-proxy/internal/utils"
	"github.com/delta10/filter-proxy/internal/wfs"
)

type ClaimsWithGroups struct {
	jwt.RegisteredClaims
	Groups []string `json:"groups"`
}

type AuthorizationResponse struct {
	Result         bool   `json:"result"`
	ResponseFilter string `json:"response_filter"`
}

func main() {
	config, err := config.NewConfig("config.yaml")
	if err != nil {
		log.Fatalln(err)
	}

	router := mux.NewRouter()
	for _, configuredPath := range config.Paths {
		path := configuredPath

		if path.Passthrough {
			router.PathPrefix(path.Path).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				client := &http.Client{}

				//http: Request.RequestURI can't be set in client requests.
				//http://golang.org/src/pkg/net/http/client.go
				r.RequestURI = ""

				backend, ok := config.Backends[path.Backend.Slug]
				if !ok {
					writeError(w, http.StatusBadRequest, "could not find backend associated with this path: "+path.Backend.Slug)
					return
				}

				backendBaseUrl, err := url.Parse(backend.BaseURL)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not parse backend URL")
					return
				}

				r.URL.Host = backendBaseUrl.Host
				r.URL.Scheme = backendBaseUrl.Scheme

				for headerKey, headerValue := range backend.Auth.Header {
					parsedHeaderValue := utils.EnvSubst(headerValue)
					r.Header.Set(headerKey, parsedHeaderValue)
				}

				utils.DelHopHeaders(r.Header)
				addForwardedForHeaders(r, r)

				client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				}

				resp, err := client.Do(r)
				if err != nil {
					writeError(w, http.StatusBadGateway, fmt.Sprintf("could not fetch backend response: %s", err))
					return
				}

				defer resp.Body.Close()

				utils.DelHopHeaders(resp.Header)
				utils.CopyHeader(w.Header(), resp.Header)
				w.WriteHeader(resp.StatusCode)
				io.Copy(w, resp.Body)
			})
		} else {
			router.HandleFunc(path.Path, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)

				backend, ok := config.Backends[path.Backend.Slug]
				if !ok {
					writeError(w, http.StatusBadRequest, "could not find backend associated with this path: "+path.Backend.Slug)
					return
				}

				utils.DelHopHeaders(r.Header)

				var bodyFilterParams map[string]interface{}
				if path.RequestRewrite != "" {
					var result map[string]interface{}
					json.Unmarshal(body, &result)

					query, err := gojq.Parse(path.RequestRewrite)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not parse filter")
						return
					}

					iter := query.Run(result)
					for {
						v, ok := iter.Next()
						if !ok {
							break
						}

						if _, ok := v.(error); ok {
							continue
						}

						bodyFilterParams = v.(map[string]interface{})
					}
				}

				authorizationStatusCode, authorizationResponse, isTransaction := authorizeRequestWithService(config, backend, path, r, bodyFilterParams, body)
				if authorizationStatusCode != http.StatusOK {
					writeError(w, authorizationStatusCode, "unauthorized request")
					return
				}

				if !authorizationResponse.Result {
					writeError(w, http.StatusUnauthorized, "result field is not true")
					return
				}

				allowedMethods := path.AllowedMethods
				if len(allowedMethods) == 0 {
					allowedMethods = []string{"GET"}
				}

				if !utils.StringInSlice(r.Method, allowedMethods) {
					writeError(w, http.StatusBadRequest, "request method is not allowed")
					return
				}

				routeRegexp, _ := route.NewRouteRegexp(path.Backend.Path, route.RegexpTypePath, route.RouteRegexpOptions{})

				parsedRequestPath, err := routeRegexp.URL(mux.Vars(r))
				if err != nil {
					writeError(w, http.StatusBadRequest, "could not parse request URL")
					return
				}

				backendBaseUrl, err := url.Parse(backend.BaseURL)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not parse backend URL")
					return
				}

				fullBackendURL := backendBaseUrl.JoinPath(parsedRequestPath)

				// Copy query parameters to backend
				fullBackendURL.RawQuery = r.URL.Query().Encode()

				var backendRequest *http.Request
				if len(bodyFilterParams) > 0 {
					backendRequestBody, err := json.MarshalIndent(bodyFilterParams, "", "    ")
					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not marshal json")
						return
					}

					backendRequest, err = http.NewRequest(r.Method, fullBackendURL.String(), bytes.NewReader(backendRequestBody))
					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not construct backend request")
						return
					}

					backendRequest.Header.Set("Content-Type", "application/json")
				} else {
					requestBody := io.Reader(nil)

					if isTransaction {
						var transactionBody wfs.Transaction

						err := xml.Unmarshal(body, &transactionBody)
						if len(body) > 0 && err != nil {
							writeError(w, http.StatusBadRequest, "Error validating transaction body while constructing backend request")
							return
						}

						marshaledBody, err := xml.Marshal(transactionBody)
						if err != nil {
							writeError(w, http.StatusInternalServerError, "Error processing transaction body")
							return
						}

						requestBody = bytes.NewReader(marshaledBody)
					}

					backendRequest, err = http.NewRequest(r.Method, fullBackendURL.String(), requestBody)

					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not construct backend request")
						return
					}
				}

				tlsConfig := &tls.Config{}
				if backend.Auth.TLS.RootCertificates != "" {
					rootCertificates, err := os.ReadFile(backend.Auth.TLS.RootCertificates)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not retrieve root certs for backend")
						return
					}

					roots := x509.NewCertPool()
					ok := roots.AppendCertsFromPEM(rootCertificates)
					if !ok {
						writeError(w, http.StatusInternalServerError, "could not load root certs for backend")
						return
					}

					tlsConfig.RootCAs = roots
				}

				if backend.Auth.TLS.Certificate != "" && backend.Auth.TLS.Key != "" {
					cert, err := tls.LoadX509KeyPair(backend.Auth.TLS.Certificate, backend.Auth.TLS.Key)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not load TLS keypair for backend")
						return
					}

					tlsConfig.Certificates = []tls.Certificate{cert}
				}

				transport := &http.Transport{TLSClientConfig: tlsConfig}

				if backend.Auth.Basic.Username != "" && backend.Auth.Basic.Password != "" {
					parsedPassword := utils.EnvSubst(backend.Auth.Basic.Password)
					backendRequest.SetBasicAuth(backend.Auth.Basic.Username, parsedPassword)
				}

				for headerKey, headerValue := range backend.Auth.Header {
					parsedHeaderValue := utils.EnvSubst(headerValue)
					backendRequest.Header.Set(headerKey, parsedHeaderValue)
				}

				addForwardedForHeaders(backendRequest, r)

				client := &http.Client{
					Timeout:   25 * time.Second,
					Transport: transport,
				}

				proxyResp, err := client.Do(backendRequest)
				if err != nil {
					writeError(w, http.StatusBadGateway, fmt.Sprintf("could not fetch backend response: %s", err))
					return
				}

				defer proxyResp.Body.Close()

				if proxyResp.StatusCode == http.StatusOK && (path.ResponseRewrite != "" || authorizationResponse.ResponseFilter != "") {
					body, _ := io.ReadAll(proxyResp.Body)
					var result map[string]interface{}
					json.Unmarshal(body, &result)

					var responseRewrite = ""
					if authorizationResponse.ResponseFilter != "" {
						responseRewrite = authorizationResponse.ResponseFilter
					} else {
						responseRewrite = path.ResponseRewrite
					}

					query, err := gojq.Parse(responseRewrite)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not parse filter")
						return
					}

					iter := query.Run(result)
					for {
						v, ok := iter.Next()
						if !ok {
							break
						}

						if _, ok := v.(error); ok {
							continue
						}

						response, err := json.MarshalIndent(v, "", "    ")
						if err != nil {
							writeError(w, http.StatusInternalServerError, "could not marshal json")
							return
						}

						w.Header().Set("Content-Type", "application/json")
						w.Header().Set("Cache-Control", "private")
						w.Write(response)
					}
				} else {
					utils.DelHopHeaders(proxyResp.Header)
					utils.CopyHeader(w.Header(), proxyResp.Header)
					w.Header().Set("Cache-Control", "private")
					w.WriteHeader(proxyResp.StatusCode)
					io.Copy(w, proxyResp.Body)
				}
			})
		}
	}

	// By default allow only https://filter-proxy.local
	corsOptions := cors.Options{
		AllowedOrigins: []string{
			"https://filter-proxy.local",
		},
		Debug:              config.Cors.DebugLogging,
		OptionsPassthrough: false,
	}

	if len(config.Cors.AllowedOrigins) > 0 {
		corsOptions.AllowedOrigins = config.Cors.AllowedOrigins
	}

	if len(config.Cors.AllowedMethods) > 0 {
		corsOptions.AllowedMethods = config.Cors.AllowedMethods
	}

	if len(config.Cors.AllowedHeaders) > 0 {
		corsOptions.AllowedHeaders = config.Cors.AllowedHeaders
	}

	if config.Cors.AllowCredentials {
		corsOptions.AllowCredentials = config.Cors.AllowCredentials
	}

	if config.Cors.AllowPrivateNetwork {
		corsOptions.AllowPrivateNetwork = config.Cors.AllowPrivateNetwork
	}

	c := cors.New(corsOptions)

	handler := c.Handler(router)

	s := &http.Server{
		Addr:           config.ListenAddress,
		Handler:        requestLoggingMiddleware(handler),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	log.Printf("listening on %v", config.ListenAddress)
	if config.ListenTLS.Certificate != "" && config.ListenTLS.Key != "" {
		log.Fatal(s.ListenAndServeTLS(config.ListenTLS.Certificate, config.ListenTLS.Key))
	} else {
		log.Fatal(s.ListenAndServe())
	}
}

func authorizeRequestWithService(config *config.Config, backend config.Backend, path config.Path, r *http.Request, filterParams map[string]interface{}, body []byte) (int, *AuthorizationResponse, bool) {
	if config.AuthorizationServiceURL == "" {
		log.Print("returned unauthenticated as there is no authorization service URL configured")
		return http.StatusInternalServerError, nil, false
	}

	if utils.QueryParamsContainMultipleKeys(r.URL.Query()) {
		log.Print("rejected request as query parameters contain multiple keys")
		return http.StatusBadRequest, nil, false
	}

	authorizationBody := map[string]interface{}{
		"source":     path.Backend.Slug,
		"user_agent": r.Header.Get("User-Agent"),
		"ip":         utils.ReadUserIP(r),
	}

	isTransactionSet := false

	if backend.Type == "OWS" {
		queryParams := utils.QueryParamsToLower(r.URL.Query())
		var transaction wfs.Transaction

		requestParam := queryParams.Get("request")
		serviceParam := queryParams.Get("service")

		if len(body) > 0 && len(queryParams) > 0 {
			log.Printf("Invalid request: cannot have both XML body and query parameters")
			return http.StatusBadRequest, nil, false
		}

		err := xml.Unmarshal(body, &transaction)
		transactionSet := transaction.XMLName.Local != ""
		isTransactionSet = transactionSet

		if len(body) > 0 && err != nil {
			log.Printf("Invalid XML in request body: %v", err)
			return http.StatusBadRequest, nil, false
		}

		if transactionSet {
			authorizationBody["service"] = "WFS"
		} else {
			authorizationBody["service"] = serviceParam
		}

		authorizationBody["request"] = requestParam

		if authorizationBody["service"] == "WMS" {
			authorizationBody["resource"] = queryParams.Get("layers") + queryParams.Get("layer")
			authorizationBody["params"] = map[string]interface{}{
				"service":    serviceParam,
				"request":    requestParam,
				"cql_filter": queryParams.Get("cql_filter"),
			}
		} else if authorizationBody["service"] == "WFS" {
			if transactionSet {
				layerName, transactionCount := utils.GetTransactionMetadata(transaction)

				if transactionCount > 1 {
					log.Printf("we only allow one wfs transaction at a time")
					return http.StatusBadRequest, nil, false
				}

				authorizationBody["resource"] = layerName
				authorizationBody["request"] = "Transaction"
			} else {
				authorizationBody["resource"] = queryParams.Get("typename") + queryParams.Get("typenames")
				authorizationBody["params"] = map[string]interface{}{
					"service":    serviceParam,
					"request":    requestParam,
					"cql_filter": queryParams.Get("cql_filter"),
				}
			}
		} else {
			log.Printf("unauthorized service type: %s", authorizationBody["service"])
			return http.StatusUnauthorized, nil, false
		}
	} else if backend.Type == "WMTS" {
		queryParams := utils.QueryParamsToLower(r.URL.Query())
		authorizationBody["service"] = queryParams.Get("service")
		authorizationBody["request"] = queryParams.Get("request")
		authorizationBody["resource"] = queryParams.Get("layer")
		authorizationBody["params"] = map[string]interface{}{
			"service": queryParams.Get("service"),
			"request": queryParams.Get("request"),
		}
	} else if backend.Type == "REST" {
		authorizationBody["resource"] = path.Backend.Path

		params := make(map[string]interface{})

		for k, v := range r.URL.Query() {
			params[k] = v
		}

		if path.RequestRewrite != "" {
			for k, v := range filterParams {
				params[k] = v
			}
		}

		authorizationBody["params"] = params
	} else if backend.Type != "" {
		log.Printf("unsupported backend type configured: %s", backend.Type)
		return http.StatusInternalServerError, nil, false
	}

	marshalledAuthorizationBody, err := json.Marshal(authorizationBody)
	if err != nil {
		log.Print("could not marshall authorization body")
		return http.StatusInternalServerError, nil, false
	}

	request, err := http.NewRequest("GET", config.AuthorizationServiceURL, bytes.NewReader(marshalledAuthorizationBody))
	if err != nil {
		log.Print("could not construct authorization request")
		return http.StatusInternalServerError, nil, false
	}

	if r.Header.Get("Cookie") != "" {
		request.Header.Set("Cookie", r.Header.Get("Cookie"))
	}

	if r.Header.Get("Authorization") != "" {
		request.Header.Set("Authorization", r.Header.Get("Authorization"))
	}

	addForwardedForHeaders(request, r)

	client := &http.Client{
		Timeout: 25 * time.Second,
	}

	resp, err := client.Do(request)
	if err != nil {
		log.Printf("could not fetch authorization response: %s", err)
		return http.StatusInternalServerError, nil, false
	}

	defer resp.Body.Close()

	resBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("could not read authorization response: %s", err)
		return http.StatusInternalServerError, nil, false
	}

	responseData := AuthorizationResponse{}
	err = json.Unmarshal(resBody, &responseData)
	if err != nil {
		log.Printf("could not unmarshal authorization response: %s", err)
		return http.StatusInternalServerError, nil, false
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("received an authorization error: %v, %s", resp.StatusCode, resBody)
	}

	return resp.StatusCode, &responseData, isTransactionSet
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	resp := make(map[string]string)
	resp["message"] = message
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
	}

	w.WriteHeader(statusCode)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func addForwardedForHeaders(backendRequest *http.Request, originalRequest *http.Request) {
	backendRequest.Header.Set("X-Forwarded-Host", originalRequest.Host)
	backendRequest.Header.Set("X-Forwarded-For", utils.ReadUserIP(originalRequest))

	if originalRequest.TLS == nil {
		backendRequest.Header.Set("X-Forwarded-Proto", "http")
	} else {
		backendRequest.Header.Set("X-Forwarded-Proto", "https")
	}
}

func requestLoggingMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		log.Printf(
			"%s %s %s",
			r.Method,
			r.URL.Path,
			r.Header.Get("User-Agent"),
		)

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
