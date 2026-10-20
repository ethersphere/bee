// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	batchstoremock "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
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

// rejectAddr returns a stamp validator that rejects the stamp of the chunk
// at bad, as postage.ValidStamp does for an unknown or expired batch.
func rejectAddr(bad swarm.Address) func(swarm.Chunk) (swarm.Chunk, error) {
	return func(ch swarm.Chunk) (swarm.Chunk, error) {
		if ch.Address().Equal(bad) {
			return nil, fmt.Errorf("batchstore get: %w, %w", storage.ErrNotFound, postage.ErrNotFound)
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
		connect0(a, b)
		sendRoot(t, b, a, whole) // a stall is judged only with a Status
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

// TestFetchRejectedShare checks that each peer's rejected deliveries count
// against its own share of the rejected cache (wantset.Set.RejectFrom), so
// that a peer that delivers many IDs this node rejects pushes out its own
// rejections, not those of other peers.
func TestFetchRejectedShare(t *testing.T) {
	t.Parallel()

	const junk = 6
	rejectAll := func(swarm.Chunk) (swarm.Chunk, error) {
		return nil, fmt.Errorf("batchstore get: %w, %w", storage.ErrNotFound, postage.ErrNotFound)
	}
	chs := validChunks(1 + junk)
	a := newTestNodeOpts(t, nodeOpts{validStamp: rejectAll})
	b := newTestNode(t, chs[1:]...)
	c := newTestNode(t, chs[0])

	connect0(a, c) // a's streams go to c
	if res, err := a.svc.FetchWithResult(context.Background(), c.addr, idsOf(t, chs[0])); err != nil || res.Rejected != 1 {
		t.Fatalf("fetch from c: %+v, %v", res, err)
	}
	connect0(a, b) // and now to b
	if res, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs[1:]...)); err != nil || res.Rejected != junk {
		t.Fatalf("fetch from b: %+v, %v", res, err)
	}
	ws := a.svc.WantSet()
	if got, want := ws.RejectedBy(b.addr), junk; got != want {
		t.Fatalf("rejected through b: %d, want %d", got, want)
	}
	if got, want := ws.RejectedBy(c.addr), 1; got != want {
		t.Fatalf("rejected through c: %d, want %d", got, want)
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

// TestFetchClaimRechecksReserve checks the reserve check after the claim
// (§7.8, fetched once): while a fetch from B2 waits for the fetch limiter, a
// fetch from B1 claims x, stores it and ends its claim. B2's claim is then
// granted, but x is not requested again: no Want, no second delivery and no
// cross protocol duplicate. B2's fetch stream, which opened before the wait,
// ends without a Want, which B2 does not hold against this node.
func TestFetchClaimRechecksReserve(t *testing.T) {
	t.Parallel()

	chs := validChunks(1)
	checked := make(chan struct{})
	var once sync.Once
	a := newTestNodeOpts(t, nodeOpts{
		fetchRate: 2, // a token every 500 ms once the burst is gone
		hasHook:   func(swarm.Address) { once.Do(func() { close(checked) }) },
	})
	b2 := newTestNode(t, chs...)
	connect0(a, b2)
	if !a.svc.DrainFetchLimiter() {
		t.Fatal("fetch limiter burst not taken")
	}

	ids := idsOf(t, chs...)
	done := fetchAsync(a, b2.addr, ids)
	select {
	case <-checked: // x is a candidate: B2's fetch waits for the limiter
	case <-time.After(5 * time.Second):
		t.Fatal("no reserve check")
	}
	b1 := swarm.RandAddress(t)
	if r := a.svc.WantSet().Claim(fpID(ids[0]), b1); r != wantset.Granted {
		t.Fatalf("claim through B1: %v", r)
	}
	if err := a.store.ReservePutter().Put(context.Background(), chs[0]); err != nil {
		t.Fatal(err)
	}
	a.svc.WantSet().Done(fpID(ids[0]))

	res := <-done
	if res.Stored != 0 || len(res.Undelivered) != 0 || len(res.Held) != 0 || len(res.Deferred) != 0 {
		t.Fatalf("%+v", res)
	}
	if n := len(sentWants(t, a, b2.addr)); n != 0 {
		t.Fatalf("x requested from B2 after B1 stored it: %d wants", n)
	}
	records, err := a.rec.Records(b2.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if err := r.Err(); errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("B2 found a violation in a stream without a Want: %v", err)
		}
	}
	if fails, _ := b2.scorer.get(a.addr); len(fails) != 0 {
		t.Fatalf("B2 scored this node: %v", fails)
	}
	if d := a.svc.CrossProtocolDuplicates(); d != 0 {
		t.Fatalf("cross protocol duplicates %v, want 0", d)
	}
	assertWantClaims(t, a, map[string]float64{"granted": 1})
	assertNoClaims(t, a)
}

// fillIDs returns n distinct IDs for filling a want set.
func fillIDs(n int) []fingerprint.ID {
	ids := make([]fingerprint.ID, n)
	for i := range ids {
		ids[i][0], ids[i][1], ids[i][2] = 0xee, byte(i>>8), byte(i)
	}
	return ids
}

// TestFetchWantSetFull checks the IDs the want set does not grant because it,
// or the peer's share of it, is full: they are Deferred, not requested, and
// counted in want_claims{peer_full} and {full}. A closed want set grants
// nothing either (want_claims{closed}).
func TestFetchWantSetFull(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		fill     func(t *testing.T, ws *wantset.Set, peer swarm.Address)
		deferred bool
	}{
		{name: "peer_full", deferred: true, fill: func(t *testing.T, ws *wantset.Set, peer swarm.Address) {
			t.Helper()
			for _, id := range fillIDs(wantset.MaxClaimsPerPeer) {
				if r := ws.Claim(id, peer); r != wantset.Granted {
					t.Fatal(r)
				}
			}
		}},
		{name: "full", deferred: true, fill: func(t *testing.T, ws *wantset.Set, _ swarm.Address) {
			t.Helper()
			for i, id := range fillIDs(wantset.MaxClaims) {
				if r := ws.Claim(id, testPeerN(i/wantset.MaxClaimsPerPeer)); r != wantset.Granted {
					t.Fatal(r)
				}
			}
		}},
		{name: "closed", fill: func(t *testing.T, ws *wantset.Set, _ swarm.Address) {
			t.Helper()
			if err := ws.Close(); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			chs := validChunks(1)
			a := newTestNode(t)
			b := newTestNode(t, chs...)
			connect0(a, b)
			tc.fill(t, a.svc.WantSet(), b.addr)

			ids := idsOf(t, chs...)
			res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
			if err != nil {
				t.Fatal(err)
			}
			if tc.deferred {
				assertChunkIDs(t, res.Deferred, ids...)
			} else if len(res.Deferred) != 0 {
				t.Fatalf("deferred %v", res.Deferred)
			}
			if res.Stored != 0 || len(res.Undelivered) != 0 || len(res.Held) != 0 || res.Rejected != 0 {
				t.Fatalf("%+v", res)
			}
			if _, err := a.rec.Records(b.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName); !errors.Is(err, streamtest.ErrRecordsNotFound) {
				t.Fatalf("fetch stream opened: %v", err)
			}
			assertWantClaims(t, a, map[string]float64{tc.name: 1, "granted": 0})
		})
	}
}

// testPeerN returns a distinct overlay for i.
func testPeerN(i int) swarm.Address {
	b := make([]byte, swarm.HashSize)
	b[0], b[1] = 0x77, byte(i)
	return swarm.NewAddress(b)
}

// TestFetchStampCheckFailure checks that only stamp validator errors about
// the stamp itself reject the ID for RejectTTL. Any other error (a batch
// store failure) releases the claim and reports the ID Unverified, which a
// later fetch requests again; neither is the peer's fault.
func TestFetchStampCheckFailure(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		err    error
		reject bool
	}{
		{name: "unknown batch", err: fmt.Errorf("batchstore get: %w, %w", storage.ErrNotFound, postage.ErrNotFound), reject: true},
		{name: "invalid chunk address", err: postage.ErrInvalidChunkAddress, reject: true},
		{name: "bucket mismatch", err: postage.ErrBucketMismatch, reject: true},
		{name: "invalid index", err: postage.ErrInvalidIndex, reject: true},
		{name: "owner mismatch", err: postage.ErrOwnerMismatch, reject: true},
		{name: "signature length", err: crypto.ErrInvalidLength, reject: true},
		{name: "batch store failure", err: errors.New("leveldb: closed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			chs := validChunks(1)
			var mu sync.Mutex
			fail := true
			a := newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
				mu.Lock()
				defer mu.Unlock()
				if fail {
					fail = false // once
					return nil, tc.err
				}
				return ch, nil
			}})
			b := newTestNode(t, chs...)
			connect0(a, b)

			ids := idsOf(t, chs...)
			res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
			if err != nil {
				t.Fatal(err)
			}
			if got := a.svc.WantSet().IsRejected(fpID(ids[0])); got != tc.reject {
				t.Fatalf("rejected %v, want %v", got, tc.reject)
			}
			if tc.reject {
				if res.Rejected != 1 || len(res.Deferred) != 0 {
					t.Fatalf("%+v", res)
				}
			} else {
				if res.Rejected != 0 || len(res.Undelivered) != 0 || len(res.Deferred) != 0 {
					t.Fatalf("%+v", res)
				}
				assertChunkIDs(t, res.Unverified, ids...)
			}
			if l := a.svc.WantSet().Len(); l != 0 {
				t.Fatalf("want set holds %d claims", l)
			}

			// a later fetch requests it again only when it was not rejected
			res, err = a.svc.FetchWithResult(context.Background(), b.addr, ids)
			if err != nil {
				t.Fatal(err)
			}
			wants := 1
			if !tc.reject {
				wants = 2
				if res.Stored != 1 {
					t.Fatalf("stored %d on the second fetch, want 1", res.Stored)
				}
			}
			if n := len(sentWants(t, a, b.addr)); n != wants {
				t.Fatalf("%d wants, want %d", n, wants)
			}
			assertNotScored(t, a, b.addr)
		})
	}
}

// badSignature returns ch with a stamp whose signature does not recover (an
// invalid recovery code), as a peer that makes up stamps on a real batch
// would deliver it.
func badSignature(ch swarm.Chunk) swarm.Chunk {
	st := ch.Stamp()
	sig := append([]byte(nil), st.Sig()...)
	sig[len(sig)-1] = 0
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(postage.NewStamp(st.BatchID(), st.Index(), st.Timestamp(), sig))
}

// recoverStamp is a stamp validator that fails as postage.ValidStamp does
// when the signature does not recover: with the raw recovery error, before
// any check with a sentinel error.
func recoverStamp(ch swarm.Chunk) (swarm.Chunk, error) {
	if _, err := postage.RecoverBatchOwner(ch.Address(), ch.Stamp()); err != nil {
		return nil, err
	}
	return ch, nil
}

// TestFetchBadSignatureRejected checks that a delivered stamp whose
// signature does not recover is rejected for RejectTTL although the stamp
// validator's error has no sentinel: it is the stamp's fault, so the ID is
// not Unverified, and a later fetch does not request it again.
func TestFetchBadSignatureRejected(t *testing.T) {
	t.Parallel()

	chs := validChunks(2)
	bad := badSignature(chs[0])
	a := newTestNodeOpts(t, nodeOpts{validStamp: recoverStamp})
	b := newTestNode(t, bad, chs[1])
	connect0(a, b)

	ids := idsOf(t, bad, chs[1])
	for range 2 {
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
		if err != nil {
			t.Fatal(err)
		}
		if res.Rejected != 1 || len(res.Unverified) != 0 || len(res.Deferred) != 0 || len(res.Undelivered) != 0 {
			t.Fatalf("%+v", res)
		}
	}
	if !a.svc.WantSet().IsRejected(fpID(ids[0])) {
		t.Fatal("bad signature not rejected")
	}
	if n := len(sentWants(t, a, b.addr)); n != 1 {
		t.Fatalf("%d wants, want 1", n)
	}
	assertStored(t, a, true, ids[1])
	assertNotScored(t, a, b.addr)
}

// signedChunks returns n chunks stamped by the owner of one batch, and the
// node's stamp validator, postage.ValidStamp over a batch store that holds
// that batch.
func signedChunks(t *testing.T, n int) ([]swarm.Chunk, postage.ValidStampFn) {
	t.Helper()
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := crypto.NewEthereumAddress(key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	batch := postagetesting.MustNewBatch(postagetesting.WithOwner(owner))
	issuer := postage.NewStampIssuer("label", "keyID", batch.ID, big.NewInt(3), batch.Depth, batch.BucketDepth, 1000, true)
	stamper := postage.NewStamper(inmemstore.New(), issuer, crypto.NewDefaultSigner(key))
	chs := make([]swarm.Chunk, n)
	for i, ch := range validChunks(n) {
		idAddress, err := storage.IdentityAddress(ch)
		if err != nil {
			t.Fatal(err)
		}
		st, err := stamper.Stamp(ch.Address(), idAddress)
		if err != nil {
			t.Fatal(err)
		}
		chs[i] = swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(st)
	}
	return chs, postage.ValidStamp(batchstoremock.New(batchstoremock.WithBatch(batch)))
}

// TestFetchBadSignatureValidStamp is TestFetchBadSignatureRejected through
// the node's stamp validator, postage.ValidStamp over a batch store: a stamp
// on a real batch whose signature does not recover is rejected, and a valid
// one is stored.
func TestFetchBadSignatureValidStamp(t *testing.T) {
	t.Parallel()

	chs, validStamp := signedChunks(t, 2)
	if _, err := validStamp(chs[0]); err != nil {
		t.Fatalf("valid stamp: %v", err)
	}
	bad := badSignature(chs[1])
	if _, err := validStamp(bad); err == nil {
		t.Fatal("bad signature accepted by the stamp validator")
	}
	a := newTestNodeOpts(t, nodeOpts{validStamp: validStamp})
	b := newTestNode(t, chs[0], bad)
	connect0(a, b)

	ids := idsOf(t, chs[0], bad)
	res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != 1 || res.Rejected != 1 || len(res.Unverified) != 0 || len(res.Undelivered) != 0 {
		t.Fatalf("%+v", res)
	}
	assertStored(t, a, true, ids[0])
	assertStored(t, a, false, ids[1])
	if !a.svc.WantSet().IsRejected(fpID(ids[1])) {
		t.Fatal("bad signature not rejected")
	}
	assertNotScored(t, a, b.addr)
}

// TestFetchPeekSkipsLimiter checks that IDs the want set would not grant
// (here held by another claim) are skipped before the fetch limiter wait,
// so they cost no fetch rate.
func TestFetchPeekSkipsLimiter(t *testing.T) {
	t.Parallel()

	chs := validChunks(1)
	a := newTestNodeOpts(t, nodeOpts{fetchRate: 1}) // a token every second once the burst is gone
	b := newTestNode(t, chs...)
	connect0(a, b)
	if !a.svc.DrainFetchLimiter() {
		t.Fatal("fetch limiter burst not taken")
	}
	ids := idsOf(t, chs...)
	if r := a.svc.WantSet().Claim(fpID(ids[0]), swarm.RandAddress(t)); r != wantset.Granted {
		t.Fatal(r)
	}

	start := time.Now()
	res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("fetch of a held ID waited %s for the fetch limiter", elapsed)
	}
	assertChunkIDs(t, res.Held, ids...)
	assertWantClaims(t, a, map[string]float64{"denied": 1, "granted": 0})
}

// TestFetchPeekCountsPicked checks that the candidates of one Want count
// towards the want set's caps before the fetch limiter wait (PeekPicked):
// with room for two claims, two of five IDs are candidates and charged, and
// the other three are deferred without costing fetch rate.
func TestFetchPeekCountsPicked(t *testing.T) {
	t.Parallel()

	const room = 2
	chs := validChunks(5)
	a := newTestNodeOpts(t, nodeOpts{fetchRate: 1}) // the burst refills a token a second
	b := newTestNode(t, chs...)
	connect0(a, b)
	for i, id := range fillIDs(wantset.MaxClaims - room) {
		if r := a.svc.WantSet().Claim(id, testPeerN(i/wantset.MaxClaimsPerPeer)); r != wantset.Granted {
			t.Fatal(r)
		}
	}

	res, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...))
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != room || len(res.Deferred) != len(chs)-room {
		t.Fatalf("stored %d, deferred %d, want %d, %d", res.Stored, len(res.Deferred), room, len(chs)-room)
	}
	// at most room tokens were taken (one more refills within a second)
	if !a.svc.TakeFetchTokens(rangesync.MaxWantIDs - room) {
		t.Fatal("the fetch charged the fetch limiter for deferred IDs")
	}
	assertWantClaims(t, a, map[string]float64{"granted": room, "full": float64(len(chs) - room)})
}

// TestFetchExpiredDuringPut checks that a claim that expires while Put
// blocks (Put ignores ctx and waits for the reserve's batch lock) is counted
// as expired, not late: the delivery arrived in time.
func TestFetchExpiredDuringPut(t *testing.T) {
	t.Parallel()

	const deadline = 500 * time.Millisecond
	chs := validChunks(1)
	a := newTestNodeOpts(t, nodeOpts{putHook: func(swarm.Chunk) error {
		time.Sleep(2 * deadline)
		return nil
	}})
	a.svc.SetWantSet(wantset.New(deadline, rangesync.RejectTTL))
	b := newTestNode(t, chs...)
	connect0(a, b)

	res, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...))
	if err != nil || res.Stored != 1 {
		t.Fatalf("%+v, %v", res, err)
	}
	assertWantClaims(t, a, map[string]float64{"granted": 1, "expired": 1, "late": 0})
}

// TestFetchExpiredDuringStampCheck checks that a claim that expires while the
// stamp validator waits for the batch store, after which the chunk is found
// in the reserve (another writer stored it), is counted as expired: the
// cross protocol duplicate is within want_claims{expired}.
func TestFetchExpiredDuringStampCheck(t *testing.T) {
	t.Parallel()

	const deadline = 500 * time.Millisecond
	chs := validChunks(1)
	var (
		a, b *testNode
		id   fingerprint.ID
	)
	a = newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
		// the batch store stalls past the claim deadline, while another
		// writer stores the chunk
		for a.svc.WantSet().HeldBy(id, b.addr) {
			time.Sleep(10 * time.Millisecond)
		}
		if err := a.store.ReservePutter().Put(context.Background(), ch); err != nil {
			return nil, err
		}
		return ch, nil
	}})
	a.svc.SetWantSet(wantset.New(deadline, rangesync.RejectTTL))
	b = newTestNode(t, chs...)
	connect0(a, b)
	ids := idsOf(t, chs...)
	id = fpID(ids[0])

	res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
	if err != nil || res.Stored != 0 || len(res.Undelivered) != 0 {
		t.Fatalf("%+v, %v", res, err)
	}
	if d := a.svc.CrossProtocolDuplicates(); d != 1 {
		t.Fatalf("cross protocol duplicates %v, want 1", d)
	}
	assertWantClaims(t, a, map[string]float64{"granted": 1, "expired": 1, "late": 0})
}

// TestFetchLocalStoreError checks that a fetch that fails because this
// node's reserve fails (a lookup or a Put) returns an error wrapping
// ErrLocal, and that the peer is not scored.
func TestFetchLocalStoreError(t *testing.T) {
	t.Parallel()

	storeErr := errors.New("leveldb: closed")
	for _, tc := range []struct {
		name string
		opts nodeOpts
	}{
		{name: "lookup", opts: nodeOpts{storeErr: func() error { return storeErr }}},
		{name: "put", opts: nodeOpts{putHook: func(swarm.Chunk) error { return storeErr }}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			chs := validChunks(2)
			a := newTestNodeOpts(t, tc.opts)
			b := newTestNode(t, chs...)
			connect0(a, b)

			_, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...))
			if !errors.Is(err, rangesync.ErrLocal) || !errors.Is(err, storeErr) {
				t.Fatalf("got %v, want %v wrapping %v", err, rangesync.ErrLocal, storeErr)
			}
			assertNotScored(t, a, b.addr)
			assertNoClaims(t, a)
		})
	}
}

// TestFetchTimeoutClamped checks that a fetch timeout of the claim deadline
// or more is clamped to half of it, so that a fetch ends before its claims
// expire.
func TestFetchTimeoutClamped(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ set, want time.Duration }{
		{0, rangesync.ClaimDeadline / 2},
		{time.Second, time.Second},
		{rangesync.ClaimDeadline - time.Nanosecond, rangesync.ClaimDeadline - time.Nanosecond},
		{rangesync.ClaimDeadline, rangesync.ClaimDeadline / 2},
		{2 * rangesync.ClaimDeadline, rangesync.ClaimDeadline / 2},
	} {
		n := newTestNodeOpts(t, nodeOpts{fetchTimeout: tc.set})
		if got := n.svc.FetchTimeout(); got != tc.want {
			t.Errorf("FetchTimeout %s: got %s, want %s", tc.set, got, tc.want)
		}
	}
}
