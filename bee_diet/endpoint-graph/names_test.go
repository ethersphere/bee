// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestEndpointDirName(t *testing.T) {
	t.Parallel()

	ep := Endpoint{Method: "GET", Path: "/accounting"}
	if got := EndpointDirName(ep); got != "GET_accounting" {
		t.Fatalf("EndpointDirName: %q", got)
	}
	ep = Endpoint{Method: "POST", Path: "/bytes/{address}"}
	if got := EndpointDirName(ep); got != "POST_bytes_address" {
		t.Fatalf("EndpointDirName param: %q", got)
	}
}

func TestShortBeeNames(t *testing.T) {
	t.Parallel()

	if got := shortBeePkg("github.com/ethersphere/bee/v2/pkg/accounting"); got != "pkg/accounting" {
		t.Fatalf("shortBeePkg: %q", got)
	}
	if got := shortBeeType("github.com/ethersphere/bee/v2/pkg/accounting.Interface"); got != "accounting.Interface" {
		t.Fatalf("shortBeeType: %q", got)
	}
	if got := shortBeeSymbol("github.com/ethersphere/bee/v2/pkg/node.InitStateStore"); got != "pkg/node.InitStateStore" {
		t.Fatalf("shortBeeSymbol: %q", got)
	}
}
