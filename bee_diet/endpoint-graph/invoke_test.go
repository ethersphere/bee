// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
)

func TestFnVisitedPerScope(t *testing.T) {
	g := &graphBuilder{fnVisited: map[string]struct{}{}, callScope: "scope-a"}
	keyA := "pkg/api.helper\x00scope-a"
	g.fnVisited[keyA] = struct{}{}

	g.callScope = "scope-b"
	keyB := "pkg/api.helper\x00scope-b"
	if _, seen := g.fnVisited[keyB]; seen {
		t.Fatal("fnVisited must be scoped per callScope")
	}
	if _, seen := g.fnVisited[keyA]; !seen {
		t.Fatal("original scoped visit must remain")
	}
}

func TestInvokeResolutionAmbiguous(t *testing.T) {
	res := invokeResolution{ambiguous: true, implCount: 9}
	if !res.ambiguous || res.implCount != 9 || len(res.impls) != 0 {
		t.Fatalf("unexpected resolution: %+v", res)
	}
}
