// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestIsBeeCapabilityInterface(t *testing.T) {
	t.Parallel()

	cases := []struct {
		iface string
		want  bool
	}{
		{beeModule + "/pkg/accounting.Interface", true},
		{beeModule + "/pkg/storage.StateStorer", true},
		{beeModule + "/pkg/log.Logger", false},
		{beeModule + "/pkg/jsonhttp.MethodHandler", false},
		{"net/http.ResponseWriter", false},
		{"github.com/prometheus/client_golang/prometheus.Counter", false},
		{"fmt.Stringer", false},
	}
	for _, c := range cases {
		if got := isSubsystemCapabilityInterface(c.iface); got != c.want {
			t.Errorf("isSubsystemCapabilityInterface(%q) = %v, want %v", c.iface, got, c.want)
		}
	}
}

func TestIsCapabilityField(t *testing.T) {
	t.Parallel()

	if !isCapabilityField("accounting") {
		t.Error("accounting should be capability field")
	}
	if isCapabilityField("logger") {
		t.Error("logger should not be capability field")
	}
}

func TestIsBeeProjectInterface(t *testing.T) {
	t.Parallel()

	if !isBeeProjectInterface(beeModule + "/pkg/bigint.BigInt") {
		t.Error("bigint is bee project type")
	}
	if isBeeProjectInterface("net/http.Request") {
		t.Error("net/http is not bee project")
	}
}
