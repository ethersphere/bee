// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestMergeGraphsKeepsCallerScopedNodesSeparate(t *testing.T) {
	t.Parallel()

	g1 := EndpointGraph{
		Endpoint: Endpoint{Mount: "mountA", Method: "GET", Path: "/accounting", Handler: "h1"},
		Nodes: []GraphNode{
			{ID: "route:GET /accounting", Type: NodeRoute, Layer: LayerRoute, Label: "GET /accounting"},
			{ID: scopedNodeID("h1", "pkg", "pkg/api"), Type: NodePackage, Layer: LayerCall, Label: "pkg/api ← h1", Package: "pkg/api"},
			{ID: scopedNodeID("h1", "pkg", "pkg/accounting"), Type: NodePackage, Layer: LayerCall, Label: "pkg/accounting ← h1", Package: "pkg/accounting"},
		},
		Edges: []GraphEdge{
			{From: "route:GET /accounting", To: scopedNodeID("h1", "pkg", "pkg/api"), Kind: "calls"},
			{From: scopedNodeID("h1", "pkg", "pkg/api"), To: scopedNodeID("h1", "pkg", "pkg/accounting"), Kind: "calls", Branch: "success"},
		},
	}
	g2 := EndpointGraph{
		Endpoint: Endpoint{Mount: "mountA", Method: "GET", Path: "/balances", Handler: "h2"},
		Nodes: []GraphNode{
			{ID: "route:GET /balances", Type: NodeRoute, Layer: LayerRoute, Label: "GET /balances"},
			{ID: scopedNodeID("h2", "pkg", "pkg/api"), Type: NodePackage, Layer: LayerCall, Label: "pkg/api ← h2", Package: "pkg/api"},
			{ID: scopedNodeID("h2", "pkg", "pkg/accounting"), Type: NodePackage, Layer: LayerCall, Label: "pkg/accounting ← h2", Package: "pkg/accounting"},
		},
		Edges: []GraphEdge{
			{From: "route:GET /balances", To: scopedNodeID("h2", "pkg", "pkg/api"), Kind: "calls"},
			{From: scopedNodeID("h2", "pkg", "pkg/api"), To: scopedNodeID("h2", "pkg", "pkg/accounting"), Kind: "calls", Branch: "success"},
		},
	}

	merged, err := mergeGraphs("/tmp/graphs", []loadedEndpointGraph{
		{dir: "GET_accounting", graph: g1},
		{dir: "GET_balances", graph: g2},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Stats.NodeCount != 6 {
		t.Fatalf("nodes: %d, want 6 caller-scoped nodes", merged.Stats.NodeCount)
	}

	apiH1 := scopedNodeID("h1", "pkg", "pkg/api")
	apiH2 := scopedNodeID("h2", "pkg", "pkg/api")
	if apiH1 == apiH2 {
		t.Fatal("pkg/api nodes must differ by caller handler")
	}

	for _, id := range []string{apiH1, apiH2} {
		var found *MergedNode
		for i := range merged.Nodes {
			if merged.Nodes[i].ID == id {
				found = &merged.Nodes[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("missing node %s", id)
		}
		if found.EndpointCount != 1 {
			t.Fatalf("node %s endpoint count: %d", id, found.EndpointCount)
		}
	}
}

func TestMergeGraphsCollapsesPackageNodes(t *testing.T) {
	t.Parallel()

	makeGraph := func(method, path, handler string) EndpointGraph {
		caller := handlerCallRef(handler)
		apiID := scopedNodeID(caller, "pkg", "pkg/api")
		acctID := scopedNodeID(caller, "pkg", "pkg/accounting")
		return EndpointGraph{
			CompressPackages: true,
			Endpoint:         Endpoint{Mount: "mountA", Method: method, Path: path, Handler: handler},
			Nodes: []GraphNode{
				{ID: "route:" + method + " " + path, Type: NodeRoute, Layer: LayerRoute, Label: method + " " + path},
				{ID: apiID, Type: NodePackage, Layer: LayerCall, Label: "pkg/api ← " + caller, Package: "pkg/api"},
				{ID: acctID, Type: NodePackage, Layer: LayerCall, Label: "pkg/accounting ← " + caller, Package: "pkg/accounting"},
			},
			Edges: []GraphEdge{
				{From: "route:" + method + " " + path, To: apiID, Kind: "calls"},
				{From: apiID, To: acctID, Kind: "invokes", Branch: "success"},
			},
		}
	}

	merged, err := mergeGraphs("/tmp/graphs", []loadedEndpointGraph{
		{dir: "GET_accounting", graph: makeGraph("GET", "/accounting", "accountingInfoHandler")},
		{dir: "GET_balances", graph: makeGraph("GET", "/balances", "balancesHandler")},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !merged.CompressPackages {
		t.Fatal("expected compress_packages on merged graph")
	}
	if merged.Stats.NodeCount != 4 {
		t.Fatalf("nodes: %d, want 4 collapsed package nodes + 2 routes", merged.Stats.NodeCount)
	}

	for _, pkg := range []string{"pkg/api", "pkg/accounting"} {
		id := mergedPackageNodeID(pkg)
		var found *MergedNode
		for i := range merged.Nodes {
			if merged.Nodes[i].ID == id {
				found = &merged.Nodes[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("missing collapsed node %s", id)
		}
		if found.EndpointCount != 2 {
			t.Fatalf("node %s endpoint count: %d, want 2", id, found.EndpointCount)
		}
		if found.Label != pkg {
			t.Fatalf("node %s label: %q, want %q", id, found.Label, pkg)
		}
	}
}

func TestFilterGraphsByMount(t *testing.T) {
	t.Parallel()

	graphs := []loadedEndpointGraph{
		{dir: "GET_bytes", graph: EndpointGraph{Endpoint: Endpoint{Mount: "mountAPI", Method: "POST", Path: "/bytes"}}},
		{dir: "GET_accounting", graph: EndpointGraph{Endpoint: Endpoint{Mount: "mountBusinessDebug", Method: "GET", Path: "/accounting"}}},
		{dir: "GET_health", graph: EndpointGraph{Endpoint: Endpoint{Mount: "mountTechnicalDebug", Method: "GET", Path: "/health"}}},
	}

	api := filterGraphsByMount(graphs, []string{"mountAPI"})
	if len(api) != 1 || api[0].dir != "GET_bytes" {
		t.Fatalf("mountAPI filter: %+v", api)
	}

	two := filterGraphsByMount(graphs, []string{"mountAPI,mountBusinessDebug"})
	if len(two) != 2 {
		t.Fatalf("two-mount filter: %d", len(two))
	}

	all := filterGraphsByMount(graphs, nil)
	if len(all) != 3 {
		t.Fatalf("no filter: %d", len(all))
	}
}

func TestMergedPackageSummary(t *testing.T) {
	t.Parallel()

	merged, err := mergeGraphs("/tmp", []loadedEndpointGraph{
		{dir: "GET_accounting", graph: EndpointGraph{
			Endpoint: Endpoint{Method: "GET", Path: "/accounting", Handler: "h1"},
			Nodes: []GraphNode{
				{ID: scopedNodeID("h1", "pkg", "pkg/accounting"), Type: NodePackage, Package: "pkg/accounting", Label: "pkg/accounting ← h1", Layer: LayerCall},
			},
		}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Packages) != 1 || merged.Packages[0].Package != "pkg/accounting" {
		t.Fatalf("packages: %+v", merged.Packages)
	}
}
