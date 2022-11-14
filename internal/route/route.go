// Copyright 2012 The Gorilla Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package route

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

type RouteRegexpOptions struct {
	StrictSlash    bool
	UseEncodedPath bool
}

type RegexpType int

const (
	RegexpTypePath RegexpType = iota
	RegexpTypeHost
	RegexpTypePrefix
	RegexpTypeQuery
)

// newRouteRegexp parses a route template and returns a routeRegexp,
// used to match a host, a path or a query string.
//
// It will extract named variables, assemble a regexp to be matched, create
// a "reverse" template to build URLs and compile regexps to validate variable
// values used in URL building.
//
// Previously we accepted only Python-like identifiers for variable
// names ([a-zA-Z_][a-zA-Z0-9_]*), but currently the only restriction is that
// name and pattern can't be empty, and names can't contain a colon.
func NewRouteRegexp(tpl string, typ RegexpType, options RouteRegexpOptions) (*RouteRegexp, error) {
	// Check if it is well-formed.
	idxs, errBraces := braceIndices(tpl)
	if errBraces != nil {
		return nil, errBraces
	}
	// Backup the original.
	template := tpl
	// Now let's parse it.
	defaultPattern := "[^/]+"
	if typ == RegexpTypeQuery {
		defaultPattern = ".*"
	} else if typ == RegexpTypeHost {
		defaultPattern = "[^.]+"
	}
	// Only match strict slash if not matching
	if typ != RegexpTypePath {
		options.StrictSlash = false
	}
	// Set a flag for StrictSlash.
	endSlash := false
	if options.StrictSlash && strings.HasSuffix(tpl, "/") {
		tpl = tpl[:len(tpl)-1]
		endSlash = true
	}
	VarsN := make([]string, len(idxs)/2)
	varsR := make([]*regexp.Regexp, len(idxs)/2)
	pattern := bytes.NewBufferString("")
	pattern.WriteByte('^')
	reverse := bytes.NewBufferString("")
	var end int
	var err error
	for i := 0; i < len(idxs); i += 2 {
		// Set all values we are interested in.
		raw := tpl[end:idxs[i]]
		end = idxs[i+1]
		parts := strings.SplitN(tpl[idxs[i]+1:end-1], ":", 2)
		name := parts[0]
		patt := defaultPattern
		if len(parts) == 2 {
			patt = parts[1]
		}
		// Name or pattern can't be empty.
		if name == "" || patt == "" {
			return nil, fmt.Errorf("mux: missing name or pattern in %q",
				tpl[idxs[i]:end])
		}
		// Build the regexp pattern.
		fmt.Fprintf(pattern, "%s(?P<%s>%s)", regexp.QuoteMeta(raw), varGroupName(i/2), patt)

		// Build the reverse template.
		fmt.Fprintf(reverse, "%s%%s", raw)

		// Append variable name and compiled pattern.
		VarsN[i/2] = name
		varsR[i/2], err = regexp.Compile(fmt.Sprintf("^%s$", patt))
		if err != nil {
			return nil, err
		}
	}
	// Add the remaining.
	raw := tpl[end:]
	pattern.WriteString(regexp.QuoteMeta(raw))
	if options.StrictSlash {
		pattern.WriteString("[/]?")
	}
	if typ == RegexpTypeQuery {
		// Add the default pattern if the query value is empty
		if queryVal := strings.SplitN(template, "=", 2)[1]; queryVal == "" {
			pattern.WriteString(defaultPattern)
		}
	}
	if typ != RegexpTypePrefix {
		pattern.WriteByte('$')
	}

	var wildcardHostPort bool
	if typ == RegexpTypeHost {
		if !strings.Contains(pattern.String(), ":") {
			wildcardHostPort = true
		}
	}
	reverse.WriteString(raw)
	if endSlash {
		reverse.WriteByte('/')
	}
	// Compile full regexp.
	reg, errCompile := regexp.Compile(pattern.String())
	if errCompile != nil {
		return nil, errCompile
	}

	// Check for capturing groups which used to work in older versions
	if reg.NumSubexp() != len(idxs)/2 {
		panic(fmt.Sprintf("route %s contains capture groups in its regexp. ", template) +
			"Only non-capturing groups are accepted: e.g. (?:pattern) instead of (pattern)")
	}

	// Done!
	return &RouteRegexp{
		Template:         template,
		RegexpType:       typ,
		Options:          options,
		Regexp:           reg,
		Reverse:          reverse.String(),
		VarsN:            VarsN,
		VarsR:            varsR,
		WildcardHostPort: wildcardHostPort,
	}, nil
}

// routeRegexp stores a regexp to match a host or path and information to
// collect and validate route variables.
type RouteRegexp struct {
	// The unmodified template.
	Template string
	// The type of match
	RegexpType RegexpType
	// Options for matching
	Options RouteRegexpOptions
	// Expanded regexp.
	Regexp *regexp.Regexp
	// Reverse template.
	Reverse string
	// Variable names.
	VarsN []string
	// Variable regexps (validators).
	VarsR []*regexp.Regexp
	// Wildcard host-port (no strict port match in hostname)
	WildcardHostPort bool
}

// Match matches the regexp against the URL host or path.
func (r *RouteRegexp) Match(req *http.Request, match *mux.RouteMatch) bool {
	if r.RegexpType == RegexpTypeHost {
		host := getHost(req)
		if r.WildcardHostPort {
			// Don't be strict on the port match
			if i := strings.Index(host, ":"); i != -1 {
				host = host[:i]
			}
		}
		return r.Regexp.MatchString(host)
	}

	if r.RegexpType == RegexpTypeQuery {
		return r.matchQueryString(req)
	}
	path := req.URL.Path
	if r.Options.UseEncodedPath {
		path = req.URL.EscapedPath()
	}
	return r.Regexp.MatchString(path)
}

// url builds a URL part using the given values.
func (r *RouteRegexp) URL(values map[string]string) (string, error) {
	urlValues := make([]interface{}, len(r.VarsN))
	for k, v := range r.VarsN {
		value, ok := values[v]
		if !ok {
			return "", fmt.Errorf("mux: missing route variable %q", v)
		}
		if r.RegexpType == RegexpTypeQuery {
			value = url.QueryEscape(value)
		}
		urlValues[k] = value
	}
	rv := fmt.Sprintf(r.Reverse, urlValues...)
	if !r.Regexp.MatchString(rv) {
		// The URL is checked against the full regexp, instead of checking
		// individual variables. This is faster but to provide a good error
		// message, we check individual regexps if the URL doesn't match.
		for k, v := range r.VarsN {
			if !r.VarsR[k].MatchString(values[v]) {
				return "", fmt.Errorf(
					"mux: variable %q doesn't match, expected %q", values[v],
					r.VarsR[k].String())
			}
		}
	}
	return rv, nil
}

// getURLQuery returns a single query parameter from a request URL.
// For a URL with foo=bar&baz=ding, we return only the relevant key
// value pair for the routeRegexp.
func (r *RouteRegexp) getURLQuery(req *http.Request) string {
	if r.RegexpType != RegexpTypeQuery {
		return ""
	}
	templateKey := strings.SplitN(r.Template, "=", 2)[0]
	val, ok := findFirstQueryKey(req.URL.RawQuery, templateKey)
	if ok {
		return templateKey + "=" + val
	}
	return ""
}

// findFirstQueryKey returns the same result as (*url.URL).Query()[key][0].
// If key was not found, empty string and false is returned.
func findFirstQueryKey(rawQuery, key string) (value string, ok bool) {
	query := []byte(rawQuery)
	for len(query) > 0 {
		foundKey := query
		if i := bytes.IndexAny(foundKey, "&;"); i >= 0 {
			foundKey, query = foundKey[:i], foundKey[i+1:]
		} else {
			query = query[:0]
		}
		if len(foundKey) == 0 {
			continue
		}
		var value []byte
		if i := bytes.IndexByte(foundKey, '='); i >= 0 {
			foundKey, value = foundKey[:i], foundKey[i+1:]
		}
		if len(foundKey) < len(key) {
			// Cannot possibly be key.
			continue
		}
		keyString, err := url.QueryUnescape(string(foundKey))
		if err != nil {
			continue
		}
		if keyString != key {
			continue
		}
		valueString, err := url.QueryUnescape(string(value))
		if err != nil {
			continue
		}
		return valueString, true
	}
	return "", false
}

func (r *RouteRegexp) matchQueryString(req *http.Request) bool {
	return r.Regexp.MatchString(r.getURLQuery(req))
}

// braceIndices returns the first level curly brace indices from a string.
// It returns an error in case of unbalanced braces.
func braceIndices(s string) ([]int, error) {
	var level, idx int
	var idxs []int
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if level++; level == 1 {
				idx = i
			}
		case '}':
			if level--; level == 0 {
				idxs = append(idxs, idx, i+1)
			} else if level < 0 {
				return nil, fmt.Errorf("mux: unbalanced braces in %q", s)
			}
		}
	}
	if level != 0 {
		return nil, fmt.Errorf("mux: unbalanced braces in %q", s)
	}
	return idxs, nil
}

// varGroupName builds a capturing group name for the indexed variable.
func varGroupName(idx int) string {
	return "v" + strconv.Itoa(idx)
}

// getHost tries its best to return the request host.
// According to section 14.23 of RFC 2616 the Host header
// can include the port number if the default value of 80 is not used.
func getHost(r *http.Request) string {
	if r.URL.IsAbs() {
		return r.URL.Host
	}
	return r.Host
}
