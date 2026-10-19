// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// assertWantClaims checks the want_claims counters of n by result.
func assertWantClaims(t *testing.T, n *testNode, want map[string]float64) {
	t.Helper()
	for result, v := range want {
		if got := n.svc.WantClaims(result); got != v {
			t.Errorf("want_claims{result=%q} = %v, want %v", result, got, v)
		}
	}
}

// rejectAddr returns a stamp validator that rejects the chunk at bad.
func rejectAddr(bad swarm.Address) func(swarm.Chunk) (swarm.Chunk, error) {
	return func(ch swarm.Chunk) (swarm.Chunk, error) {
		if ch.Address().Equal(bad) {
			return nil, errors.New("invalid stamp")
		}
		return ch, nil
	}
}

// TestFetchWithResult checks the outcome FetchWithResult reports: the chunks
// stored, the requested IDs the peer did not deliver, the IDs claimed through
// another peer and the rejected ones. Only the IDs the peer did not deliver
// are Undelivered; a rejected stamp and a foreign claim are not.
func TestFetchWithResult(t *testing.T) {
	t.Parallel()

	chs := validChunks(5)
	// 0, 1: stored; 2: not held by b; 3: claimed through another peer;
	// 4: stamp rejected
	a := newTestNodeOpts(t, nodeOpts{validStamp: rejectAddr(chs[4].Address())})
	b := newTestNode(t, chs[0], chs[1], chs[3], chs[4])
	connect0(a, b)

	ids := idsOf(t, chs...)
	other := swarm.RandAddress(t)
	if r := a.svc.WantSet().Claim(fpID(ids[3]), other); r != wantset.Granted {
		t.Fatal(r)
	}
	res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 2 || res.Rejected != 1 {
		t.Fatalf("stored %d, rejected %d, want 2, 1", res.Stored, res.Rejected)
	}
	assertChunkIDs(t, res.Undelivered, ids[2])
	assertChunkIDs(t, res.Held, ids[3])
	assertStored(t, a, true, ids[:2]...)
	assertStored(t, a, false, ids[2:]...)
	assertWantClaims(t, a, map[string]float64{"granted": 4, "denied": 1, "expired": 0, "late": 0})
	if l := a.svc.WantSet().Len(); l != 1 {
		t.Fatalf("want set holds %d claims, want 1 (the foreign one)", l)
	}
	assertNotScored(t, a, b.addr)
}

// TestFetchWithResultFailed checks that the IDs of a Want that failed are
// Undelivered: a holder that stalls past the fetch timeout, and a holder that
// delivers an invalid chunk, which is Undelivered too.
func TestFetchWithResultFailed(t *testing.T) {
	t.Parallel()

	t.Run("stalled", func(t *testing.T) {
		t.Parallel()

		chs := validChunks(2)
		a := newTestNodeOpts(t, nodeOpts{fetchTimeout: 100 * time.Millisecond})
		b := newTestNode(t)
		stall := make(chan struct{})
		t.Cleanup(func() { close(stall) })
		fakeHolder(a, b, func(*pb.Want, protobuf.Writer) { <-stall })

		ids := idsOf(t, chs...)
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
		if !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		assertChunkIDs(t, res.Undelivered, ids...)
		assertFailed(t, a.scorer, b.addr)
		assertNoClaims(t, a, ids...)
	})

	t.Run("invalid chunk", func(t *testing.T) {
		t.Parallel()

		chs := validChunks(2)
		a := newTestNode(t)
		b := newTestNode(t)
		bad := delivery(t, chs[0])
		bad.Data = append([]byte(nil), bad.Data...)
		bad.Data[len(bad.Data)-1] ^= 0xff
		fakeHolder(a, b, func(_ *pb.Want, w protobuf.Writer) { _ = w.WriteMsg(bad) })

		ids := idsOf(t, chs...)
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
		if !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		assertChunkIDs(t, res.Undelivered, ids...)
		assertNoClaims(t, a, ids...)
	})
}

// TestFetchRejectedNotRefetched checks the rejected cache (§8.2): a chunk
// whose stamp the stamp validator rejects, or whose stamp is older than the
// one the reserve holds (Put returns ErrOverwriteNewerChunk, the stale stamp
// of §7.7), is requested once. Later fetches within RejectTTL skip it without
// a Want, through every peer, and count it as rejected.
func TestFetchRejectedNotRefetched(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	for _, tc := range []struct {
		name string
		opts nodeOpts
	}{
		{name: "invalid stamp", opts: nodeOpts{validStamp: rejectAddr(chs[0].Address())}},
		{name: "stale stamp", opts: nodeOpts{putHook: func(ch swarm.Chunk) error {
			if ch.Address().Equal(chs[0].Address()) {
				return storage.ErrOverwriteNewerChunk
			}
			return nil
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := newTestNodeOpts(t, tc.opts)
			b := newTestNode(t, chs...)
			c := newTestNode(t, chs...)
			connect0(a, b)

			ids := idsOf(t, chs[0])
			const rounds = 3
			for i := range rounds {
				res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
				if err != nil {
					t.Fatal(err)
				}
				if res.Stored != 0 || res.Rejected != 1 || len(res.Undelivered) != 0 || len(res.Held) != 0 {
					t.Fatalf("fetch %d: %+v", i, res)
				}
			}
			if n := len(sentWants(t, a, b.addr)); n != 1 {
				t.Fatalf("%d wants sent, want 1", n)
			}
			// another peer that lists it: no stream is opened
			res, err := a.svc.FetchWithResult(context.Background(), c.addr, ids)
			if err != nil || res.Rejected != 1 {
				t.Fatalf("fetch from c: %+v, %v", res, err)
			}
			if _, err := a.rec.Records(c.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName); !errors.Is(err, streamtest.ErrRecordsNotFound) {
				t.Fatalf("fetch stream opened to c: %v", err)
			}
			assertStored(t, a, false, ids...)
			assertWantClaims(t, a, map[string]float64{"granted": 1, "rejected": rounds})
			if got := a.svc.RejectedCacheSize(); got != 1 {
				t.Fatalf("rejected_cache_size = %v, want 1", got)
			}
			if l := a.svc.WantSet().Len(); l != 0 {
				t.Fatalf("want set holds %d claims", l)
			}
			assertNotScored(t, a, b.addr)

			// the other chunk is not affected
			stored, err := a.svc.Fetch(context.Background(), b.addr, idsOf(t, chs[1]))
			if err != nil || stored != 1 {
				t.Fatalf("stored %d, err %v", stored, err)
			}
		})
	}
}

// TestFetchRejectTTL checks that a rejected ID is requested again once the
// reject TTL has passed.
func TestFetchRejectTTL(t *testing.T) {
	t.Parallel()

	const ttl = 50 * time.Millisecond
	chs := validChunks(1)
	a := newTestNodeOpts(t, nodeOpts{validStamp: rejectAddr(chs[0].Address())})
	a.svc.SetWantSet(wantset.New(rangesync.ClaimDeadline, ttl))
	b := newTestNode(t, chs...)
	connect0(a, b)

	ids := idsOf(t, chs...)
	fetch := func() {
		t.Helper()
		if _, err := a.svc.Fetch(context.Background(), b.addr, ids); err != nil {
			t.Fatal(err)
		}
	}
	fetch()
	fetch()
	if n := len(sentWants(t, a, b.addr)); n != 1 {
		t.Fatalf("%d wants sent within the TTL, want 1", n)
	}
	time.Sleep(2 * ttl)
	if got := a.svc.RejectedCacheSize(); got != 0 {
		t.Fatalf("rejected_cache_size = %v after the TTL, want 0", got)
	}
	fetch()
	if n := len(sentWants(t, a, b.addr)); n != 2 {
		t.Fatalf("%d wants sent after the TTL, want 2", n)
	}
}

// expireClaims returns a test node whose claims expire after deadline, and
// a holder b whose fetch handler signals the Want, waits for proceed and then
// writes the given deliveries.
func expireClaims(t *testing.T, deadline time.Duration, deliveries ...*pb.Delivery) (a, b *testNode, wanted, proceed chan struct{}) {
	t.Helper()
	a = newTestNode(t)
	a.svc.SetWantSet(wantset.New(deadline, rangesync.RejectTTL))
	b = newTestNode(t)
	wanted, proceed = make(chan struct{}), make(chan struct{})
	fakeHolder(a, b, func(_ *pb.Want, w protobuf.Writer) {
		close(wanted)
		<-proceed
		for _, d := range deliveries {
			_ = w.WriteMsg(d)
		}
	})
	return a, b, wanted, proceed
}

// fetchAsync runs FetchWithResult in a goroutine; the result arrives on the
// returned channel.
func fetchAsync(a *testNode, peer swarm.Address, ids []storer.ChunkID) <-chan rangesync.FetchResult {
	done := make(chan rangesync.FetchResult, 1)
	go func() {
		res, _ := a.svc.FetchWithResult(context.Background(), peer, ids)
		done <- res
	}()
	return done
}

// TestFetchLateDelivery checks want_claims{result="late"}: a delivery that
// arrives after its claim expired and the ID was claimed through another peer
// is counted late, and its claim expired. The chunk is still stored, which
// ends the other claim.
func TestFetchLateDelivery(t *testing.T) {
	t.Parallel()

	const deadline = 50 * time.Millisecond
	chs := validChunks(1)
	a, b, wanted, proceed := expireClaims(t, deadline, delivery(t, chs[0]))
	ids := idsOf(t, chs...)
	done := fetchAsync(a, b.addr, ids)

	<-wanted
	time.Sleep(2 * deadline)
	if r := a.svc.WantSet().Claim(fpID(ids[0]), swarm.RandAddress(t)); r != wantset.Granted {
		t.Fatalf("claim through another peer: %v", r)
	}
	close(proceed)
	res := <-done
	if res.Stored != 1 || len(res.Undelivered) != 0 {
		t.Fatalf("%+v", res)
	}
	assertWantClaims(t, a, map[string]float64{"granted": 1, "expired": 1, "late": 1})
	if l := a.svc.WantSet().Len(); l != 0 {
		t.Fatalf("want set holds %d claims, want 0", l)
	}
}

// TestFetchExpiredClaim checks want_claims{result="expired"}: a claim that
// expired before its fetch released it, with nothing delivered, counts as
// expired and not late, and its ID is Undelivered.
func TestFetchExpiredClaim(t *testing.T) {
	t.Parallel()

	const deadline = 50 * time.Millisecond
	chs := validChunks(1)
	a, b, wanted, proceed := expireClaims(t, deadline)
	ids := idsOf(t, chs...)
	done := fetchAsync(a, b.addr, ids)

	<-wanted
	time.Sleep(2 * deadline)
	close(proceed)
	res := <-done
	assertChunkIDs(t, res.Undelivered, ids...)
	assertWantClaims(t, a, map[string]float64{"granted": 1, "expired": 1, "late": 0})
}
