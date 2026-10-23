// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"
)

const openAPISpecPath = "../../openapi/Swarm.yaml"

// undocumentedRoute reports whether a route is intentionally left out of the
// OpenAPI spec: the index and robots pages, metrics and profiling endpoints.
func undocumentedRoute(path string) bool {
	switch path {
	case "/", "/robots.txt", "/metrics":
		return true
	}
	return strings.HasPrefix(path, "/debug/")
}

// pathVariable matches a mux or OpenAPI path variable, with an optional mux
// regular expression, such as {address} or {path:.*}.
var pathVariable = regexp.MustCompile(`\{[^}:]+(:[^}]*)?\}`)

// normalizePath drops path variable names so that mux and OpenAPI templates of
// the same route compare equal, for example /bzz/{address}/{path:.*} and
// /bzz/{address}/{path}.
func normalizePath(path string) string {
	return pathVariable.ReplaceAllString(path, "{}")
}

// TestRouterMatchesOpenAPISpec checks that every route served by the API is
// documented in the OpenAPI spec with the same HTTP methods, and that the spec
// documents no route that the API does not serve.
func TestRouterMatchesOpenAPISpec(t *testing.T) {
	t.Parallel()

	specOps := openAPIOperations(t)

	_, _, _, _, s := newTestServer(t, testServerOptions{})
	routerOps := routerOperations(t, s)

	for path, methods := range routerOps {
		specMethods, ok := specOps[path]
		if !ok {
			t.Errorf("route %s is not documented in %s", path, openAPISpecPath)
			continue
		}
		// A nil method list means the handler serves any method, so the spec
		// only has to document at least one.
		if methods != nil && !slices.Equal(methods, specMethods) {
			t.Errorf("route %s serves methods %v, but the spec documents %v", path, methods, specMethods)
		}
	}

	for path := range specOps {
		if _, ok := routerOps[path]; !ok {
			t.Errorf("path %s is documented in %s, but no route serves it", path, openAPISpecPath)
		}
	}
}

// openAPIOperations returns the documented HTTP methods of every path in the
// spec, keyed by normalized path, sorted and in upper case.
func openAPIOperations(t *testing.T) map[string][]string {
	t.Helper()

	data, err := os.ReadFile(openAPISpecPath)
	if err != nil {
		t.Fatal(err)
	}

	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}

	ops := make(map[string][]string, len(spec.Paths))
	for path, item := range spec.Paths {
		var methods []string
		for key := range item {
			switch method := strings.ToUpper(key); method {
			case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
				http.MethodPatch, http.MethodDelete, http.MethodOptions:
				methods = append(methods, method)
			}
		}
		slices.Sort(methods)
		ops[normalizePath(path)] = methods
	}
	return ops
}

// routerOperations returns the HTTP methods served by every documented route
// of the API router, keyed by normalized path. The methods are read from the
// Allow header of the method not allowed response to an unknown method. A nil
// method list means the handler does not restrict methods.
func routerOperations(t *testing.T, s *api.Service) map[string][]string {
	t.Helper()

	ops := make(map[string][]string)
	err := s.Router().Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, ok := documentedRoutePath(route)
		if !ok {
			return nil
		}

		req := httptest.NewRequestWithContext(t.Context(), "UNKNOWN", path, nil)
		rec := httptest.NewRecorder()
		route.GetHandler().ServeHTTP(rec, req)

		var methods []string
		if rec.Code == http.StatusMethodNotAllowed {
			methods = strings.Split(rec.Header().Get("Allow"), ", ")
			slices.Sort(methods)
		}
		ops[normalizePath(path)] = methods
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

// documentedRoutePath returns the path template of a route that must be
// documented in the spec, and false for any other route.
func documentedRoutePath(route *mux.Route) (string, bool) {
	// Subdomain routes serve the same content as /bzz and have no separate
	// documentation.
	if _, err := route.GetHostTemplate(); err == nil {
		return "", false
	}
	path, err := route.GetPathTemplate()
	if err != nil {
		return "", false
	}
	// Every API route is registered with and without the version prefix.
	if strings.HasPrefix(path, api.RootPath+"/") || undocumentedRoute(path) {
		return "", false
	}
	return path, true
}
