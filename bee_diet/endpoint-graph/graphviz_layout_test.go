// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestParseGraphvizPlain(t *testing.T) {
	t.Parallel()

	plain := `graph 1 15.787 2
node route_GET__accounting 2.844 1 1.5534 0.5 "GET /accounting" filled ellipse black lightyellow
node handler_accountingInfoHandler 5.8846 1 2.3056 0.5 "handler" filled box black lightyellow
edge route_GET__accounting handler_accountingInfoHandler 4 3.6221 1 3.9165 1 4.2613 1 4.5912 1 wraps 4.1763 1.1042 solid black
stop
`
	layout, err := parseGraphvizPlain([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if layout.WidthInches != 15.787 || layout.HeightInches != 2 {
		t.Fatalf("graph size: %+v", layout)
	}
	loc, ok := layout.Nodes["route_GET__accounting"]
	if !ok {
		t.Fatal("missing route node layout")
	}
	x, y, w, h := loc.yedGeometry(layout.HeightInches)
	if w <= 0 || h <= 0 {
		t.Fatalf("bad node size: %v %v", w, h)
	}
	if x == 0 && y == 0 {
		t.Fatalf("expected non-zero position, got x=%v y=%v", x, y)
	}
}
