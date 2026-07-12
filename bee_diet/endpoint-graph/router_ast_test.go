// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func loadRouterEndpoints(t *testing.T) []Endpoint {
	t.Helper()
	root := beeRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "pkg", "api", "router.go"))
	if err != nil {
		t.Fatal(err)
	}
	eps, err := ParseRouterAST("router.go", data)
	if err != nil {
		t.Fatal(err)
	}
	return eps
}

func findEndpoint(t *testing.T, eps []Endpoint, method, path string) Endpoint {
	t.Helper()
	for _, ep := range eps {
		if ep.Method == method && ep.Path == path {
			return ep
		}
	}
	t.Fatalf("endpoint not found: %s %s", method, path)
	return Endpoint{}
}

func TestParseRouterHandlerCoverage(t *testing.T) {
	t.Parallel()

	eps := loadRouterEndpoints(t)
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{"POST", "/bytes", "bytesUploadHandler"},
		{"GET", "/bytes/{address}", "bytesGetHandler"},
		{"HEAD", "/bytes/{address}", "bytesHeadHandler"},
		{"POST", "/chunks", "chunkUploadHandler"},
		{"GET", "/chunks/{address}", "chunkGetHandler"},
		{"POST", "/bzz", "bzzUploadHandler"},
		{"GET", "/bzz/{address}/{path:.*}", "bzzDownloadHandler"},
		{"GET", "/health", "healthHandler"},
		{"GET", "/readiness", "readinessHandler"},
		{"GET", "/debugstore", "debugStorage"},
		{"GET", "/loggers", "loggerGetHandler"},
		{"POST", "/chequebook/deposit", "chequebookDepositHandler"},
		{"GET", "/status", "statusGetHandler"},
		{"POST", "/welcome-message", "setWelcomeMessageHandler"},
		{"PATCH", "/tags/{id}", "doneSplitHandler"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			ep := findEndpoint(t, eps, tc.method, tc.path)
			if ep.Handler != tc.want {
				t.Fatalf("handler=%q want %q", ep.Handler, tc.want)
			}
		})
	}
}

func TestParseRouterExternalHandlers(t *testing.T) {
	t.Parallel()

	eps := loadRouterEndpoints(t)
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{"GET", "/debug/pprof/cmdline", "pprof.Cmdline"},
		{"GET", "/debug/fgprof", "fgprof.Handler"},
		{"GET", "/debug/vars", "expvar.Handler"},
		{"GET", "/metrics", "promhttp"},
		{"GET", "/debug/pprof/*", "pprof.Index"},
		{"GET", "/bzz/{address}", "anonymous"},
		{"GET", "/", "anonymous"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			ep := findEndpoint(t, eps, tc.method, tc.path)
			if ep.Handler != tc.want {
				t.Fatalf("handler=%q want %q", ep.Handler, tc.want)
			}
			if tc.want == "anonymous" {
				return
			}
			if !ep.External {
				t.Fatalf("expected external=true for %q", ep.Handler)
			}
		})
	}
}

func TestRouterMissingHandlers(t *testing.T) {
	t.Parallel()

	eps := loadRouterEndpoints(t)
	missing := 0
	for _, ep := range eps {
		if ep.Handler == "" || ep.Handler == "unknown" {
			missing++
			t.Errorf("missing handler: %s %s (%q)", ep.Method, ep.Path, ep.Handler)
		}
	}
	if missing > 0 {
		t.Fatalf("%d endpoints still missing handler names", missing)
	}
}

func TestExternalHandlerHasNoGraphDeps(t *testing.T) {
	root := beeRoot(t)
	prog, err := loadSSA(root)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := loadServiceFields(root)
	if err != nil {
		t.Fatal(err)
	}

	ep := Endpoint{
		Mount: "mountTechnicalDebug", Method: "GET", Path: "/debug/pprof/cmdline",
		Handler: "pprof.Cmdline", External: true,
	}
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, false, true)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range g.Nodes {
		if n.Type == NodeExternalDeps || n.Type == NodeMiddleware {
			t.Fatalf("external endpoint should not include %s node: %+v", n.Type, n)
		}
	}
	if g.EntityProjection != nil && len(g.EntityProjection.Nodes) > 1 {
		t.Fatalf("external endpoint should not build subsystem entity projection: %+v", g.EntityProjection)
	}
}

func TestNoExternalDepsWhenBeeSubsystemsUsed(t *testing.T) {
	root := beeRoot(t)
	prog, err := loadSSA(root)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := loadServiceFields(root)
	if err != nil {
		t.Fatal(err)
	}

	ep := Endpoint{
		Mount: "mountBusinessDebug", Method: "GET", Path: "/accounting",
		Handler:     "accountingInfoHandler",
		Middlewares: []string{"checkRouteAvailability"},
	}
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, false, true)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range g.Nodes {
		if n.Type == NodeExternalDeps {
			t.Fatalf("accounting should not expose external deps node: %+v", n)
		}
	}
}
