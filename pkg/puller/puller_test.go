// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller_test

import (
	"errors"
	"io/ioutil"
	"math"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/intervalstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/puller"
	mockps "github.com/ethersphere/bee/pkg/pullsync/mock"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	mockk "github.com/ethersphere/bee/pkg/topology/kademlia/mock"
)

const max = math.MaxUint64

var (
	call = func(b uint8, f, t uint64) c {
		return c{b: b, f: f, t: t}
	}

	reply = mockps.NewReply // alias to make code more readable
)

// test that adding one peer starts syncing
func TestOneSync(t *testing.T) {
	var (
		addr        = test.RandomAddress()
		cursors     = []uint64{1000, 1000, 1000}
		liveReplies = []uint64{1001}
	)

	puller, _, kad, pullsync := newPuller(opts{
		kad: []mockk.Option{
			mockk.WithEachPeerRevCalls(
				mockk.AddrTuple{Addr: addr, PO: 1},
			), mockk.WithDepth(1),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithLiveSyncReplies(liveReplies...)},
		bins:     3,
	})
	defer puller.Close()
	defer pullsync.Close()
	time.Sleep(100 * time.Millisecond)

	kad.Trigger()

	waitCursorsCalled(t, pullsync, addr, false)

	waitSyncCalled(t, pullsync, addr, false)
}

func TestNoSyncOutsideDepth(t *testing.T) {
	var (
		addr        = test.RandomAddress()
		addr2       = test.RandomAddress()
		cursors     = []uint64{1000, 1000, 1000}
		liveReplies = []uint64{1}
	)

	puller, _, kad, pullsync := newPuller(opts{
		kad: []mockk.Option{
			mockk.WithEachPeerRevCalls(
				mockk.AddrTuple{Addr: addr, PO: 1},
				mockk.AddrTuple{Addr: addr2, PO: 1},
			), mockk.WithDepth(2),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithLiveSyncReplies(liveReplies...)},
		bins:     3,
	})
	defer puller.Close()
	defer pullsync.Close()
	time.Sleep(100 * time.Millisecond)

	kad.Trigger()

	waitCursorsCalled(t, pullsync, addr, true)
	waitCursorsCalled(t, pullsync, addr2, true)

	waitSyncCalled(t, pullsync, addr, true)
	waitSyncCalled(t, pullsync, addr2, true)
}

func TestSyncFlow_PeerWithinDepth_Live(t *testing.T) {
	addr := test.RandomAddress()

	for _, tc := range []struct {
		name         string   // name of test
		cursors      []uint64 // mocked cursors to be exchanged from peer
		liveReplies  []uint64
		intervals    string // expected intervals on pivot
		expCalls     []c    // expected historical sync calls
		expLiveCalls []c    // expected live sync calls
	}{
		{
			name: "cursor 0, 1 chunk on live", cursors: []uint64{0, 0},
			intervals:    "[[1 1]]",
			liveReplies:  []uint64{1},
			expLiveCalls: []c{call(1, 1, max), call(1, 2, max)},
		},
		{
			name: "cursor 0 - calls 1-1, 2-5, 6-10", cursors: []uint64{0, 0},
			intervals:    "[[1 10]]",
			liveReplies:  []uint64{1, 5, 10},
			expLiveCalls: []c{call(1, 1, max), call(1, 2, max), call(1, 6, max), call(1, 11, max)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			puller, st, kad, pullsync := newPuller(opts{
				kad: []mockk.Option{
					mockk.WithEachPeerRevCalls(
						mockk.AddrTuple{Addr: addr, PO: 1},
					), mockk.WithDepth(1),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors), mockps.WithLiveSyncReplies(tc.liveReplies...)},
				bins:     2,
			})
			t.Cleanup(func() {
				pullsync.Close()
				puller.Close()
			})
			time.Sleep(100 * time.Millisecond)

			kad.Trigger()
			waitCursorsCalled(t, pullsync, addr, false)
			waitLiveSyncCalled(t, pullsync, addr, false)

			waitCheckCalls(t, tc.expCalls, pullsync.SyncCalls, addr) // hist always empty
			waitCheckCalls(t, tc.expLiveCalls, pullsync.LiveSyncCalls, addr)

			// check the intervals
			checkIntervals(t, st, addr, tc.intervals, 1)
		})
	}
}

func TestSyncFlow_PeerWithinDepth_Historical(t *testing.T) {
	addr := test.RandomAddress()

	for _, tc := range []struct {
		name         string   // name of test
		cursors      []uint64 // mocked cursors to be exchanged from peer
		intervals    string   // expected intervals on pivot
		expCalls     []c      // expected historical sync calls
		expLiveCalls []c      // expected live sync calls
	}{
		{
			name: "1,1 - 1 call", cursors: []uint64{0, 1}, //the third cursor is to make sure we dont get a request for a bin we dont need
			intervals:    "[[1 1]]",
			expCalls:     []c{call(1, 1, 1)},
			expLiveCalls: []c{call(1, 2, math.MaxUint64)},
		},
		{
			name: "1,10 - 1 call", cursors: []uint64{0, 10},
			intervals:    "[[1 10]]",
			expCalls:     []c{call(1, 1, 10)},
			expLiveCalls: []c{call(1, 11, math.MaxUint64)},
		},
		{
			name: "1,50 - 1 call", cursors: []uint64{0, 50},
			intervals:    "[[1 50]]",
			expCalls:     []c{call(1, 1, 50)},
			expLiveCalls: []c{call(1, 51, math.MaxUint64)},
		},
		{
			name: "1,51 - 2 calls", cursors: []uint64{0, 51},
			intervals:    "[[1 51]]",
			expCalls:     []c{call(1, 1, 51), call(1, 51, 51)},
			expLiveCalls: []c{call(1, 52, math.MaxUint64)},
		},
		{
			name: "1,100 - 2 calls", cursors: []uint64{130, 100},
			intervals:    "[[1 100]]",
			expCalls:     []c{call(1, 1, 100), call(1, 51, 100)},
			expLiveCalls: []c{call(1, 101, math.MaxUint64)},
		},
		{
			name: "1,200 - 4 calls", cursors: []uint64{130, 200},
			intervals:    "[[1 200]]",
			expCalls:     []c{call(1, 1, 200), call(1, 51, 200), call(1, 101, 200), call(1, 151, 200)},
			expLiveCalls: []c{call(1, 201, math.MaxUint64)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			puller, st, kad, pullsync := newPuller(opts{
				kad: []mockk.Option{
					mockk.WithEachPeerRevCalls(
						mockk.AddrTuple{Addr: addr, PO: 1},
					), mockk.WithDepth(1),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors), mockps.WithAutoReply(), mockps.WithLiveSyncBlock()},
				bins:     2,
			})
			defer puller.Close()
			defer pullsync.Close()
			time.Sleep(100 * time.Millisecond)

			kad.Trigger()

			waitCursorsCalled(t, pullsync, addr, false)
			waitSyncCalledTimes(t, pullsync, addr, len(tc.expCalls))

			// check historical sync calls
			waitCheckCalls(t, tc.expCalls, pullsync.SyncCalls, addr)
			waitCheckCalls(t, tc.expLiveCalls, pullsync.LiveSyncCalls, addr)

			// check the intervals
			checkIntervals(t, st, addr, tc.intervals, 1)
		})
	}
}

func TestSyncFlow_PeerWithinDepth_Live2(t *testing.T) {
	addr := test.RandomAddress()

	for _, tc := range []struct {
		name         string   // name of test
		cursors      []uint64 // mocked cursors to be exchanged from peer
		liveReplies  []mockps.SyncReply
		intervals    string // expected intervals on pivot
		expCalls     []c    // expected historical sync calls
		expLiveCalls []c    // expected live sync calls
	}{
		{
			name: "cursor 0, 1 chunk on live", cursors: []uint64{0, 0, 0, 0, 0},
			intervals:    "[[1 1]]",
			liveReplies:  []mockps.SyncReply{reply(2, 1, 1, false), reply(2, 2, 0, true), reply(3, 1, 1, false), reply(3, 2, 0, true), reply(4, 1, 1, false), reply(4, 2, 0, true)},
			expLiveCalls: []c{call(2, 1, max), call(2, 2, max), call(3, 1, max), call(3, 2, max), call(4, 1, max), call(4, 2, max)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			puller, st, kad, pullsync := newPuller(opts{
				kad: []mockk.Option{
					mockk.WithEachPeerRevCalls(
						mockk.AddrTuple{Addr: addr, PO: 3}, // po is 3, depth is 2, so we're in depth
					), mockk.WithDepth(2),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors), mockps.WithLateSyncReply(tc.liveReplies...)},
				bins:     5,
			})
			defer puller.Close()
			defer pullsync.Close()
			time.Sleep(100 * time.Millisecond)

			kad.Trigger()
			pullsync.TriggerChange()
			waitCursorsCalled(t, pullsync, addr, false)
			waitLiveSyncCalledTimes(t, pullsync, addr, len(tc.expLiveCalls))
			time.Sleep(100 * time.Millisecond)

			waitCheckCalls(t, tc.expCalls, pullsync.SyncCalls, addr) // hist always empty
			checkCallsUnordered(t, tc.expLiveCalls, pullsync.LiveSyncCalls(addr))

			// check the intervals
			checkIntervals(t, st, addr, tc.intervals, 2)
		})
	}
}

func TestPeerDisconnected(t *testing.T) {
	cursors := []uint64{0, 0}
	addr := test.RandomAddress()

	p, _, kad, pullsync := newPuller(opts{
		kad: []mockk.Option{
			mockk.WithEachPeerRevCalls(
				mockk.AddrTuple{Addr: addr, PO: 1},
			), mockk.WithDepthCalls(2, 2, 2), // peer moved from out of depth to depth
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors), mockps.WithLiveSyncBlock()},
		bins:     5,
	})
	t.Cleanup(func() {
		pullsync.Close()
		p.Close()
	})

	time.Sleep(50 * time.Millisecond)

	kad.Trigger()
	waitCursorsCalled(t, pullsync, addr, true)
	waitLiveSyncCalled(t, pullsync, addr, true)
	kad.ResetPeers()
	kad.Trigger()
	time.Sleep(50 * time.Millisecond)

	if puller.IsSyncing(p, addr) {
		t.Fatalf("peer is syncing but shouldnt")
	}
}

// TestDepthChange tests that puller reacts correctly to
// depth changes signalled from kademlia.
// Due to the fact that the component does goroutine termination
// and new syncing sessions autonomously, the testing strategy is a bit
// more tricky than usual. The idea is that syncReplies basically allow us
// to somehow see through the inner workings of the syncing strategies.
// When a sync reply is specified with block=true, the protocol mock basically
// returns an interval back to the caller, as if we have successfully synced the
// requested interval. This in turn means that the interval would be persisted
// in the state store, allowing us to inspect for which bins intervals exist when
// we check which bins were synced or not (presence of the key in the db indicates
// the bin was synced). This also means that tweaking these tests needs to
// be done carefully and with the understanding of what each change does to
// the tested unit.
func TestDepthChange(t *testing.T) {
	var (
		addr     = test.RandomAddress()
		interval = "[[1 1]]"
	)

	for _, tc := range []struct {
		name           string
		cursors        []uint64
		binsSyncing    []uint8
		binsNotSyncing []uint8
		syncReplies    []mockps.SyncReply
		depths         []uint8
	}{
		{
			name:        "move peer around", //moves the peer from depth outside of depth then back into depth
			cursors:     []uint64{0, 0, 0, 0, 0},
			binsSyncing: []uint8{3, 4}, binsNotSyncing: []uint8{1, 2},
			syncReplies: []mockps.SyncReply{
				reply(3, 1, 1, false),
				reply(3, 2, 1, true),
				reply(4, 1, 1, false),
				reply(4, 2, 1, true),
			},
			depths: []uint8{0, 1, 2, 3, 4, 4, 0, 3},
		},
		{
			name:           "peer moves out of depth then back in",
			cursors:        []uint64{0, 0, 0, 0, 0},
			binsNotSyncing: []uint8{1, 2}, // only bins 3,4 are expected to sync
			binsSyncing:    []uint8{3, 4},
			syncReplies: []mockps.SyncReply{
				reply(3, 1, 1, false),
				reply(3, 2, 1, true),
				reply(4, 1, 1, false),
				reply(4, 2, 1, true),
			},
			depths: []uint8{0, 1, 2, 3, 4, 3},
		},
		{
			name:           "peer moves out of depth",
			cursors:        []uint64{0, 0, 0, 0, 0},
			binsNotSyncing: []uint8{1, 2, 3, 4}, // no bins should be syncing
			syncReplies: []mockps.SyncReply{
				reply(1, 1, 1, true),
				reply(2, 1, 1, true),
				reply(3, 1, 1, true),
				reply(4, 1, 1, true),
			},
			depths: []uint8{0, 1, 2, 3, 4},
		},
		{
			name:           "peer moves within depth",
			cursors:        []uint64{0, 0, 0, 0, 0},
			binsNotSyncing: []uint8{1, 2}, // only bins 3,4 are expected to sync
			binsSyncing:    []uint8{3, 4},
			depths:         []uint8{0, 1, 2, 3},
			syncReplies: []mockps.SyncReply{
				reply(3, 1, 1, false),
				reply(3, 2, 1, true),
				reply(4, 1, 1, false),
				reply(4, 2, 1, true),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			puller, st, kad, pullsync := newPuller(opts{
				kad: []mockk.Option{
					mockk.WithEachPeerRevCalls(
						mockk.AddrTuple{Addr: addr, PO: 3},
					), mockk.WithDepthCalls(tc.depths...),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors), mockps.WithLateSyncReply(tc.syncReplies...)},
				bins:     5,
			})
			defer puller.Close()
			defer pullsync.Close()

			time.Sleep(100 * time.Millisecond)

			for i := 0; i < len(tc.depths)-1; i++ {
				kad.Trigger()
				time.Sleep(100 * time.Millisecond)
			}
			pullsync.TriggerChange()
			time.Sleep(100 * time.Millisecond)

			// check the intervals
			for _, b := range tc.binsSyncing {
				checkIntervals(t, st, addr, interval, b)
			}

			for _, b := range tc.binsNotSyncing {
				checkNotFound(t, st, addr, b)
			}
		})
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

func checkNotFound(t *testing.T, s storage.StateStorer, addr swarm.Address, bin uint8) {
	t.Helper()
	key := puller.PeerIntervalKey(addr, bin)
	i := &intervalstore.Intervals{}
	err := s.Get(key, i)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatal(err)
		}
		return
	}
	if i.String() == "[]" {
		return
	}
	t.Fatalf("wanted error but got none. bin %d", bin)
}

func waitCheckCalls(t *testing.T, expCalls []c, callsFn func(swarm.Address) []mockps.SyncCall, addr swarm.Address) {
	t.Helper()
	for i := 0; i < 10; i++ {
		time.Sleep(50 * time.Millisecond)
		calls := callsFn(addr)
		if l := len(calls); l != len(expCalls) {
			t.Log(l, len(expCalls), "continue")
			continue
		}
		// check the calls
		for i, v := range expCalls {
			if b := calls[i].Bin; b != v.b {
				t.Errorf("bin mismatch. got %d want %d index %d", b, v.b, i)
			}
			if f := calls[i].From; f != v.f {
				t.Errorf("from mismatch. got %d want %d index %d", f, v.f, i)
			}
			if tt := calls[i].To; tt != v.t {
				t.Errorf("to mismatch. got %d want %d index %d", tt, v.t, i)
			}
		}
		return
	}
	calls := callsFn(addr)
	t.Fatalf("expected %d calls but got %d. calls: %v", len(expCalls), len(calls), calls)
}

// this is needed since there are several goroutines checking the calls,
// so the call list in the test is no longer expected to be in order
func checkCallsUnordered(t *testing.T, expCalls []c, calls []mockps.SyncCall) {
	t.Helper()
	exp := len(expCalls)
	if l := len(calls); l != exp {
		t.Fatalf("expected %d calls but got %d. calls: %v", exp, l, calls)
	}

	isIn := func(vv c, calls []mockps.SyncCall) bool {
		for _, v := range calls {
			if v.Bin == vv.b && v.From == vv.f && v.To == vv.t {
				return true
			}
		}
		return false
	}

	for i, v := range expCalls {
		if !isIn(v, calls) {
			t.Fatalf("call %d not found", i)
		}
	}
}

// waitCursorsCalled waits until GetCursors are called on the given address.
func waitCursorsCalled(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address, invert bool) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if v := ps.CursorsCalls(addr); v {
			if invert {
				t.Fatal("got a call to sync to a peer but shouldnt")
			} else {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)
	}
	if invert {
		return
	}
	t.Fatal("timed out waiting for cursors")
}

// waitLiveSyncCalled waits until SyncInterval is called on the address given.
func waitLiveSyncCalled(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address, invert bool) {
	t.Helper()
	for i := 0; i < 15; i++ {
		v := ps.LiveSyncCalls(addr)
		if len(v) > 0 {
			if invert {
				t.Fatal("got a call to sync to a peer but shouldnt")
			} else {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)
	}
	if invert {
		return
	}
	t.Fatal("timed out waiting for sync")
}

// waitSyncCalled waits until SyncInterval is called on the address given.
func waitSyncCalled(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address, invert bool) {
	t.Helper()
	for i := 0; i < 15; i++ {
		v := ps.SyncCalls(addr)
		if len(v) > 0 {
			if invert {
				t.Fatal("got a call to sync to a peer but shouldnt")
			} else {
				return
			}
		}

		time.Sleep(50 * time.Millisecond)
	}
	if invert {
		return
	}
	t.Fatal("timed out waiting for sync")
}

func waitSyncCalledTimes(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address, times int) {
	t.Helper()
	for i := 0; i < 15; i++ {
		v := ps.SyncCalls(addr)
		if len(v) == times {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for sync")
}

func waitLiveSyncCalledTimes(t *testing.T, ps *mockps.PullSyncMock, addr swarm.Address, times int) {
	t.Helper()
	for i := 0; i < 15; i++ {
		v := ps.LiveSyncCalls(addr)
		if len(v) == times {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for sync")
}

type opts struct {
	pullSync []mockps.Option
	kad      []mockk.Option
	bins     uint8
}

func newPuller(ops opts) (*puller.Puller, storage.StateStorer, *mockk.Mock, *mockps.PullSyncMock) {
	s := mock.NewStateStore()
	ps := mockps.NewPullSync(ops.pullSync...)
	kad := mockk.NewMockKademlia(ops.kad...)
	logger := logging.New(ioutil.Discard, 0)

	o := puller.Options{
		Bins: ops.bins,
	}
	return puller.New(s, kad, ps, logger, o), s, kad, ps
}

type c struct {
	b    uint8  //bin
	f, t uint64 //from, to
}
