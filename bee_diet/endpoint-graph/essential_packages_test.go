// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestIsEssentialPackage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pkg  string
		want bool
	}{
		{"pkg/storer", true},
		{"pkg/postage/batchstore", true},
		{"pkg/storage/leveldbstore", true},
		{"pkg/api", false},
		{"pkg/jsonhttp", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isEssentialPackage(tc.pkg); got != tc.want {
			t.Fatalf("isEssentialPackage(%q)=%v want %v", tc.pkg, got, tc.want)
		}
	}
}

func TestYedEssentialFillColor(t *testing.T) {
	t.Parallel()

	n := GraphNode{Layer: LayerCall, Package: "pkg/accounting"}
	if got := yedFillColorForNode(n, true); got != "#FF6666" {
		t.Fatalf("essential color=%q", got)
	}
	if got := yedFillColorForNode(n, false); got == "#FF6666" {
		t.Fatal("essential color disabled")
	}
}
