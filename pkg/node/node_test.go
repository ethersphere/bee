// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"testing"

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
