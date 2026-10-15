// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

type mockNetStream struct {
	network.Stream
	protocol protocol.ID
}

func (m *mockNetStream) Protocol() protocol.ID {
	return m.protocol
}

func TestStreamVersion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		protocolID protocol.ID
		wantMajor  int64
		wantMinor  int64
		wantPatch  int64
		wantErr    bool
	}{
		{
			name:       "valid standard version",
			protocolID: "/swarm/pingpong/1.2.3/ping",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
		},
		{
			name:       "valid rc version",
			protocolID: "/swarm/pingpong/2.0.0-rc1/ping",
			wantMajor:  2,
			wantMinor:  0,
			wantPatch:  0,
		},
		{
			name:       "invalid version format",
			protocolID: "/swarm/pingpong/abc/ping",
			wantErr:    true,
		},
		{
			name:       "too short protocol ID",
			protocolID: "/ping",
			wantErr:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &mockNetStream{protocol: tc.protocolID}
			srv := &libp2p.Service{}
			wrapped := srv.WrapStream(s)

			v, err := wrapped.Version()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if v == nil {
				t.Fatal("expected version to be non-nil")
			}

			expected := semver.Version{Major: tc.wantMajor, Minor: tc.wantMinor}
			if v.Major != expected.Major || v.Minor != expected.Minor {
				t.Errorf("got version %v, want %v", v, expected)
			}
		})
	}
}
