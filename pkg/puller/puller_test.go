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

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/puller/intervalstore"
	mockps "github.com/ethersphere/bee/v2/pkg/pullsync/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	resMock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kadMock "github.com/ethersphere/bee/v2/pkg/topology/kademlia/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/google/go-cmp/cmp"
)

// test that adding one peer starts syncing
func TestOneSync(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		cursors = []uint64{1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 1, Start: 1, Topmost: 1000, Peer: addr},
			{Bin: 2, Start: 1, Topmost: 1001, Peer: addr},
		}
	)

	_, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 1},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors, 0), mockps.WithReplies(replies...)},
		bins:     3,
		rs:       resMock.NewReserve(resMock.WithRadius(1)),
	})
	time.Sleep(100 * time.Millisecond)

	kad.Trigger()

	waitCursorsCalled(t, pullsync, addr)
	waitSyncCalledBins(t, pullsync, addr, 1, 2)
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
			{Bin: 3, Start: 1, Topmost: 1000, Peer: addr},
		}
	)

	_, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: addr, PO: 2},
				kadMock.AddrTuple{Addr: addr2, PO: 0},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors, 0), mockps.WithReplies(replies...)},
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, st, kad, pullsync := newPuller(t, opts{
				kad: []kadMock.Option{
					kadMock.WithEachPeerRevCalls(
						kadMock.AddrTuple{Addr: addr, PO: 1},
					),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors, 0), mockps.WithReplies(tc.replies...)},
				bins:     2,
				rs:       resMock.NewReserve(resMock.WithRadius(1)),
			})

			time.Sleep(100 * time.Millisecond)
			kad.Trigger()
			waitSyncCalledBins(t, pullsync, addr, 1)
			waitSyncStart(t, pullsync, addr, tc.replies[len(tc.replies)-1].Start)
			time.Sleep(100 * time.Millisecond)
			checkIntervals(t, st, addr, tc.intervals, 1)
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
		pullSync: []mockps.Option{mockps.WithCursors(cursors, 0)},
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
		t.Fatalf("peer is syncing but shouldn't")
	}
}

func TestEpochReset(t *testing.T) {
	t.Parallel()

	cursors := []uint64{0, 50}

	beforeEpoch := 900
	afterEpoch := 1000

	addr := swarm.RandAddress(t)
	s := mock.NewStateStore()

	replies := []mockps.SyncReply{
		{Bin: 1, Start: 1, Topmost: 50, Peer: addr},
		{Bin: 1, Start: 1, Topmost: 50, Peer: addr},
	}

	peer := kadMock.AddrTuple{Addr: addr, PO: 1}

	p, kad, pullsync := newPullerWithState(t, s, opts{
		kad:      []kadMock.Option{kadMock.WithEachPeerRevCalls(peer)},
		pullSync: []mockps.Option{mockps.WithCursors(cursors, uint64(beforeEpoch)), mockps.WithReplies(replies...)},
		bins:     2,
		rs:       resMock.NewReserve(resMock.WithRadius(1)),
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()
	waitSync(t, pullsync, addr)
	if !p.IsSyncing(addr) {
		t.Fatalf("peer is not syncing but should")
	}
	kad.ResetPeers()
	kad.Trigger()
	time.Sleep(100 * time.Millisecond)
	if p.IsSyncing(addr) {
		t.Fatalf("peer is syncing but shouldn't")
	}

	beforeCalls := pullsync.SyncCalls(addr)

	pullsync.SetEpoch(uint64(afterEpoch))
	pullsync.ResetCalls(addr)

	kad.AddRevPeers(peer)
	kad.Trigger()
	waitSync(t, pullsync, addr)

	afterCalls := pullsync.SyncCalls(addr)

	// after resetting the epoch, the peer will resync all intervals again.
	// Hence why the sync calls from before and now should be the same.
	if diff := cmp.Diff(beforeCalls, afterCalls); diff != "" {
		t.Fatalf("invalid calls (+want -have):\n%s", diff)
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
		pullSync: []mockps.Option{mockps.WithCursors(cursors, 0), mockps.WithReplies(mockps.SyncReply{Bin: 1, Start: 1, Topmost: 1, Peer: addr})},
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

func TestRadiusDecreaseNeighbor(t *testing.T) {
	t.Parallel()

	base := swarm.RandAddress(t)
	peerAddr := swarm.RandAddressAt(t, base, 2)

	var (
		cursors = []uint64{1000, 1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 0, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 1, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 2, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 3, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 0, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 1, Start: 1, Topmost: 1000, Peer: peerAddr},
		}
	)

	// at first, sync all bins
	rs := resMock.NewReserve(resMock.WithRadius(0))

	_, _, kad, pullsync := newPulleAddr(t, base, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: peerAddr, PO: 2},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors, 0), mockps.WithReplies(replies...)},
		bins:     4,
		rs:       rs,
	})

	waitSyncCalledBins(t, pullsync, peerAddr, 0, 1, 2, 3)

	// sync all bins >= 2, as this peer is still within depth
	rs.SetStorageRadius(2)
	kad.Trigger()
	time.Sleep(time.Millisecond * 250)

	// peer is still within depth, resync bins < 2
	pullsync.ResetCalls(swarm.ZeroAddress)
	rs.SetStorageRadius(0)
	kad.Trigger()
	time.Sleep(time.Millisecond * 250)

	waitSyncCalledBins(t, pullsync, peerAddr, 0, 1)
}

func TestRadiusDecreaseNonNeighbor(t *testing.T) {
	t.Parallel()

	base := swarm.RandAddress(t)
	peerAddr := swarm.RandAddressAt(t, base, 1)

	var (
		cursors = []uint64{1000, 1000, 1000, 1000}
		replies = []mockps.SyncReply{
			{Bin: 0, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 1, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 2, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 3, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 0, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 1, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 2, Start: 1, Topmost: 1000, Peer: peerAddr},
			{Bin: 3, Start: 1, Topmost: 1000, Peer: peerAddr},
		}
	)

	// at first, sync all bins
	rs := resMock.NewReserve(resMock.WithRadius(0))

	_, _, kad, pullsync := newPulleAddr(t, base, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(
				kadMock.AddrTuple{Addr: peerAddr, PO: 2},
			),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors, 0), mockps.WithReplies(replies...)},
		bins:     4,
		rs:       rs,
	})

	waitSyncCalledBins(t, pullsync, peerAddr, 0, 1, 2, 3)

	// syncs bin 2 only as this peer is out of depth
	rs.SetStorageRadius(3)
	kad.Trigger()
	time.Sleep(time.Millisecond * 250)

	// peer is now within depth, resync all bins
	pullsync.ResetCalls(swarm.ZeroAddress)
	rs.SetStorageRadius(0)
	kad.Trigger()
	time.Sleep(time.Millisecond * 250)

	waitSyncCalledBins(t, pullsync, peerAddr, 0, 1, 2, 3)
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
		pullSync: []mockps.Option{mockps.WithCursors(cursors, 0), mockps.WithReplies(replies...)},
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
		t.Fatalf("peer is syncing but shouldn't")
	}
}

// TestContinueSyncing adds a single peer with PO 0 to hist and live sync only a peer
// to test that when Sync returns an error, the syncing does not terminate.
func TestContinueSyncing(t *testing.T) {
	t.Parallel()

	addr := swarm.RandAddress(t)

	_, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(kadMock.AddrTuple{Addr: addr, PO: 0}),
		},
		pullSync: []mockps.Option{
			mockps.WithCursors([]uint64{1}, 0),
			mockps.WithSyncError(errors.New("sync error")),
			mockps.WithReplies(
				mockps.SyncReply{Start: 1, Topmost: 2, Peer: addr},
				mockps.SyncReply{Start: 1, Topmost: 2, Peer: addr},
				mockps.SyncReply{Start: 1, Topmost: 2, Peer: addr},
			),
		},
		bins: 1,
		rs:   resMock.NewReserve(resMock.WithRadius(0)),

		syncSleepDur: time.Millisecond * 10,
	})

	time.Sleep(100 * time.Millisecond)
	kad.Trigger()

	err := spinlock.Wait(time.Second, func() bool {
		return len(pullsync.SyncCalls(addr)) == 1
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPeerGone(t *testing.T) {
	t.Parallel()

	var (
		addr    = swarm.RandAddress(t)
		replies = []mockps.SyncReply{
			{Bin: 0, Start: 1, Topmost: 1001, Peer: addr},
			{Bin: 1, Start: 1, Topmost: 1001, Peer: addr},
		}
	)

	p, _, kad, pullsync := newPuller(t, opts{
		kad: []kadMock.Option{
			kadMock.WithEachPeerRevCalls(kadMock.AddrTuple{Addr: addr, PO: 1}),
		},
		pullSync: []mockps.Option{
			mockps.WithCursors([]uint64{1, 1}, 0),
			mockps.WithReplies(replies...),
		},
		bins: 2,
		rs:   resMock.NewReserve(resMock.WithRadius(1)),

		syncSleepDur: time.Millisecond * 10,
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
		t.Fatalf("peer is syncing but shouldn't")
	}
}

func checkIntervals(t *testing.T, s storage.StateStorer, addr swarm.Address, expInterval string, bin uint8) {
	t.Helper()
	key := puller.PeerIntervalKey(addr, bin)
	i := &intervalstore.Intervals{}
	err := s.Get(key, i)
	if err != nil {
		t.Fatalf("error getting interval for bin %d: %v", bin, err)
	}
	if v := i.String(); v != expInterval {
		t.Fatalf("got unexpected interval: %s, want %s bin %d", v, expInterval, bin)
	}
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
	pullSync     []mockps.Option
	kad          []kadMock.Option
	rs           *resMock.ReserveStore
	bins         uint8
	syncSleepDur time.Duration
}

func newPuller(t *testing.T, ops opts) (*puller.Puller, storage.StateStorer, *kadMock.Mock, *mockps.PullSyncMock) {
	t.Helper()

	logger := log.Noop

	s, err := leveldb.NewStateStore(t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}

	ps := mockps.NewPullSync(ops.pullSync...)
	kad := kadMock.NewMockKademlia(ops.kad...)

	o := puller.Options{
		Bins: ops.bins,
	}
	p := puller.New(swarm.RandAddress(t), s, kad, ops.rs, ps, nil, logger, o)
	p.Start(context.Background())

	testutil.CleanupCloser(t, p, s)

	return p, s, kad, ps
}

func newPulleAddr(t *testing.T, addr swarm.Address, ops opts) (*puller.Puller, storage.StateStorer, *kadMock.Mock, *mockps.PullSyncMock) {
	t.Helper()

	logger := log.Noop

	s, err := leveldb.NewStateStore(t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}

	ps := mockps.NewPullSync(ops.pullSync...)
	kad := kadMock.NewMockKademlia(ops.kad...)

	o := puller.Options{
		Bins: ops.bins,
	}
	p := puller.New(addr, s, kad, ops.rs, ps, nil, logger, o)
	p.Start(context.Background())

	testutil.CleanupCloser(t, p, s)

	return p, s, kad, ps
}

func newPullerWithState(t *testing.T, s storage.StateStorer, ops opts) (*puller.Puller, *kadMock.Mock, *mockps.PullSyncMock) {
	t.Helper()

	ps := mockps.NewPullSync(ops.pullSync...)
	kad := kadMock.NewMockKademlia(ops.kad...)
	logger := log.Noop

	o := puller.Options{
		Bins: ops.bins,
	}
	p := puller.New(swarm.RandAddress(t), s, kad, ops.rs, ps, nil, logger, o)
	p.Start(context.Background())

	testutil.CleanupCloser(t, p)

	return p, kad, ps
}
