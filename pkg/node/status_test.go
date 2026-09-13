// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/node"
)

func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status node.Status
		want   string
	}{
		{0, "unknown"},
		{node.StatusStarting, "starting"},
		{node.StatusWaitingChainSync, "waiting_chain_sync"},
		{node.StatusStartingChequebook, "starting_chequebook"},
		{node.StatusOpeningLocalstore, "opening_localstore"},
		{node.StatusResettingReserve, "resetting_reserve"},
		{node.StatusLoadingPostageSnapshot, "loading_postage_snapshot"},
		{node.StatusSyncingPostage, "syncing_postage"},
		{node.StatusUpdatingStakeHeight, "updating_stake_height"},
		{node.StatusWarmingUp, "warming_up"},
		{node.StatusCountingReserve, "counting_reserve"},
		{node.StatusEvictingReserve, "evicting_reserve"},
		{node.StatusSyncingReserve, "syncing_reserve"},
		{node.StatusReady, "ready"},
		{node.Status(99), "unknown"},
	}

	for _, tc := range tests {
		if got := tc.status.String(); got != tc.want {
			t.Fatalf("Status(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestStatusStore(t *testing.T) {
	t.Parallel()

	var unset *node.StatusStore
	if unset.Status() != 0 || unset.StatusCode() != 0 || unset.StatusString() != "unknown" {
		t.Fatal("nil store must report unknown")
	}
	unset.Set(node.StatusReady) // must not panic

	store := node.NewStatusStore()
	if store.StatusString() != "unknown" {
		t.Fatalf("new store = %q, want unknown", store.StatusString())
	}

	store.Set(node.StatusOpeningLocalstore)
	if store.Status() != node.StatusOpeningLocalstore {
		t.Fatalf("got status %d, want %d", store.Status(), node.StatusOpeningLocalstore)
	}
	if store.StatusCode() != int32(node.StatusOpeningLocalstore) {
		t.Fatalf("got code %d, want %d", store.StatusCode(), node.StatusOpeningLocalstore)
	}
	if store.StatusString() != "opening_localstore" {
		t.Fatalf("got name %q, want opening_localstore", store.StatusString())
	}

	var _ api.BeeStatus = store
}

func TestStatusStoreConcurrent(t *testing.T) {
	t.Parallel()

	store := node.NewStatusStore()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				store.Set(node.StatusReady)
				_ = store.Status()
				_ = store.StatusCode()
				_ = store.StatusString()
			}
		}()
	}
	wg.Wait()
}

func TestApplyReservePhase(t *testing.T) {
	t.Parallel()

	syncRate := func() float64 { return 0 }
	notStabilized := func() bool { return false }
	stabilized := func() bool { return true }

	t.Run("counting and evicting", func(t *testing.T) {
		t.Parallel()
		b := node.NewTestBeeWithStatus()
		b.ApplyReservePhase("counting_reserve", syncRate, notStabilized)
		if b.CurrentStatus() != node.StatusCountingReserve {
			t.Fatalf("got %s, want counting_reserve", b.CurrentStatus())
		}
		b.ApplyReservePhase("evicting_reserve", syncRate, notStabilized)
		if b.CurrentStatus() != node.StatusEvictingReserve {
			t.Fatalf("got %s, want evicting_reserve", b.CurrentStatus())
		}
	})

	t.Run("idle while warming up", func(t *testing.T) {
		t.Parallel()
		b := node.NewTestBeeWithStatus()
		b.ApplyReservePhase("counting_reserve", syncRate, notStabilized)
		b.ApplyReservePhase("idle", syncRate, notStabilized)
		if b.CurrentStatus() != node.StatusWarmingUp {
			t.Fatalf("got %s, want warming_up", b.CurrentStatus())
		}
	})

	t.Run("idle after warmup with pullsync", func(t *testing.T) {
		t.Parallel()
		b := node.NewTestBeeWithStatus()
		b.ApplyReservePhase("idle", func() float64 { return 1.5 }, stabilized)
		if b.CurrentStatus() != node.StatusSyncingReserve {
			t.Fatalf("got %s, want syncing_reserve", b.CurrentStatus())
		}
	})

	t.Run("idle after warmup ready", func(t *testing.T) {
		t.Parallel()
		b := node.NewTestBeeWithStatus()
		b.ApplyReservePhase("idle", syncRate, stabilized)
		if b.CurrentStatus() != node.StatusReady {
			t.Fatalf("got %s, want ready", b.CurrentStatus())
		}
	})

	t.Run("syncing does not override evicting", func(t *testing.T) {
		t.Parallel()
		b := node.NewTestBeeWithStatus()
		b.ApplyReservePhase("evicting_reserve", syncRate, stabilized)
		b.ApplyReservePhase("syncing_reserve", syncRate, stabilized)
		if b.CurrentStatus() != node.StatusEvictingReserve {
			t.Fatalf("got %s, want evicting_reserve", b.CurrentStatus())
		}
	})

	t.Run("warmup ready skipped while evicting", func(t *testing.T) {
		t.Parallel()
		b := node.NewTestBeeWithStatus()
		b.ApplyReservePhase("evicting_reserve", syncRate, stabilized)
		b.SetReadyFromWarmup()
		if b.CurrentStatus() != node.StatusEvictingReserve {
			t.Fatalf("got %s, want evicting_reserve", b.CurrentStatus())
		}
	})
}
