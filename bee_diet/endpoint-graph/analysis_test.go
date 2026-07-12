// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func beeRoot(t *testing.T) string {
	t.Helper()
	root, err := resolveBeeRoot("", resolveToolDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseRouterASTAccounting(t *testing.T) {
	root := beeRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "pkg", "api", "router.go"))
	if err != nil {
		t.Fatal(err)
	}
	eps, err := ParseRouterAST("router.go", data)
	if err != nil {
		t.Fatal(err)
	}
	var ep *Endpoint
	for i := range eps {
		if eps[i].Method == "GET" && eps[i].Path == "/accounting" {
			ep = &eps[i]
			break
		}
	}
	if ep == nil {
		t.Fatal("GET /accounting not found")
	}
	if ep.Handler != "accountingInfoHandler" {
		t.Fatalf("handler: %s", ep.Handler)
	}
	if !containsString(ep.Middlewares, "checkRouteAvailability") {
		t.Fatalf("middlewares: %v", ep.Middlewares)
	}
}

func TestGraphBypassesOmittedMiddleware(t *testing.T) {
	root := beeRoot(t)
	prog, err := loadSSA(root)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := loadServiceFields(root)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		ep   Endpoint
	}{
		{
			name: "accounting",
			ep: Endpoint{
				Mount: "mountBusinessDebug", Method: "GET", Path: "/accounting",
				Handler:     "accountingInfoHandler",
				Middlewares: []string{"checkRouteAvailability"},
			},
		},
		{
			name: "settlements",
			ep: Endpoint{
				Mount: "mountBusinessDebug", Method: "GET", Path: "/settlements",
				Handler:     "swapSettlementsHandler",
				Middlewares: []string{"checkSwapAvailability"},
			},
		},
		{
			name: "bytes-download",
			ep: Endpoint{
				Mount: "mountAPI", Method: "GET", Path: "/bytes/{address}",
				Handler: "bytesGetHandler",
				Middlewares: []string{
					"checkRouteAvailability", "contentLengthMetricMiddleware",
					"downloadSpeedMetricMiddleware", "newTracingHandler", "actDecryptionHandler",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := analyzeEndpointGraph(prog, tc.ep, fields, 0, false, true)
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range g.Nodes {
				if n.Type == NodeMiddleware {
					t.Fatalf("middleware must not appear in graph: %+v", n)
				}
			}
		})
	}
}

func TestParseRouterASTSettlementsMiddleware(t *testing.T) {
	root := beeRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "pkg", "api", "router.go"))
	if err != nil {
		t.Fatal(err)
	}
	eps, err := ParseRouterAST("router.go", data)
	if err != nil {
		t.Fatal(err)
	}
	var ep *Endpoint
	for i := range eps {
		if eps[i].Method == "GET" && eps[i].Path == "/settlements" {
			ep = &eps[i]
			break
		}
	}
	if ep == nil {
		t.Fatal("GET /settlements not found")
	}
	if !containsString(ep.Middlewares, "checkSwapAvailability") {
		t.Fatalf("expected checkSwapAvailability, got %v", ep.Middlewares)
	}
}

func TestAccountingPackageGraph(t *testing.T) {
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

	if !containsString(g.CapabilityPackages, "pkg/accounting") {
		t.Fatalf("missing accounting: %v", g.CapabilityPackages)
	}

	hasAccountingPkg := false
	for _, n := range g.Nodes {
		switch n.Type {
		case NodeCapability, NodeEntity:
			t.Fatalf("unexpected detailed node in package-compressed graph: %+v", n)
		case NodeServiceField:
			t.Fatalf("unexpected service field in package-compressed graph: %+v", n)
		case NodeImplementation, NodeBackend:
			t.Fatalf("unexpected wiring node in package-compressed graph: %+v", n)
		case NodePackage:
			if n.Package == "pkg/accounting" {
				hasAccountingPkg = true
			}
		}
	}
	if !hasAccountingPkg {
		t.Fatal("missing pkg/accounting package node")
	}
}

func TestAccountingCapabilityGraph(t *testing.T) {
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
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, false, false)
	if err != nil {
		t.Fatal(err)
	}

	if !containsString(g.CapabilityPackages, "pkg/accounting") {
		t.Fatalf("missing accounting: %v", g.CapabilityPackages)
	}
	if !containsString(g.CapabilityPackages, "pkg/swarm") {
		t.Fatalf("missing swarm: %v", g.CapabilityPackages)
	}
	if !containsString(g.CapabilityPackages, "pkg/bigint") {
		t.Fatalf("missing bigint: %v", g.CapabilityPackages)
	}
	if !containsString(g.CapabilityPackages, "pkg/jsonhttp") {
		t.Fatalf("missing jsonhttp: %v", g.CapabilityPackages)
	}

	hasCap := false
	hasField := false
	hasWiring := false
	hasBackend := false
	hasSwarmPkg := false
	for _, n := range g.Nodes {
		switch n.Type {
		case NodeCapability, NodeEntity:
			if strings.Contains(n.Label, "pkg/accounting.Interface") || strings.Contains(n.Interface, "accounting.Interface") {
				hasCap = true
			}
			if strings.Contains(n.Label, "pkg/storage.StateStorer") || strings.Contains(n.Interface, "storage.StateStorer") {
				hasCap = true
			}
		case NodeServiceField:
			if n.Field == "accounting" {
				hasField = true
			}
		case NodeImplementation:
			if n.Package == "pkg/statestore/storeadapter" || n.Package == "pkg/storage/cache" {
				hasWiring = true
			}
		case NodeBackend:
			if strings.Contains(n.Label, "disk:statestore") {
				hasBackend = true
			}
		case NodePackage:
			if n.Package == "pkg/swarm" {
				hasSwarmPkg = true
			}
		}
	}
	if !hasCap {
		t.Error("missing capability nodes")
	}
	if !hasField {
		t.Error("missing service field s.accounting")
	}
	if !hasWiring {
		t.Error("missing wiring layer nodes")
	}
	if !hasBackend {
		t.Error("missing backend disk:statestore")
	}
	if !hasSwarmPkg {
		t.Error("missing pkg/swarm package node")
	}

	for _, n := range g.Nodes {
		if strings.Contains(n.Label, beeModule) {
			t.Errorf("node label contains full module path: %s", n.Label)
		}
		if strings.Contains(n.Interface, beeModule) {
			t.Errorf("node interface contains full module path: %s", n.Interface)
		}
		if strings.Contains(n.ID, beeModule) {
			t.Errorf("node id contains full module path: %s", n.ID)
		}
		if strings.HasPrefix(n.Label, "log.Logger") || strings.HasPrefix(n.Interface, "log.Logger") {
			t.Errorf("log.Logger capability in graph: %+v", n)
		}
		if strings.HasPrefix(n.Label, "net/http") || strings.HasPrefix(n.Interface, "net/http") {
			t.Errorf("stdlib in graph: %+v", n)
		}
		if n.Type == NodeServiceField && (n.Field == "logger" || n.Field == "loggerV1") {
			t.Errorf("infra service field in graph: %+v", n)
		}
	}
	for _, n := range g.Nodes {
		if n.Type == NodePackage && n.Metadata != nil && n.Metadata["caller"] == "" {
			t.Errorf("package node missing caller metadata: %+v", n)
		}
	}

	for _, e := range g.Edges {
		if strings.Contains(e.From, "storeadapter") && strings.Contains(e.To, "leveldbstore") && e.Kind == "resolves" {
			t.Errorf("false wiring edge storeadapter -> leveldbstore: %+v", e)
		}
		if strings.Contains(e.From, "StateStorer.Get") && strings.Contains(e.To, "storeadapter") && e.Kind == "calls" {
			t.Errorf("redundant calls edge when resolves already links capability to storeadapter: %+v", e)
		}
	}
}

func TestAccountingDirectCallsGraph(t *testing.T) {
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
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range g.Nodes {
		if n.Type == NodePackage {
			t.Fatalf("package node in direct-calls mode: %+v", n)
		}
	}

	hasDirect := false
	for _, e := range g.Edges {
		if strings.Contains(e.From, "PeerAccounting") && strings.Contains(e.To, "StateStorer.Get") {
			hasDirect = true
		}
		if strings.Contains(e.To, "pkg/accounting") && e.Kind == "calls" {
			t.Fatalf("accounting package edge in direct mode: %+v", e)
		}
	}
	if !hasDirect {
		t.Fatal("missing direct edge PeerAccounting -> StateStorer.Get")
	}
}

func TestGranteeCompressedPackages(t *testing.T) {
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
		Mount: "mountAPI", Method: "POST", Path: "/grantee",
		Handler: "actCreateGranteesHandler",
	}
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, false, true)
	if err != nil {
		t.Fatal(err)
	}

	caller := handlerCallRef("actCreateGranteesHandler")
	wantID := scopedNodeID(caller, "pkg", "pkg/api")
	apiPackages := 0
	for _, n := range g.Nodes {
		if n.Type == NodeCapability || n.Type == NodeEntity {
			t.Fatalf("detailed capability/entity node should be compressed: %+v", n)
		}
		if n.Type == NodePackage && n.ID == wantID {
			apiPackages++
		}
	}
	if apiPackages != 1 {
		t.Fatalf("want 1 pkg/api node for handler, got %d", apiPackages)
	}

	wantPkgs := []string{"pkg/accesscontrol", "pkg/postage"}
	for _, pkg := range wantPkgs {
		id := scopedNodeID(caller, "pkg", pkg)
		found := false
		for _, n := range g.Nodes {
			if n.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing compressed package node %s", pkg)
		}
	}
}

func TestGranteeDirectCallsKeepsMethods(t *testing.T) {
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
		Mount: "mountAPI", Method: "POST", Path: "/grantee",
		Handler: "actCreateGranteesHandler",
	}
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}

	storerMethods := 0
	for _, n := range g.Nodes {
		if n.Type == NodeEntity {
			t.Fatalf("entity node in direct-calls mode: %+v", n)
		}
		if n.Type == NodeCapability && strings.Contains(n.Interface, "api.Storer") {
			storerMethods++
		}
	}
	if storerMethods < 2 {
		t.Fatalf("direct-calls should keep per-method Storer nodes, got %d", storerMethods)
	}
}

func TestWiringChainLinear(t *testing.T) {
	entry, ok := newWiringIndex().lookup(beeModule + "/pkg/storage.StateStorer")
	if !ok {
		t.Fatal("no wiring for StateStorer")
	}
	want := []string{"pkg/statestore/storeadapter", "pkg/storage/cache", "pkg/storage/leveldbstore"}
	if len(entry.Stack) != len(want) {
		t.Fatalf("stack: %v", entry.Stack)
	}
	for i := range want {
		if entry.Stack[i] != want[i] {
			t.Fatalf("stack[%d]=%s want %s", i, entry.Stack[i], want[i])
		}
	}
}
