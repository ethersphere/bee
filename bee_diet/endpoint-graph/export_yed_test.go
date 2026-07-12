// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportYedEndpointGraph(t *testing.T) {
	t.Parallel()

	g := &EndpointGraph{
		Endpoint: Endpoint{Method: "GET", Path: "/accounting", Handler: "accountingInfoHandler"},
		Nodes: []GraphNode{
			{ID: "route:GET /accounting", Type: NodeRoute, Layer: LayerRoute, Label: "GET /accounting"},
			{ID: "n:pkg/api.accountingInfoHandler:pkg:pkg/accounting", Type: NodePackage, Layer: LayerCall, Label: "pkg/accounting", Package: "pkg/accounting"},
		},
		Edges: []GraphEdge{
			{From: "route:GET /accounting", To: "n:pkg/api.accountingInfoHandler:pkg:pkg/accounting", Kind: "calls"},
		},
	}

	data, err := renderYedGraphML(yedGraphFromEndpoint(g, yedExportOptions{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{
		`xmlns="http://graphml.graphdrawing.org/xmlns"`,
		`xmlns:y="http://www.yworks.com/xml/graphml"`,
		`<y:ShapeNode>`,
		`GET /accounting`,
		`pkg/accounting`,
		`source="route_GET__accounting"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in graphml output", want)
		}
	}
}

func TestExportYedMergedGraph(t *testing.T) {
	t.Parallel()

	mg := &MergedGraph{
		Version: 1,
		Nodes: []MergedNode{
			{
				GraphNode: GraphNode{
					ID: "pkg:pkg/jsonhttp", Type: NodePackage, Layer: LayerCall,
					Label: "pkg/jsonhttp", Package: "pkg/jsonhttp",
				},
				EndpointCount: 3,
				Endpoints:     []string{"GET /a", "GET /b", "GET /c"},
			},
		},
		Edges: []MergedEdge{},
	}

	data, err := renderYedGraphML(yedGraphFromMerged(mg, yedExportOptions{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "(3 endpoints)") {
		t.Fatal("merged endpoint count missing from label")
	}
}

func TestExportYedCLIRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	in := filepath.Join(dir, "graph.json")
	out := filepath.Join(dir, "graph.graphml")

	g := EndpointGraph{
		Endpoint: Endpoint{Method: "POST", Path: "/grantee", Handler: "actCreateGranteesHandler"},
		Nodes: []GraphNode{
			{ID: "route:POST /grantee", Type: NodeRoute, Layer: LayerRoute, Label: "POST /grantee"},
		},
	}
	if err := writeJSON(in, g); err != nil {
		t.Fatal(err)
	}
	jsonData, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeYedExportFile(out, jsonData, yedExportOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestExportYedUsesGraphvizLayout(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("graphviz dot not installed")
	}

	g := EndpointGraph{
		Endpoint: Endpoint{Method: "GET", Path: "/accounting", Handler: "accountingInfoHandler"},
		Nodes: []GraphNode{
			{ID: "route:GET /accounting", Type: NodeRoute, Layer: LayerRoute, Label: "GET /accounting"},
			{ID: "handler:accountingInfoHandler", Type: NodeHandler, Layer: LayerRoute, Label: "pkg/api.accountingInfoHandler"},
		},
		Edges: []GraphEdge{
			{From: "route:GET /accounting", To: "handler:accountingInfoHandler", Kind: "wraps"},
		},
	}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	out, err := renderYedExportFromJSON(data, yedExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	routeGeom := strings.Count(s, "route_GET__accounting")
	handlerGeom := strings.Contains(s, "handler_accountingInfoHandler")
	if !handlerGeom || routeGeom == 0 {
		t.Fatalf("missing expected nodes in graphml")
	}
	var geoms []string
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "<y:Geometry") {
			geoms = append(geoms, strings.TrimSpace(line))
		}
	}
	if len(geoms) < 2 {
		t.Fatalf("expected geometry for multiple nodes, got %d", len(geoms))
	}
	if geoms[0] == geoms[1] {
		t.Fatalf("expected distinct node positions, got duplicate geometry")
	}
}
