package utils

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

func QueryParamsToLower(queryParams url.Values) url.Values {
	lowercaseParams := url.Values{}

	for key, values := range queryParams {
		lowercaseKey := strings.ToLower(key)
		lowercaseParams[lowercaseKey] = values
	}

	return lowercaseParams
}

func GenerateBasicAuthHeader(username, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

func CopyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func DelHopHeaders(header http.Header) {
	// Hop-by-hop headers. These are removed when sent to the backend.
	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
	var hopHeaders = []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te", // canonicalized version of "TE"
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
		"Access-Control-Allow-Origin",
	}

	for _, h := range hopHeaders {
		header.Del(h)
	}
}

func EnvSubst(input string) string {
	re := regexp.MustCompile(`\${([^}]+)}`)

	result := re.ReplaceAllStringFunc(input, func(match string) string {
		varName := match[2 : len(match)-1]
		if value, exists := os.LookupEnv(varName); exists {
			return value
		}

		return ""
	})

	return result
}
