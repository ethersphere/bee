// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/node"
)

type mockSyncer struct {
	rate    float64
	started int
}

func (m *mockSyncer) SyncRate() float64     { return m.rate }
func (m *mockSyncer) Start(context.Context) { m.started++ }

func TestCombinedSyncerSingle(t *testing.T) {
	t.Parallel()

	p := &mockSyncer{rate: 0.3}
	s := node.NewCombinedSyncer(p)
	if got := s.SyncRate(); got != 0.3 {
		t.Fatalf("got rate %v, want 0.3", got)
	}
	p.rate = 0
	if got := s.SyncRate(); got != 0 {
		t.Fatalf("got rate %v, want 0", got)
	}
	s.Start(context.Background())
	if p.started != 1 {
		t.Fatalf("started %d times, want 1", p.started)
	}
}

func TestCombinedSyncerSum(t *testing.T) {
	t.Parallel()

	a, b := &mockSyncer{rate: 2}, &mockSyncer{rate: 1.5}
	s := node.NewCombinedSyncer(a, nil, b)
	if got := s.SyncRate(); got != 3.5 {
		t.Fatalf("got rate %v, want 3.5", got)
	}
	s.Start(context.Background())
	if a.started != 1 || b.started != 1 {
		t.Fatalf("started a=%d b=%d, want 1 each", a.started, b.started)
	}
	if got := node.NewCombinedSyncer().SyncRate(); got != 0 {
		t.Fatalf("empty: got rate %v, want 0", got)
	}
}

func TestResolveSyncMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                     string
		full, boot, rs, pull     bool
		wantRS, wantPuller, warn bool
		wantErr                  error
	}{
		{name: "full defaults", full: true, pull: true, wantPuller: true},
		{name: "full rangesync only", full: true, rs: true, wantRS: true},
		{name: "full both", full: true, rs: true, pull: true, wantRS: true, wantPuller: true},
		{name: "full none", full: true, wantErr: node.ErrNoSyncProtocol},
		{name: "light defaults", pull: true},
		{name: "light rangesync", rs: true, pull: true, warn: true},
		{name: "light none", rs: false, pull: false},
		{name: "boot rangesync", full: true, boot: true, rs: true, pull: true, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := node.ResolveSyncMode(tc.full, tc.boot, tc.rs, tc.pull)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got err %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if m.Rangesync != tc.wantRS || m.Puller != tc.wantPuller || m.RangesyncIgnored != tc.warn {
				t.Fatalf("got %+v, want rangesync=%v puller=%v ignored=%v", m, tc.wantRS, tc.wantPuller, tc.warn)
			}
		})
	}
}
