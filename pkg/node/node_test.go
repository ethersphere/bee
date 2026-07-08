// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/node"
)

func TestValidatePublicAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		addr   string
		expErr bool
	}{
		{
			name:   "empty host",
			addr:   ":1635",
			expErr: true,
		},
		{
			name:   "localhost",
			addr:   "localhost:1635",
			expErr: true,
		},
		{
			name:   "loopback",
			addr:   "127.0.0.1:1635",
			expErr: true,
		},
		{
			name:   "loopback ipv6",
			addr:   "[::1]:1635",
			expErr: true,
		},
		{
			name:   "missing port",
			addr:   "1.2.3.4",
			expErr: true,
		},
		{
			name:   "empty port",
			addr:   "1.2.3.4:",
			expErr: true,
		},
		{
			name:   "invalid port number",
			addr:   "1.2.3.4:abc",
			expErr: true,
		},
		{
			name:   "valid",
			addr:   "1.2.3.4:1635",
			expErr: false,
		},
		{
			name:   "valid ipv6",
			addr:   "[2001:db8::1]:1635",
			expErr: false,
		},
		{
			name:   "empty",
			addr:   "",
			expErr: false,
		},
		{
			name:   "valid hostname",
			addr:   "example.com:8080",
			expErr: false,
		},
		{
			name:   "valid hostname with hyphen",
			addr:   "test-example.com:8080",
			expErr: false,
		},
		{
			name:   "private IP",
			addr:   "192.168.1.1:8080",
			expErr: true,
		},
		{
			name:   "invalid hostname format",
			addr:   "invalid..hostname:8080",
			expErr: true,
		},
		{
			name:   "hostname starts with hyphen",
			addr:   "-test.com:8080",
			expErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := node.ValidatePublicAddress(tc.addr)
			if tc.expErr && err == nil {
				t.Fatal("expected error, but got none")
			}
			if !tc.expErr && err != nil {
				t.Fatalf("expected no error, but got: %v", err)
			}
		})
	}
}

func TestUseEmbeddedSnapshot(t *testing.T) {
	t.Parallel()

	const (
		mainnet = uint64(1)
		testnet = uint64(10)
	)

	testCases := []struct {
		name             string
		skip             bool
		batchStoreExists bool
		resync           bool
		networkID        uint64
		mode             api.BeeNodeMode
		want             bool
	}{
		{name: "first boot on mainnet", networkID: mainnet, mode: api.FullMode, want: true},
		{name: "existing store, no resync", batchStoreExists: true, networkID: mainnet, mode: api.FullMode, want: false},
		{name: "resync on a fresh store", resync: true, networkID: mainnet, mode: api.FullMode, want: true},
		// New behavior: resync rebuilds from the snapshot even with a store present.
		{name: "resync on an existing store uses the snapshot", batchStoreExists: true, resync: true, networkID: mainnet, mode: api.FullMode, want: true},
		{name: "resync with skip syncs from the chain", batchStoreExists: true, resync: true, skip: true, networkID: mainnet, mode: api.FullMode, want: false},
		{name: "skip on first boot", skip: true, networkID: mainnet, mode: api.FullMode, want: false},
		{name: "light node uses the snapshot", networkID: mainnet, mode: api.LightMode, want: true},
		{name: "ultra-light never uses the snapshot", resync: true, networkID: mainnet, mode: api.UltraLightMode, want: false},
		{name: "non-mainnet never uses the snapshot", resync: true, networkID: testnet, mode: api.FullMode, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := node.UseEmbeddedSnapshot(tc.skip, tc.batchStoreExists, tc.resync, tc.networkID, tc.mode)
			if got != tc.want {
				t.Fatalf("UseEmbeddedSnapshot = %v, want %v", got, tc.want)
			}
		})
	}
}
