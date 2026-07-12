// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestParseGraphvizPlainQuotedNodeName(t *testing.T) {
	t.Parallel()

	plain := `graph 1 15.787 2
node "g0" 2.844 1 1.5534 0.5 "GET /accounting" filled ellipse black lightyellow
node "g1 with space" 5.8846 1 2.3056 0.5 "handler label" filled box black lightyellow
edge "g0" "g1 with space" 4 3.6221 1 3.9165 1 4.2613 1 4.5912 1 wraps 4.1763 1.1042 solid black
stop
`
	layout, err := parseGraphvizPlain([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := layout.Nodes["g0"]; !ok {
		t.Fatal("missing g0")
	}
	if _, ok := layout.Nodes["g1 with space"]; !ok {
		t.Fatal("missing quoted node name with space")
	}
	if len(layout.Edges) != 1 {
		t.Fatalf("edges: %d", len(layout.Edges))
	}
}

func TestParseGraphvizPlainLongLabel(t *testing.T) {
	t.Parallel()

	plain := `graph 1 100 50
node g0 1 2 3 4 "label with spaces and (3 endpoints)" filled box black wheat
stop
`
	layout, err := parseGraphvizPlain([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := layout.Nodes["g0"]; !ok {
		t.Fatal("missing g0")
	}
}
