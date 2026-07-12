// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEntityProjectionAccounting(t *testing.T) {
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
		Handler: "accountingInfoHandler",
	}
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if g.EntityProjection == nil {
		t.Fatal("expected entity_projection")
	}

	hasRuntime := false
	hasAccounting := false
	for _, n := range g.EntityProjection.Nodes {
		switch n.ID {
		case "entity:runtime:api":
			hasRuntime = true
		case "entity:field:accounting":
			hasAccounting = true
		}
	}
	if !hasRuntime {
		t.Fatal("missing entity:runtime:api")
	}
	if !hasAccounting {
		t.Fatal("missing entity:field:accounting")
	}

	foundDep := false
	for _, e := range g.EntityProjection.Edges {
		if e.From == "entity:runtime:api" && e.To == "entity:field:accounting" && e.Kind == entityKindDependsOn {
			foundDep = true
		}
	}
	if !foundDep {
		t.Fatalf("expected depends_on edge api→accounting, got %+v", g.EntityProjection.Edges)
	}

	for _, n := range g.Nodes {
		if n.Type == NodeMiddleware {
			t.Fatalf("middleware must not appear in raw graph: %+v", n)
		}
	}
}

func TestEntityProjectionSchemaCompat(t *testing.T) {
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
		Handler: "accountingInfoHandler",
	}
	g, err := analyzeEndpointGraph(prog, ep, fields, 0, false, true)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EndpointGraph
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.EntityProjection == nil {
		t.Fatal("entity_projection lost in JSON round-trip")
	}

	var legacy struct {
		Endpoint Endpoint    `json:"endpoint"`
		Nodes    []GraphNode `json:"nodes"`
		Edges    []GraphEdge `json:"edges"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Nodes) == 0 {
		t.Fatal("legacy nodes missing")
	}
}

func TestLoadServiceFieldsIncludesEnvelopeStorage(t *testing.T) {
	root := beeRoot(t)
	fields, err := loadServiceFields(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["envelopeStorage"]; !ok {
		t.Fatal("expected envelopeStorage in service fields")
	}
}

func TestWriteGraphFilesNoDotFile(t *testing.T) {
	dir := t.TempDir()
	g := &EndpointGraph{
		Endpoint: Endpoint{Method: "GET", Path: "/x", Handler: "healthHandler"},
		Nodes: []GraphNode{{
			ID: "route:GET /x", Type: NodeRoute, Layer: LayerRoute, Label: "GET /x",
		}},
	}
	if _, err := writeGraphFiles(dir, "graph", g, graphWriteOpts{SVG: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "graph.dot")); err == nil {
		t.Fatal("graph.dot must not be written")
	}
}
