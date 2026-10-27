// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

// TestCombinedSyncerBothDrivers: with the puller and the reconciler both
// enabled, the combined rate is 0 only when both are 0, so the reserve
// worker lowers the radius only when neither driver is catching up.
func TestCombinedSyncerBothDrivers(t *testing.T) {
	t.Parallel()

	puller, reconciler := &mockSyncer{}, &mockSyncer{}
	s := node.NewCombinedSyncer(puller, reconciler)
	for _, tc := range []struct {
		puller, reconciler float64
		zero               bool
	}{
		{0, 0, true},
		{0.5, 0, false},
		{0, 1, false},
		{0.5, 1, false},
	} {
		puller.rate, reconciler.rate = tc.puller, tc.reconciler
		if got := s.SyncRate(); (got == 0) != tc.zero {
			t.Fatalf("puller %v reconciler %v: got rate %v, want zero=%v", tc.puller, tc.reconciler, got, tc.zero)
		}
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

func TestSyncWiring(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                        string
		full, rs, pull              bool
		addrIndex, register, puller bool
		wantErr                     error
	}{
		{name: "defaults", full: true, pull: true, puller: true},
		{name: "rangesync only", full: true, rs: true, addrIndex: true, register: true},
		{name: "both false", full: true, wantErr: node.ErrNoSyncProtocol},
		{name: "light node with rangesync", rs: true, pull: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, r, p, err := node.ResolveSyncWiring(tc.full, false, tc.rs, tc.pull)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got err %v, want %v", err, tc.wantErr)
			}
			if a != tc.addrIndex || r != tc.register || p != tc.puller {
				t.Fatalf("got addrIndex=%v register=%v puller=%v, want %v %v %v", a, r, p, tc.addrIndex, tc.register, tc.puller)
			}
		})
	}
}

type orderCloser struct {
	name  string
	order *[]string
}

func (c orderCloser) Close() error { *c.order = append(*c.order, c.name); return nil }

func TestRangesyncCloserOrder(t *testing.T) {
	t.Parallel()

	var order []string
	c := node.NewRangesyncCloser(orderCloser{"reconciler", &order}, orderCloser{"service", &order})
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "reconciler" || order[1] != "service" {
		t.Fatalf("close order %v, want [reconciler service]", order)
	}
}

// TestRangesyncOptions: the node's rangesync service reports its sessions'
// outcomes and the wants of the sessions it serves to the reconciler, which
// also gives each of those sessions its Striper (T4.7 part 1); the rest
// keeps its defaults.
func TestRangesyncOptions(t *testing.T) {
	t.Parallel()

	r := reconciler.New(swarm.RandAddress(t), nil, nil, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })
	o := node.RangesyncOptions(r, 250)
	if o.Scorer != r || o.Striper == nil || o.Wants == nil {
		t.Fatalf("options %+v, want the reconciler as scorer, striper and wants", o)
	}
	if _, ok := o.Striper(swarm.RandAddress(t)).(rangesync.TransferStriper); !ok {
		t.Fatal("the served sessions' Striper is not a TransferStriper")
	}
	if o.FetchRate != 250 {
		t.Fatalf("fetch rate %d, want the flag's 250", o.FetchRate)
	}
	o.Scorer, o.Striper, o.Wants, o.FetchRate = nil, nil, nil, 0
	if !reflect.DeepEqual(o, rangesync.Options{}) {
		t.Fatalf("options %+v set past the reconciler's and the fetch rate", o)
	}
}

// TestResolveFetchRate (T6.5 part 2, fix round 1): --rangesync-fetch-rate is
// validated on every node; zero, which a zero-valued Options holds, means
// the default on every node (light nodes and NewBee embedders included), a
// negative value or one above rangesync.MaxFetchRate is refused, and a value
// other than zero and the default is ignored, with a warning, where
// rangesync does not run: light and boot nodes, and full nodes without
// --rangesync-enable.
func TestResolveFetchRate(t *testing.T) {
	t.Parallel()

	const def = rangesync.DefaultFetchRate
	zero := node.Options{}.RangesyncFetchRate
	for _, tc := range []struct {
		name                 string
		full, boot, rs, pull bool
		rate                 int
		want                 int
		ignored              bool
		wantErr              error
	}{
		{name: "full defaults", full: true, pull: true, rate: def, want: def},
		{name: "full pullsync only", full: true, pull: true, rate: 250, want: 250, ignored: true},
		{name: "full rangesync only", full: true, rs: true, rate: 250, want: 250},
		{name: "full both", full: true, rs: true, pull: true, rate: 2000, want: 2000},
		{name: "full rangesync default", full: true, rs: true, rate: def, want: def},
		{name: "full rangesync max", full: true, rs: true, rate: rangesync.MaxFetchRate, want: rangesync.MaxFetchRate},
		{name: "light defaults", pull: true, rate: def, want: def},
		{name: "light rangesync", rs: true, pull: true, rate: 250, want: 250, ignored: true},
		{name: "boot rangesync", full: true, boot: true, rs: true, pull: true, rate: 250, want: 250, ignored: true},
		{name: "zero options, full pullsync only", full: true, pull: true, rate: zero, want: def},
		{name: "zero options, full rangesync only", full: true, rs: true, rate: zero, want: def},
		{name: "zero options, full both", full: true, rs: true, pull: true, rate: zero, want: def},
		{name: "zero options, light", pull: true, rate: zero, want: def},
		{name: "zero options, light rangesync", rs: true, pull: true, rate: zero, want: def},
		{name: "zero options, boot", full: true, boot: true, pull: true, rate: zero, want: def},
		{name: "negative", full: true, rs: true, rate: -1, wantErr: node.ErrInvalidFetchRate},
		{name: "negative pullsync only", full: true, pull: true, rate: -1, wantErr: node.ErrInvalidFetchRate},
		{name: "negative on a light node", pull: true, rate: -1000, wantErr: node.ErrInvalidFetchRate},
		{name: "above max", full: true, rs: true, rate: rangesync.MaxFetchRate + 1, wantErr: node.ErrInvalidFetchRate},
		{name: "above 1e9", full: true, rs: true, rate: 2_000_000_000, wantErr: node.ErrInvalidFetchRate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := node.ResolveSyncMode(tc.full, tc.boot, tc.rs, tc.pull)
			if err != nil {
				t.Fatal(err)
			}
			rate, ignored, err := node.ResolveFetchRate(m, tc.rate)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got err %v, want %v", err, tc.wantErr)
			}
			if rate != tc.want {
				t.Fatalf("got rate %d, want %d", rate, tc.want)
			}
			if ignored != tc.ignored {
				t.Fatalf("got ignored=%v, want %v", ignored, tc.ignored)
			}
		})
	}
}

// TestRangesyncFetchRateWiring (T6.5 part 2, fix round 1): the
// --rangesync-fetch-rate value, as NewBee resolves it, reaches
// rangesync.Options.FetchRate on a full node that runs rangesync, the default
// for a zero-valued Options, and no rangesync service is built elsewhere, so
// the value goes nowhere.
func TestRangesyncFetchRateWiring(t *testing.T) {
	t.Parallel()

	r := reconciler.New(swarm.RandAddress(t), nil, nil, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })
	for _, tc := range []struct {
		name                 string
		full, boot, rs, pull bool
		register             bool
	}{
		{name: "full rangesync only", full: true, rs: true, register: true},
		{name: "full both", full: true, rs: true, pull: true, register: true},
		{name: "full pullsync only", full: true, pull: true},
		{name: "light rangesync", rs: true, pull: true},
		{name: "boot rangesync", full: true, boot: true, rs: true, pull: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, register, _, err := node.ResolveSyncWiring(tc.full, tc.boot, tc.rs, tc.pull)
			if err != nil {
				t.Fatal(err)
			}
			if register != tc.register {
				t.Fatalf("rangesync built %v, want %v", register, tc.register)
			}
			if !register {
				return
			}
			m, err := node.ResolveSyncMode(tc.full, tc.boot, tc.rs, tc.pull)
			if err != nil {
				t.Fatal(err)
			}
			for _, tr := range []struct{ rate, want int }{
				{node.Options{}.RangesyncFetchRate, rangesync.DefaultFetchRate},
				{rangesync.DefaultFetchRate, rangesync.DefaultFetchRate},
				{1, 1},
				{250, 250},
			} {
				rate, _, err := node.ResolveFetchRate(m, tr.rate)
				if err != nil {
					t.Fatalf("rate %d: %v", tr.rate, err)
				}
				if got := node.RangesyncOptions(r, rate).FetchRate; got != tr.want {
					t.Fatalf("rate %d: fetch rate %d, want %d", tr.rate, got, tr.want)
				}
			}
		})
	}
}
