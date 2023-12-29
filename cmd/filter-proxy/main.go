package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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

	"github.com/delta10/filter-proxy/internal/config"
	"github.com/delta10/filter-proxy/internal/route"
	"github.com/delta10/filter-proxy/internal/utils"
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
		router.HandleFunc(path.Path, func(w http.ResponseWriter, r *http.Request) {
			backend, ok := config.Backends[path.Backend.Slug]
			if !ok {
				writeError(w, http.StatusBadRequest, "could not find backend associated with this path: "+path.Backend.Slug)
				return
			}

			utils.DelHopHeaders(r.Header)

			var filterParams map[string]interface{}
			if path.RequestRewrite != "" {
				body, _ := io.ReadAll(r.Body)

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

					filterParams = v.(map[string]interface{})
				}
			}

			authorizationStatusCode, authorizationResponse := authorizeRequestWithService(config, backend, path, r, filterParams)
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

			backendRequest, err := http.NewRequest(r.Method, fullBackendURL.String(), nil)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not construct backend request")
				return
			}

			if len(filterParams) > 0 {
				backendRequestBody, err := json.MarshalIndent(filterParams, "", "    ")
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not marshal json")
					return
				}

				buffer := bytes.NewBuffer(backendRequestBody)
				backendRequest.Header.Set("Content-Type", "application/json")
				backendRequest.Body = io.NopCloser(buffer)
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

			client := &http.Client{
				Timeout:   25 * time.Second,
				Transport: transport,
			}

			proxyResp, err := client.Do(backendRequest)
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not fetch backend response: %s", err))
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
					w.Write(response)
				}
			} else {
				utils.DelHopHeaders(proxyResp.Header)
				utils.CopyHeader(w.Header(), proxyResp.Header)
				w.WriteHeader(proxyResp.StatusCode)
				io.Copy(w, proxyResp.Body)
			}
		})
	}

	s := &http.Server{
		Addr:           config.ListenAddress,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	if config.ListenTLS.Certificate != "" && config.ListenTLS.Key != "" {
		log.Fatal(s.ListenAndServeTLS(config.ListenTLS.Certificate, config.ListenTLS.Key))
	} else {
		log.Fatal(s.ListenAndServe())
	}
}

func authorizeRequestWithService(config *config.Config, backend config.Backend, path config.Path, r *http.Request, filterParams map[string]interface{}) (int, *AuthorizationResponse) {
	if path.AllowAlways {
		return http.StatusOK, nil
	}

	if config.AuthorizationServiceURL == "" {
		log.Print("returned unauthenticated as there is no authorization service URL configured")
		return http.StatusInternalServerError, nil
	}

	if utils.QueryParamsContainMultipleKeys(r.URL.Query()) {
		log.Print("rejected request as query parameters contain multiple keys")
		return http.StatusBadRequest, nil
	}

	authorizationBody := map[string]interface{}{
		"source":     path.Backend.Slug,
		"user_agent": r.Header.Get("User-Agent"),
		"ip":         utils.ReadUserIP(r),
	}

	if backend.Type == "OWS" {
		queryParams := utils.QueryParamsToLower(r.URL.Query())
		authorizationBody["service"] = queryParams.Get("service")
		authorizationBody["request"] = queryParams.Get("request")

		if authorizationBody["service"] == "WMS" {
			authorizationBody["resource"] = queryParams.Get("layers") + queryParams.Get("layer")
			authorizationBody["params"] = map[string]interface{}{
				"service":    queryParams.Get("service"),
				"request":    queryParams.Get("request"),
				"cql_filter": queryParams.Get("cql_filter"),
			}
		} else if authorizationBody["service"] == "WFS" {
			authorizationBody["resource"] = queryParams.Get("typename") + queryParams.Get("typenames")
			authorizationBody["params"] = map[string]interface{}{
				"service":    queryParams.Get("service"),
				"request":    queryParams.Get("request"),
				"cql_filter": queryParams.Get("cql_filter"),
			}
		} else {
			log.Printf("unauthorized service type: %s", authorizationBody["service"])
			return http.StatusUnauthorized, nil
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
		log.Printf("unsupported backend type configured: %s")
		return http.StatusInternalServerError, nil
	}

	marshalledAuthorizationBody, err := json.Marshal(authorizationBody)
	if err != nil {
		log.Print("could not marshall authorization body")
		return http.StatusInternalServerError, nil
	}

	request, err := http.NewRequest("GET", config.AuthorizationServiceURL, bytes.NewReader(marshalledAuthorizationBody))
	if err != nil {
		log.Print("could not construct authorization request")
		return http.StatusInternalServerError, nil
	}

	if r.Header.Get("Cookie") != "" {
		request.Header.Set("Cookie", r.Header.Get("Cookie"))
	}

	if r.Header.Get("Authorization") != "" {
		request.Header.Set("Authorization", r.Header.Get("Authorization"))
	}

	request.Header.Set("X-Forwarded-For", utils.ReadUserIP(r))

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(request)
	if err != nil {
		log.Printf("could not fetch authorization response: %s", err)
		return http.StatusInternalServerError, nil
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("could not read authorization response: %s", err)
		return http.StatusInternalServerError, nil
	}

	responseData := AuthorizationResponse{}
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		log.Printf("could not unmarshal authorization response: %s", err)
		return http.StatusInternalServerError, nil
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("received an authorization error: %v, %s", resp.StatusCode, body)
	}

	return resp.StatusCode, &responseData
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
