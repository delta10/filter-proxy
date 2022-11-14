package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/MicahParks/keyfunc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"github.com/itchyny/gojq"
	"github.com/ory/oathkeeper/helper"

	"github.com/delta10/filter-proxy/internal/config"
	"github.com/delta10/filter-proxy/internal/route"
)

type ClaimsWithGroups struct {
	Groups []string
	jwt.StandardClaims
}

func main() {
	config, err := config.NewConfig("config.yaml")
	if err != nil {
		log.Fatalln(err)
	}

	router := mux.NewRouter()
	for _, path := range config.Paths {
		currentPath := path
		router.HandleFunc(currentPath.Path, func(w http.ResponseWriter, r *http.Request) {
			authorized, err := authorizeRequest(config.JwksUrl, currentPath.Authorization.Groups, r)
			if !authorized {
				writeError(w, http.StatusUnauthorized, fmt.Sprintf("could not authorize request: %s", err.Error()))
				return
			}

			routeRegexp, _ := route.NewRouteRegexp(currentPath.Backend.URL, route.RegexpTypePath, route.RouteRegexpOptions{})

			requestUrl, err := routeRegexp.URL(mux.Vars(r))
			if err != nil {
				log.Fatalln(err)
			}

			backendURL, err := url.Parse(requestUrl)
			if err != nil {
				log.Fatalln(err)
			}

			request := &http.Request{
				Method: "GET",
				URL:    backendURL,
				Header: map[string][]string{
					"X-Api-Key": {os.Getenv("API_KEY")},
				},
			}

			client := &http.Client{
				Timeout: 5 * time.Second,
			}

			proxyResp, err := client.Do(request)
			if err != nil {
				http.Error(w, "Server Error", http.StatusInternalServerError)
				log.Fatal("ServeHTTP:", err)
			}
			defer proxyResp.Body.Close()

			if proxyResp.StatusCode != http.StatusOK {
				copyHeader(w.Header(), proxyResp.Header)
				w.WriteHeader(proxyResp.StatusCode)
				io.Copy(w, proxyResp.Body)
				return
			}

			body, _ := io.ReadAll(proxyResp.Body)

			var result map[string]interface{}
			json.Unmarshal(body, &result)

			if currentPath.Filter == "" {
				response, err := json.MarshalIndent(result, "", "    ")
				if err != nil {
					log.Fatalln(err)
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write(response)
				return
			}

			query, err := gojq.Parse(currentPath.Filter)
			if err != nil {
				log.Fatalln(err)
			}

			iter := query.Run(result)
			for {
				v, ok := iter.Next()
				if !ok {
					break
				}
				if err, ok := v.(error); ok {
					log.Fatalln(err)
				}

				response, err := json.MarshalIndent(v, "", "    ")
				if err != nil {
					log.Fatalln(err)
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write(response)
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

	log.Fatal(s.ListenAndServe())
}

func authorizeRequest(jwksUrl string, authorizedGroups []string, r *http.Request) (bool, error) {
	// Create the JWKS from the resource at the given URL.
	jwks, err := keyfunc.Get(jwksUrl, keyfunc.Options{})
	if err != nil {
		return false, err
	}

	tokenFromRequest := helper.DefaultBearerTokenFromRequest(r)
	if tokenFromRequest == "" {
		return false, errors.New("could not fetch bearer token from request")
	}

	parsedToken, err := jwt.ParseWithClaims(tokenFromRequest, &ClaimsWithGroups{}, jwks.Keyfunc)
	if err != nil {
		return false, err
	}

	if _, ok := parsedToken.Method.(*jwt.SigningMethodRSA); !ok {
		return false, fmt.Errorf("unexpected signing method: %v", parsedToken.Header["alg"])
	}

	if !parsedToken.Valid {
		return false, errors.New("parsed token is not valid")
	}

	if userClaims, ok := parsedToken.Claims.(*ClaimsWithGroups); ok {
		for _, authorizedGroup := range authorizedGroups {
			for _, userGroup := range userClaims.Groups {
				if authorizedGroup == userGroup {
					return true, nil
				}
			}
		}
	}

	return false, errors.New("user is not in required groups")
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

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
