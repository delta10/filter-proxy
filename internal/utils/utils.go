package utils

import (
	"encoding/base64"
	"github.com/delta10/filter-proxy/internal/wfs"
	"net"
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

func QueryParamsContainMultipleKeys(queryParams url.Values) bool {
	params := map[string]bool{}

	for key := range queryParams {
		lowercaseKey := strings.ToLower(key)
		if params[lowercaseKey] {
			return true
		}

		params[lowercaseKey] = true
	}

	return false
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

func ReadUserIP(r *http.Request) string {
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}

	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func GetTransactionMetadata(t wfs.Transaction) (string, int) {
	var (
		lastLayerName string
		count         int
	)

	if inserts := t.Inserts; len(inserts) == 1 {
		lastLayerName, count = inserts[0].Layers[0].XMLName.Space+":"+inserts[0].Layers[0].XMLName.Local, count+1
	}
	if updates := t.Updates; len(updates) == 1 {
		lastLayerName, count = updates[0].TypeName, count+1
	}
	if deletes := t.Deletes; len(deletes) == 1 {
		lastLayerName, count = deletes[0].TypeName, count+1
	}

	return lastLayerName, count
}
