package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"github.com/itchyny/gojq"

	"github.com/delta10/filter-proxy/internal/config"
	"github.com/delta10/filter-proxy/internal/logs"
	"github.com/delta10/filter-proxy/internal/route"
	"github.com/delta10/filter-proxy/internal/utils"
)

type ClaimsWithGroups struct {
	jwt.RegisteredClaims
	Groups []string `json:"groups"`
}

type AuthorizationResponse struct {
	User struct {
		Id       int64
		Username string
		Name     string
	}
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

			authorizationStatusCode, authorizationResponse := authorizeRequestWithService(config, path, r)
			if authorizationStatusCode != http.StatusOK {
				writeError(w, authorizationStatusCode, "unauthorized request")
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

					request, err := json.MarshalIndent(v, "", "    ")
					if err != nil {
						writeError(w, http.StatusInternalServerError, "could not marshal json")
						return
					}

					backendRequest.Header.Set("Content-Type", "application/json")

					buffer := bytes.NewBuffer(request)
					backendRequest.Body = io.NopCloser(buffer)
				}
			}

			tlsConfig := &tls.Config{}
			if backend.Auth.TLS.RootCertificates != "" {
				rootCertificates, err := ioutil.ReadFile(backend.Auth.TLS.RootCertificates)
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

			if path.LogBackend != "" {
				logBackendName, ok := config.LogBackends[path.LogBackend]
				if !ok {
					writeError(w, http.StatusInternalServerError, "could not find log backend: "+path.LogBackend)
					return
				}

				logBackend := logs.NewLogBackend(logBackendName)

				labels := map[string]string{
					"system":  "filter-proxy",
					"backend": path.Backend.Slug,
				}

				logLine := map[string]string{
					"method":        r.Method,
					"path":          r.URL.String(),
					"status":        proxyResp.Status,
					"user_id":       fmt.Sprint(authorizationResponse.User.Id),
					"user_username": authorizationResponse.User.Username,
					"ip":            utils.ReadUserIP(r),
				}

				err := logBackend.WriteLog(labels, logLine)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not write log to backend")
					return
				}
			}

			defer proxyResp.Body.Close()

			if path.ResponseRewrite != "" && proxyResp.StatusCode == http.StatusOK {
				body, _ := io.ReadAll(proxyResp.Body)

				var result map[string]interface{}
				json.Unmarshal(body, &result)

				query, err := gojq.Parse(path.ResponseRewrite)
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

func authorizeRequestWithService(config *config.Config, path config.Path, r *http.Request) (int, *AuthorizationResponse) {
	if config.AuthorizationServiceURL == "" {
		log.Print("returned unauthenticated as there is no authorization service URL configured.")
		return http.StatusInternalServerError, nil
	}

	authorizationServiceURL, err := url.Parse(config.AuthorizationServiceURL)
	if err != nil {
		log.Printf("could not parse authorization url: %s", err)
		return http.StatusInternalServerError, nil
	}

	authorizationServiceURL.RawQuery = r.URL.RawQuery

	authorizationHeaders := r.Header

	authorizationHeaders.Set("X-Source-Slug", path.Backend.Slug)
	authorizationHeaders.Set("X-Original-Uri", r.URL.RequestURI())

	request := &http.Request{
		Method: "GET",
		URL:    authorizationServiceURL,
		Header: authorizationHeaders,
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(request)
	if err != nil {
		log.Printf("could not fetch authorization response: %s", err)
		return http.StatusInternalServerError, nil
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
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
