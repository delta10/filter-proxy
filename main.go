package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/itchyny/gojq"
)

func main() {
	config, err := NewConfig("config.yaml")
	if err != nil {
		log.Fatalln(err)
	}

	router := mux.NewRouter()
	for _, path := range config.Paths {
		router.HandleFunc(path.Path, func(w http.ResponseWriter, r *http.Request) {
			backendURL, err := url.Parse(path.Backend.URL)
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

			body, _ := ioutil.ReadAll(proxyResp.Body)

			var result map[string]interface{}
			json.Unmarshal(body, &result)

			if path.Filter == "" {
				response, err := json.MarshalIndent(result, "", "    ")
				if err != nil {
					log.Fatalln(err)
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write(response)
				return
			}

			query, err := gojq.Parse(path.Filter)
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
