// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
)

func TestBee260BackwardCompatibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		// Versions < 2.7.0 should require backward compatibility
		{
			name:      "version 2.6.0",
			userAgent: "bee/2.6.0 go1.22.0 linux/amd64",
			want:      true,
		},
		{
			name:      "version 2.6.5",
			userAgent: "bee/2.6.5 go1.22.0 linux/amd64",
			want:      true,
		},
		{
			name:      "version 2.5.0",
			userAgent: "bee/2.5.0 go1.21.0 linux/amd64",
			want:      true,
		},
		{
			name:      "version 2.6.0-beta1",
			userAgent: "bee/2.6.0-beta1 go1.22.0 linux/amd64",
			want:      true,
		},
		// Versions >= 2.7.0 should NOT require backward compatibility
		{
			name:      "version 2.7.0",
			userAgent: "bee/2.7.0 go1.23.0 linux/amd64",
			want:      false,
		},
		{
			name:      "version 2.8.0",
			userAgent: "bee/2.8.0 go1.23.0 linux/amd64",
			want:      false,
		},
		{
			name:      "version 3.0.0",
			userAgent: "bee/3.0.0 go1.25.0 linux/amd64",
			want:      false,
		},
		// Pre-release versions >= 2.7.0 should NOT require backward compatibility
		// This is the critical fix: 2.7.0-rcX should be treated as >= 2.7.0
		{
			name:      "version 2.7.0-rc1",
			userAgent: "bee/2.7.0-rc1 go1.23.0 linux/amd64",
			want:      false,
		},
		{
			name:      "version 2.7.0-rc12",
			userAgent: "bee/2.7.0-rc12-b39629d5-dirty go1.25.6 linux/amd64",
			want:      false,
		},
		{
			name:      "version 2.7.0-beta1",
			userAgent: "bee/2.7.0-beta1 go1.23.0 linux/amd64",
			want:      false,
		},
		{
			name:      "version 2.8.0-rc1",
			userAgent: "bee/2.8.0-rc1 go1.24.0 linux/amd64",
			want:      false,
		},
		{
			name:      "version 2.9.0-beta1",
			userAgent: "bee/2.9.0-beta1 go1.24.0 linux/amd64",
			want:      false,
		},
		// Edge cases that should return false (not requiring backward compat)
		{
			name:      "empty user agent",
			userAgent: "",
			want:      false,
		},
		{
			name:      "malformed user agent missing space",
			userAgent: "bee/2.6.0",
			want:      false,
		},
		{
			name:      "non-bee user agent",
			userAgent: "other/1.0.0 go1.22.0 linux/amd64",
			want:      false,
		},
		{
			name:      "invalid version format",
			userAgent: "bee/invalid go1.22.0 linux/amd64",
			want:      false,
		},
		{
			name:      "default libp2p user agent",
			userAgent: "github.com/libp2p/go-libp2p",
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a service with minimal configuration
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			swarmKey, err := crypto.GenerateSecp256k1Key()
			if err != nil {
				t.Fatal(err)
			}

			overlay := swarm.RandAddress(t)
			addr := ":0"
			networkID := uint64(1)

			statestore := mock.NewStateStore()
			defer statestore.Close()

			s, err := New(ctx, crypto.NewDefaultSigner(swarmKey), networkID, overlay, addr, nil, statestore, nil, log.Noop, nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()

			// Create a random test peer ID - we only need any valid libp2p peer ID
			// The peerstore lookup will be mocked by setting the AgentVersion directly
			libp2pPeerID, err := libp2ppeer.Decode("16Uiu2HAm3g4hXfCWTDhPBq3KkqpV3wGkPVgMJY3Jt8gGTYWiTWNZ")
			if err != nil {
				t.Fatal(err)
			}

			// Set the user agent in the peerstore if provided
			if tc.userAgent != "" {
				if err := s.host.Peerstore().Put(libp2pPeerID, "AgentVersion", tc.userAgent); err != nil {
					t.Fatal(err)
				}
			}

			// Test the backward compatibility check
			got := s.bee260BackwardCompatibility(libp2pPeerID)
			if got != tc.want {
				t.Errorf("bee260BackwardCompatibility() = %v, want %v (userAgent: %q)", got, tc.want, tc.userAgent)
			}
		})
	}
}
