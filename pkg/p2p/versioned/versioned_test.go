// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package versioned_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/versioned"
	"github.com/prometheus/client_golang/prometheus"
)

type mockStream struct {
	p2p.Stream
	version string
}

func (m mockStream) Version() (*semver.Version, error) {
	if m.version == "" {
		return nil, errors.New("missing version")
	}
	return semver.NewVersion(m.version)
}

func (m mockStream) Close() error {
	return nil
}

func (m mockStream) FullClose() error {
	return nil
}

func TestNewHandlersFunc(t *testing.T) {
	t.Parallel()

	var executed string

	makeHandler := func(name string) p2p.HandlerFunc {
		return func(context.Context, p2p.Peer, p2p.Stream) error {
			executed = name
			return nil
		}
	}

	handlers := []versioned.Handler{
		{Version: semver.New("1.0.0"), Handler: makeHandler("v1.0.0")},
		{Version: semver.New("1.2.0"), Handler: makeHandler("v1.2.0")},
		{Version: semver.New("1.1.0"), Handler: makeHandler("v1.1.0")},
	}

	dispatcher := versioned.NewHandlersFunc(handlers)

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

	t.Run("with options WithMetricCounter WithOnMatchFunc", func(t *testing.T) {
		counterVec := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_version_handled_total",
				Help: "Test counter for handled versions",
			},
			[]string{"version"},
		)

		var matchedVersion *semver.Version
		optsDispatcher := versioned.NewHandlersFunc(
			handlers,
			versioned.WithMetricCounter(counterVec),
			versioned.WithOnMatchFunc(func(version *semver.Version) {
				matchedVersion = version
			}),
		)

		err := optsDispatcher(context.Background(), p2p.Peer{}, mockStream{version: "1.1.0"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if matchedVersion == nil || !matchedVersion.Equal(*semver.New("1.1.0")) {
			t.Fatalf("expected matchedVersion = %s, got %v", "1.1.0", matchedVersion)
		}
	})
}
