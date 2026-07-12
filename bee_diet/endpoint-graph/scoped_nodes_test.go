// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestScopedNodeIDDistinctByCaller(t *testing.T) {
	t.Parallel()

	a := scopedNodeID(handlerCallRef("accountingInfoHandler"), "pkg", "pkg/api")
	b := scopedNodeID(handlerCallRef("bzzUploadHandler"), "pkg", "pkg/api")
	if a == b {
		t.Fatalf("same id for different callers: %q", a)
	}
	if got := scopedLabel("pkg/api", handlerCallRef("accountingInfoHandler")); got != "pkg/api ← pkg/api.accountingInfoHandler" {
		t.Fatalf("label: %q", got)
	}
	if got := capabilityTarget("accounting.Interface", "PeerAccounting"); got != "pkg/accounting.Interface.PeerAccounting" {
		t.Fatalf("capability target: %q", got)
	}
	if got := handlerCallRef("topologyHandler"); got != "pkg/api.topologyHandler" {
		t.Fatalf("handler ref: %q", got)
	}
}

func TestOmittedGraphMiddleware(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"checkRouteAvailability",
		"checkSwapAvailability",
		"checkChequebookAvailability",
		"checkChainAvailability",
		"checkStorageIncentivesAvailability",
		"contentLengthMetricMiddleware",
		"downloadSpeedMetricMiddleware",
		"newTracingHandler",
		"httpaccess.NewHTTPAccessSuppressLogHandler",
		"jsonhttp.NewMaxBodyBytesHandler",
	} {
		if !isOmittedGraphMiddleware(name) {
			t.Fatalf("%s should be omitted", name)
		}
	}
	for _, name := range []string{
		"actDecryptionHandler",
		"stakingAccessHandler",
		"postageAccessHandler",
		"statusAccessHandler",
	} {
		if isOmittedGraphMiddleware(name) {
			t.Fatalf("%s should remain in graph", name)
		}
	}
}
