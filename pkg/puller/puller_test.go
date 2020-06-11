// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller_test

import (
	"math"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/intervalstore"
	mockk "github.com/ethersphere/bee/pkg/kademlia/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/puller"
	mockps "github.com/ethersphere/bee/pkg/pullsync/mock"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/test"
)

// test that adding one peer start syncing
func TestStartSync(t *testing.T) {
	var (
		addr    = test.RandomAddress()
		cursors = []uint64{1000, 1000, 1000}
	)
	_, _, kad, pullsync := newPuller(opts{
		kad: []mockk.Option{
			mockk.WithEachPeerRevCalls(
				mockk.AddrTuple{A: addr, P: 1},
			),
		},

		pullSync: []mockps.Option{mockps.WithCursors(cursors)},
	})

	runtime.Gosched()
	time.Sleep(10 * time.Millisecond)
	kad.Trigger()

	waitSyncCalled(t, pullsync, addr, false)
}

// test that adding one peer start syncing
// then that adding another peer at the same po
// does not start another syncing session
func TestOneSync(t *testing.T) {
	var (
		addr    = test.RandomAddress()
		addr2   = test.RandomAddress()
		cursors = []uint64{1000, 1000, 1000}
	)
	_, _, kad, pullsync := newPuller(opts{
		kad: []mockk.Option{
			mockk.WithEachPeerRevCalls(
				mockk.AddrTuple{A: addr, P: 1},
				mockk.AddrTuple{A: addr2, P: 1},
			), mockk.WithDepth(2),
		},
		pullSync: []mockps.Option{mockps.WithCursors(cursors)},
	})

	runtime.Gosched()
	time.Sleep(10 * time.Millisecond)

	kad.Trigger()

	waitCursorsCalled(t, pullsync, addr, false)
	waitCursorsCalled(t, pullsync, addr2, true)

	waitSyncCalled(t, pullsync, addr, false)
	waitSyncCalled(t, pullsync, addr2, true)
}

func TestSyncFlow_Live(t *testing.T) {
	addr := test.RandomAddress()
	const max = math.MaxUint64

	call := func(b uint8, f, t uint64) c {
		return c{b: b, f: f, t: t}
	}

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
			_, st, kad, pullsync := newPuller(opts{
				kad: []mockk.Option{
					mockk.WithEachPeerRevCalls(
						mockk.AddrTuple{A: addr, P: 1},
					), mockk.WithDepth(2),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors), mockps.WithLiveSyncReplies(tc.liveReplies...)},
			})
			defer pullsync.Close()

			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)

			kad.Trigger()
			waitCursorsCalled(t, pullsync, addr, false)
			waitLiveSyncCalled(t, pullsync, addr, false)

			checkCalls(t, tc.expCalls, pullsync.SyncCalls(addr)) // hist always empty
			checkCalls(t, tc.expLiveCalls, pullsync.LiveSyncCalls(addr))

			// check the intervals
			checkIntervals(t, st, addr, tc.intervals)
		})
	}
}

func TestSyncFlow_Historical(t *testing.T) {
	addr := test.RandomAddress()

	call := func(b uint8, f, t uint64) c {
		return c{b: b, f: f, t: t}
	}

	for _, tc := range []struct {
		name         string   // name of test
		cursors      []uint64 // mocked cursors to be exchanged from peer
		intervals    string   // expected intervals on pivot
		expCalls     []c      // expected historical sync calls
		expLiveCalls []c      // expected live sync calls
	}{
		{
			name: "1,1 - 1 call", cursors: []uint64{0, 1},
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
			_, st, kad, pullsync := newPuller(opts{
				kad: []mockk.Option{
					mockk.WithEachPeerRevCalls(
						mockk.AddrTuple{A: addr, P: 1},
					), mockk.WithDepth(2),
				},
				pullSync: []mockps.Option{mockps.WithCursors(tc.cursors), mockps.WithAutoReply(), mockps.WithLiveSyncBlock()},
			})
			defer pullsync.Close()

			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)

			kad.Trigger()

			waitCursorsCalled(t, pullsync, addr, false)
			waitSyncCalled(t, pullsync, addr, false)

			// check historical sync calls
			checkCalls(t, tc.expCalls, pullsync.SyncCalls(addr))
			checkCalls(t, tc.expLiveCalls, pullsync.LiveSyncCalls(addr))

			// check the intervals
			checkIntervals(t, st, addr, tc.intervals)
		})
	}
}

func TestPeerMovedOutOfDepth(t *testing.T) {

}

func TestPeerMovedIntoDepth(t *testing.T) {

}

func checkIntervals(t *testing.T, s storage.StateStorer, addr swarm.Address, expInterval string) {
	t.Helper()

	key := puller.PeerIntervalKey(addr, 1)
	i := &intervalstore.Intervals{}
	err := s.Get(key, i)
	if err != nil {
		t.Fatal(err)
	}
	if v := i.String(); v != expInterval {
		t.Fatalf("got unexpected interval: %s, want %s", v, expInterval)
	}
}

func checkCalls(t *testing.T, expCalls []c, calls []mockps.SyncCall) {
	t.Helper()

	exp := len(expCalls)
	if l := len(calls); l != exp {
		t.Fatalf("expected %d calls but got %d. calls: %v", exp, l, calls)
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
}

// waitSyncCalled waits until SyncInterval is called on the address given.
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
	for i := 0; i < 20; i++ {
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
	for i := 0; i < 20; i++ {
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

type opts struct {
	pullSync []mockps.Option
	kad      []mockk.Option
}

func newPuller(ops opts) (*puller.Puller, storage.StateStorer, *mockk.Mock, *mockps.PullSyncMock) {
	s := mock.NewStateStore()
	ps := mockps.NewPullSync(ops.pullSync...)
	kad := mockk.NewMockKademlia(ops.kad...)
	logger := logging.New(os.Stdout, 5)
	//logger := logging.New(ioutil.Discard, 0)

	o := puller.Options{
		Topology:   kad,
		StateStore: s,
		PullSync:   ps,
		Logger:     logger,
	}
	return puller.New(o), s, kad, ps
}

type c struct {
	b    uint8  //bin
	f, t uint64 //from, to
}
