// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/intervalstore"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/puller"
	mockps "github.com/ethersphere/bee/pkg/pullsync/mock"
	"github.com/ethersphere/bee/pkg/spinlock"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	resMock "github.com/ethersphere/bee/pkg/storer/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	kadMock "github.com/ethersphere/bee/pkg/topology/kademlia/mock"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

// test that adding one peer starts syncing
func TestOneSync(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		cursors = []uint64{1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 1, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 2, Start: 1001, Topmost: 1001, Peer: addr}}
	)

	_, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 1},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithReplies(replies...)},
		bins:     3,
		rs:       resMock.NewReserve(resMock.WithRadius(1)),
	})
	time.Sleep(100 * time.Millisecond)

	kad.Trigger()

	waitCursorsCalled(t, pullsync, addr)
	waitSyncCalledBins(t, pullsync, addr, 1, 2)
	waitSync(t, pullsync, addr)
}

// TestRefreshIntervals tests that after a round of syncing, the intervals are freshed based on a ticker.
func TestRefreshIntervals(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		cursors = []uint64{1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 1, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 2, Start: 1001, Topmost: 1001, Peer: addr}}
	)

	_, st, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 1},
			),
		},
		pullSync:   []mockps.Option{mockps.WithCursors(cursors), mockps.WithReplies(replies...)},
		bins:       3,
		refreshDur: 500 * time.Millisecond,
		rs:         resMock.NewReserve(resMock.WithRadius(1)),
	})
	time.Sleep(100 * time.Millisecond)
	kad.Trigger()
	waitCursorsCalled(t, pullsync, addr)
	waitSyncCalledBins(t, pullsync, addr, 1, 2)
	waitSync(t, pullsync, addr)

	time.Sleep(500 * time.Millisecond)
	kad.Trigger()
	time.Sleep(500 * time.Millisecond)

	if err := checkIntervals(st, addr, "", 1); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatal(err)
		}
	}

}

func TestSyncOutsideDepth(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		addr2   = swarm.RandAddress(t)
		cursors = []uint64{1000, 1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 0, Start: 1, Topmost: 1000, Peer: addr2},
			{Bin: 2, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 3, Start: 1, Topmost: 1000, Peer: addr}}
	)

	_, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 2},
				kadMock.AddrTuple{Addr: addr2, PO: 0},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithReplies(replies...)},
		bins:     4,
		rs:       resMock.NewReserve(resMock.WithRadius(2)),
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()

	waitCursorsCalled(t, pullsync, addr)
	waitCursorsCalled(t, pullsync, addr2)

	waitSyncCalledBins(t, pullsync, addr, 2, 3)
	waitSyncCalledBins(t, pullsync, addr2, 0)
}

func TestSyncIntervals(t *testing.T) {
	t.Parallel()

	addr := swarm.RandAddress(t)

	for _, tc := range []struct {
		name      string   // name of test
		cursors   []uint64 // mocked cursors to be exchanged from peer
		replies   []mockps.SyncReply
		intervals string // expected intervals on pivot
	}{
		{
			name:      "0, 1 chunk on live",
			cursors:   []uint64{0, 0},
			intervals: "[[1 1]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 1, Peer: addr},
				{Bin: 1, Start: 1, Topmost: 1, Peer: addr},
			},
		},
		{
			name:      "0 - calls 1-1, 2-5, 6-10",
			cursors:   []uint64{0, 0},
			intervals: "[[1 10]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 1, Peer: addr},
				{Bin: 1, Start: 2, Topmost: 5, Peer: addr},
				{Bin: 1, Start: 6, Topmost: 10, Peer: addr},
			},
		},
		{
			name:      "0, 1 - calls 1-1",
			cursors:   []uint64{0, 1},
			intervals: "[[1 1]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 1, Peer: addr},
				{Bin: 1, Start: 1, Topmost: 1, Peer: addr},
			},
		},
		{
			name:      "0, 10 - calls 1-10",
			cursors:   []uint64{0, 10},
			intervals: "[[1 11]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 10, Peer: addr},
				{Bin: 1, Start: 11, Topmost: 11, Peer: addr},
			},
		},
		{
			name:      "0, 50 - 1 call",
			cursors:   []uint64{0, 50},
			intervals: "[[1 50]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 50, Peer: addr},
			},
		},
		{
			name:      "0, 50 - 2 calls",
			cursors:   []uint64{0, 50},
			intervals: "[[1 51]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 50, Peer: addr},
				{Bin: 1, Start: 51, Topmost: 51, Peer: addr},
			},
		},
		{
			name:      "1,100 - 2 calls",
			cursors:   []uint64{130, 100},
			intervals: "[[1 100]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 50, Peer: addr},
				{Bin: 1, Start: 51, Topmost: 100, Peer: addr},
			},
		},
		{
			name:      "1,200 - 4 calls",
			cursors:   []uint64{200, 200},
			intervals: "[[1 200]]",
			replies: []mockps.SyncReply{
				{Bin: 1, Start: 1, Topmost: 1, Peer: addr},
				{Bin: 1, Start: 2, Topmost: 100, Peer: addr},
				{Bin: 1, Start: 101, Topmost: 150, Peer: addr},
				{Bin: 1, Start: 151, Topmost: 200, Peer: addr},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, st, kad, pullsync := newPuller(t, opts{
				kad: []kadMock.Option{
					kadMock.WithEachPeerRevCalls(
						kadMock.AddrTuple{Addr: addr, PO: 1},
					),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors), mockps.WithReplies(tc.replies...)},
				bins:     2,
				rs:       resMock.NewReserve(resMock.WithRadius(1)),
			})

			time.Sleep(100 * time.Millisecond)
			kad.Trigger()
			waitSyncCalledBins(t, pullsync, addr, 1)
			waitSyncStart(t, pullsync, addr, tc.replies[len(tc.replies)-1].Start)
			time.Sleep(100 * time.Millisecond)
			if err := checkIntervals(st, addr, tc.intervals, 1); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPeerDisconnected(t *testing.T) {
	t.Parallel()

	cursors := []uint64{0, 0, 0, 0, 0}
	addr := swarm.RandAddress(t)

	p, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 1},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors)},
		bins:     5,
		rs:       resMock.NewReserve(resMock.WithRadius(2)),
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()
	waitCursorsCalled(t, pullsync, addr)
	if !p.IsSyncing(addr) {
		t.Fatalf("peer is not syncing but should")
	}
	kad.ResetPeers()
	kad.Trigger()
	time.Sleep(50 * time.Millisecond)
	if p.IsSyncing(addr) {
		t.Fatalf("peer is syncing but shouldnt")
	}
}

func TestBinReset(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		cursors = []uint64{1000, 1000, 1000}
	)

	_, s, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 1},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithReplies(mockps.SyncReply{Bin: 1, Start: 1, Topmost: 1, Peer: addr})},
		bins:     3,
		rs:       resMock.NewReserve(resMock.WithRadius(2)),
	})

	time.Sleep(100 * time.Millisecond)

	kad.Trigger()

	waitCursorsCalled(t, pullsync, addr)
	waitSync(t, pullsync, addr)

	kad.ResetPeers()
	kad.Trigger()
	time.Sleep(100 * time.Millisecond)

	if err := s.Get(fmt.Sprintf("sync|001|%s", addr.ByteString()), nil); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("got error %v, want %v", err, storage.ErrNotFound)
	}

	if err := s.Get(fmt.Sprintf("sync|000|%s", addr.ByteString()), nil); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("got error %v, want %v", err, storage.ErrNotFound)
	}
}

func TestRadiusDecrease(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		cursors = []uint64{1000, 1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 2, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 3, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 1, Start: 1, Topmost: 1000, Peer: addr},
		}
	)

	rs := resMock.NewReserve(resMock.WithRadius(2))

	_, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 2},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithReplies(replies...)},
		bins:     4,
		rs:       rs,
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()
	waitSyncCalledBins(t, pullsync, addr, 2, 3)

	pullsync.ResetCalls(swarm.ZeroAddress)
	rs.SetStorageRadius(1)
	kad.Trigger()
	time.Sleep(100 * time.Millisecond)

	waitSyncCalledBins(t, pullsync, addr, 1)
}

func TestRadiusIncrease(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		cursors = []uint64{1000, 1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 1, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 2, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 3, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 1, Start: 1, Topmost: 1000, Peer: addr},
		}
	)

	rs := resMock.NewReserve(resMock.WithRadius(1))

	p, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 1},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithReplies(replies...)},
		bins:     4,
		rs:       rs,
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()
	waitSyncCalledBins(t, pullsync, addr, 2, 3)

	pullsync.ResetCalls(swarm.ZeroAddress)
	rs.SetStorageRadius(2)
	kad.Trigger()
	time.Sleep(100 * time.Millisecond)
	if !p.IsBinSyncing(addr, 1) {
		t.Fatalf("peer is not syncing but should")
	}
	if p.IsBinSyncing(addr, 2) {
		t.Fatalf("peer is syncing but shouldnt")
	}
}

// TestContinueSyncing adds a single peer with PO 0 to hist and live sync only a peer
// to test that when Sync returns an error, the syncing does not terminate.
func TestContinueSyncing(t *testing.T) {
	t.Parallel()

	var (
		addr = swarm.RandAddress(t)
	)

	_, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(kadMock.AddrTuple{Addr: addr, PO: 0}),
		},
		pullSync: []mockps.Option{
			mockps.WithCursors([]uint64{1}),
			mockps.WithSyncError(errors.New("sync error")),
			mockps.WithReplies(
				mockps.SyncReply{Start: 1, Topmost: 2, Peer: addr},
				mockps.SyncReply{Start: 1, Topmost: 2, Peer: addr},
				mockps.SyncReply{Start: 1, Topmost: 2, Peer: addr},
			),
		},
		bins: 1,
		rs:   resMock.NewReserve(resMock.WithRadius(0)),
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()
	time.Sleep(time.Second)

	calls := len(pullsync.SyncCalls(addr))
	if calls != 1 {
		t.Fatalf("unexpected amount of calls, got %d", calls)
	}
}

func TestPeerGone(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		replies = []mockps.SyncReply{
			{Bin: 0, Start: 1, Topmost: 1001, Peer: addr},
			{Bin: 1, Start: 1, Topmost: 1001, Peer: addr}}
	)

	p, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(kadMock.AddrTuple{Addr: addr, PO: 1}),
		},
		pullSync: []mockps.Option{
			mockps.WithCursors([]uint64{1, 1}),
			mockps.WithReplies(replies...),
		},
		bins: 2,
		rs:   resMock.NewReserve(resMock.WithRadius(1)),
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()
	time.Sleep(100 * time.Millisecond)

	beforeCalls := pullsync.SyncCalls(addr)

	if len(beforeCalls) != 1 {
		t.Fatalf("unexpected amount of calls, got %d, want 1", len(beforeCalls))
	}

	kad.ResetPeers()
	kad.Trigger()
	time.Sleep(100 * time.Millisecond)

	afterCalls := pullsync.SyncCalls(addr)

	if len(beforeCalls) != len(afterCalls) {
		t.Fatalf("unexpected new calls to sync interval, expected 0, got %d", len(afterCalls)-len(beforeCalls))
	}

	if p.IsSyncing(addr) {
		t.Fatalf("peer is syncing but shouldnt")
	}
}

func checkIntervals(s storage.StateStorer, addr swarm.Address, expInterval string, bin uint8) error {
	key := puller.PeerIntervalKey(addr, bin)
	i := &intervalstore.Intervals{}
	err := s.Get(key, i)
	if err != nil {
		return fmt.Errorf("error getting interval for bin %d: %w", bin, err)
	}
	if v := i.String(); v != expInterval {
		return fmt.Errorf("got unexpected interval: %s, want %s bin %d", v, expInterval, bin)
	}

	return nil
}

// waitCursorsCalled waits until GetCursors are called on the given address.
func waitCursorsCalled(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address) {
	t.Helper()

	err := spinlock.Wait(time.Second, func() bool {
		return ps.CursorsCalls(addr)
	})
	if err != nil {
		t.Fatal("timed out waiting for cursors")
	}
}

// waitSync waits until SyncInterval is called on the address given.
func waitSync(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address) {
	t.Helper()

	err := spinlock.Wait(time.Second, func() bool {
		v := ps.SyncCalls(addr)
		return len(v) > 0
	})
	if err != nil {
		t.Fatal("timed out waiting for sync")
	}
}

// waitSyncCalled waits until SyncInterval is called on the address given.
func waitSyncStart(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address, start uint64) {
	t.Helper()

	err := spinlock.Wait(time.Second, func() bool {
		calls := ps.SyncCalls(addr)
		for _, c := range calls {
			if c.Start == start {
				return true
			}
		}
		return false
	})
	if err != nil {
		t.Fatal("timed out waiting for sync")
	}
}

func waitSyncCalledBins(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address, bins ...uint8) {
	t.Helper()

	err := spinlock.Wait(time.Second, func() bool {
		calls := ps.SyncCalls(addr)
	nextCall:
		for _, b := range bins {
			for _, c := range calls {
				if c.Bin == b {
					continue nextCall
				}
			}
			return false
		}
		return true
	})
	if err != nil {
		t.Fatal("timed out waiting for sync")
	}
}

type opts struct {
	pullSync   []mockps.Option
	kad        []kadMock.Option
	rs         *resMock.ReserveStore
	bins       uint8
	refreshDur time.Duration
}

func newPuller(t *testing.T, ops opts) (*puller.Puller, storage.StateStorer, *kadMock.Mock, *mockps.PullSyncMock) {
	t.Helper()

	s := mock.NewStateStore()
	ps := mockps.NewPullSync(ops.pullSync...)
	kad := kadMock.NewMockKademlia(ops.kad...)
	logger := log.Noop

	if ops.refreshDur == 0 {
		ops.refreshDur = time.Hour * 24
	}
	o := &puller.Options{
		Bins:                ops.bins,
		RefreshIntervalsDur: ops.refreshDur,
	}
	p := puller.New(s, kad, ops.rs, ps, nil, logger, o)
	p.Start(context.Background())

	testutil.CleanupCloser(t, p)

	return p, s, kad, ps
}
