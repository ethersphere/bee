// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

func TestAccountingGraphNoStamperStore(t *testing.T) {
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
		if strings.Contains(n.Label, "stamper") || strings.Contains(n.ID, "stamper") {
			t.Errorf("stamper must not appear on GET /accounting: %+v", n)
		}
		if n.Label == "storage.Store.Get" {
			t.Errorf("storage.Store.Get must not appear on GET /accounting: %+v", n)
		}
	}
}
