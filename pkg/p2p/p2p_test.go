// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/libp2p/go-libp2p/core/network"
)

func TestNewSwarmStreamName(t *testing.T) {
	t.Parallel()

	want := "/swarm/hive/1.2.0/peers"
	got := p2p.NewSwarmStreamName("hive", "1.2.0", "peers")

	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestReachabilityStatus_String(t *testing.T) {
	t.Parallel()

	mapping := map[string]string{
		p2p.ReachabilityStatusUnknown.String(): network.ReachabilityUnknown.String(),
		p2p.ReachabilityStatusPrivate.String(): network.ReachabilityPrivate.String(),
		p2p.ReachabilityStatusPublic.String():  network.ReachabilityPublic.String(),
	}
	for have, want := range mapping {
		if have != want {
			t.Fatalf("have reachability status string %q; want %q", have, want)
		}
	}
}

func TestNewVersionedHandlersFunc(t *testing.T) {
	t.Parallel()

	var executed string

	makeHandler := func(name string) p2p.HandlerFunc {
		return func(context.Context, p2p.Peer, p2p.Stream) error {
			executed = name
			return nil
		}
	}

	// Register handlers in intentionally unordered sequence to test automatic sorting
	handlers := []p2p.VersionedHandler{
		{Version: semver.New("1.0.0"), Handler: makeHandler("v1.0.0")},
		{Version: semver.New("1.2.0"), Handler: makeHandler("v1.2.0")},
		{Version: semver.New("1.1.0"), Handler: makeHandler("v1.1.0")},
	}

	dispatcher := p2p.NewVersionedHandlersFunc(handlers...)

	tests := []struct {
		name          string
		streamVersion string
		wantExecuted  string
		wantErr       bool
	}{
		{
			name:          "exact match for highest version (1.2.0)",
			streamVersion: "1.2.0",
			wantExecuted:  "v1.2.0",
		},
		{
			name:          "newer patch version routes to highest version (1.2.5 -> v1.2.0)",
			streamVersion: "1.2.5",
			wantExecuted:  "v1.2.0",
		},
		{
			name:          "future minor version routes to highest version (1.3.0 -> v1.2.0)",
			streamVersion: "1.3.0",
			wantExecuted:  "v1.2.0",
		},
		{
			name:          "exact match for intermediate version (1.1.0)",
			streamVersion: "1.1.0",
			wantExecuted:  "v1.1.0",
		},
		{
			name:          "intermediate patch version (1.1.4 -> v1.1.0)",
			streamVersion: "1.1.4",
			wantExecuted:  "v1.1.0",
		},
		{
			name:          "exact match for lowest version (1.0.0)",
			streamVersion: "1.0.0",
			wantExecuted:  "v1.0.0",
		},
		{
			name:          "lowest version patch (1.0.9 -> v1.0.0)",
			streamVersion: "1.0.9",
			wantExecuted:  "v1.0.0",
		},
		{
			name:          "version below lowest registered version returns error (0.9.0)",
			streamVersion: "0.9.0",
			wantErr:       true,
		},
		{
			name:          "error when stream version cannot be retrieved",
			streamVersion: "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executed = ""
			err := dispatcher(context.Background(), p2p.Peer{}, mockStream{version: tt.streamVersion})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if executed != "" {
					t.Fatalf("expected no handler to execute, but %q executed", executed)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if executed != tt.wantExecuted {
				t.Fatalf("executed handler = %q, want %q", executed, tt.wantExecuted)
			}
		})
	}
}

type mockStream struct {
	p2p.Stream
	version string
	closeFn func() error
}

func (m mockStream) Version() (*semver.Version, error) {
	if m.version == "" {
		return nil, errors.New("missing version")
	}
	return semver.NewVersion(m.version)
}

func (m mockStream) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func (m mockStream) FullClose() error {
	return m.Close()
}

type mockStreamer struct {
	p2p.Streamer
	supportedVersions map[string]bool
	closed            bool
}

func (m *mockStreamer) NewStream(_ context.Context, _ swarm.Address, _ p2p.Headers, _, version, _ string) (p2p.Stream, error) {
	if !m.supportedVersions[version] {
		return nil, errors.New("protocol version not supported")
	}
	return mockStream{
		version: version,
		closeFn: func() error {
			m.closed = true
			return nil
		},
	}, nil
}
