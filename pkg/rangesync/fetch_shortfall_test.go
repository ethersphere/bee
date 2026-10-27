// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// whole is the root of a node at radius 0.
var whole = fingerprint.NewRange(swarm.ZeroAddress, 0)

// chunksInRange returns n valid chunks whose addresses lie in r.
func chunksInRange(n int, r fingerprint.Range) []swarm.Chunk {
	var out []swarm.Chunk
	for len(out) < n {
		if ch := chunktesting.GenerateTestRandomChunk(); r.Contains(ch.Address()) {
			out = append(out, ch)
		}
	}
	return out
}

// missingIDs returns the IDs of n chunks in r that no node holds.
func missingIDs(t *testing.T, n int, r fingerprint.Range) []storer.ChunkID {
	t.Helper()
	ids := make([]storer.ChunkID, n)
	for i := range ids {
		ids[i] = chunkIDOf(t, swarm.NewChunk(addrIn(t, r), []byte{1}).WithStamp(postagetesting.MustNewStamp()))
	}
	return ids
}

// sendRoot has from send to a Status with root and waits until to holds it.
func sendRoot(t *testing.T, from, to *testNode, root fingerprint.Range) {
	t.Helper()
	if err := from.svc.SendStatusWith(context.Background(), to.addr, rangesync.Status{Root: root, LeafBits: serviceLeafBits, Mapping: fingerprint.DefaultMapperID}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the peer's status", func() bool {
		st, ok := to.svc.LastStatus(from.addr)
		return ok && st.Root.Equal(root)
	})
}

// fakeClock is a clock that the test moves.
type fakeClock struct{ now atomic.Int64 }

func newFakeClock() *fakeClock {
	c := new(fakeClock)
	c.now.Store(time.Unix(1_000_000, 0).UnixNano())
	return c
}

func (c *fakeClock) Now() time.Time      { return time.Unix(0, c.now.Load()) }
func (c *fakeClock) Add(d time.Duration) { c.now.Add(int64(d)) }

// set makes c the clock of n's shortfall rules and returns n.
func (c *fakeClock) set(n *testNode) *testNode {
	n.svc.SetNow(c.Now)
	return n
}

// assertShortfallScored checks that peer was reported to n's scorer want
// times in all, the last for a fetch shortfall, and whether it was
// disconnected.
func assertShortfallScored(t *testing.T, n *testNode, peer swarm.Address, want int, disconnected bool) {
	t.Helper()
	fails, _ := n.scorer.get(peer)
	if len(fails) != want {
		t.Fatalf("scorer fails %v, want %d", fails, want)
	}
	if want > 0 && !errors.Is(fails[want-1], rangesync.ErrFetchShortfall) {
		t.Fatalf("last fail %v does not wrap %v", fails[want-1], rangesync.ErrFetchShortfall)
	}
	if want > 0 && errors.Is(fails[want-1], rangesync.ErrInvalidMessage) != disconnected {
		t.Fatalf("last fail %v: a violation %v, want %v", fails[want-1], !disconnected, disconnected)
	}
	if got := n.rec.IsDisconnected(peer); got != disconnected {
		t.Fatalf("peer disconnected %v, want %v", got, disconnected)
	}
}

// TestFetchShortfallTolerance checks the shortfall a fetch tolerates from its
// peer: max(4, 1% of the IDs requested), as the stripe verification.
func TestFetchShortfallTolerance(t *testing.T) {
	t.Parallel()

	for requested, want := range map[int]int{0: 4, 10: 4, 499: 4, 500: 5, 1000: 10, 65536: 655} {
		if got := rangesync.ShortfallTolerance(requested); got != want {
			t.Errorf("tolerance of %d requested: %d, want %d", requested, got, want)
		}
	}
}

// TestShortfallBook checks the per-peer memory of the shortfall rules: one
// root move per peer is excused per RootExcuseInterval, again only for the
// fetches that started before it was first excused (not after the root
// swung back), a missing ID counts when the peer leaves it missing again
// within ShortfallWindow (at most MaxMissingPerPeer remembered), an offense
// repeats one within ShortfallWindow, and Clear forgets the missing IDs and
// offenses, not the excuse. It also checks which root moves are plausible.
func TestShortfallBook(t *testing.T) {
	t.Parallel()

	b := rangesync.NewShortfallBook()
	peer, other := swarm.RandAddress(t), swarm.RandAddress(t)
	now := time.Unix(1_000_000, 0)
	before, after := now.Add(-time.Second), now.Add(time.Second)
	r0, r1, r2 := rangeOf(""), rangeOf("1"), rangeOf("11")

	if !b.Excuse(peer, r0, r1, before, now) {
		t.Fatal("first move not excused")
	}
	if !b.Excuse(peer, r0, r1, before, now.Add(time.Minute)) || !b.Excuse(peer, r0, r1, now, now.Add(time.Minute)) {
		t.Fatal("the same move not excused for a fetch that ran when it was excused")
	}
	if b.Excuse(peer, r0, r1, after, now.Add(time.Minute)) {
		t.Fatal("the same move excused for a fetch that started later (the root swung back)")
	}
	if b.Excuse(peer, r1, r2, before, now.Add(time.Minute)) {
		t.Fatal("a second move excused within the interval")
	}
	if !b.Excuse(other, r0, r1, before, now) {
		t.Fatal("another peer's move not excused")
	}
	if !b.Excuse(peer, r1, r2, after, now.Add(rangesync.RootExcuseInterval)) {
		t.Fatal("a move after the interval not excused")
	}

	ids := []fingerprint.ID{{1}, {2}, {3}}
	if n := b.Relisted(peer, ids[:2], now); n != 0 {
		t.Fatalf("first misses counted %d", n)
	}
	if n := b.Relisted(peer, ids, now.Add(time.Minute)); n != 2 {
		t.Fatalf("misses again counted %d, want 2", n)
	}
	if n := b.Relisted(other, ids, now); n != 0 {
		t.Fatalf("another peer's misses counted %d", n)
	}
	if n := b.Relisted(peer, ids, now.Add(time.Minute+rangesync.ShortfallWindow)); n != 0 {
		t.Fatalf("misses past the window counted %d", n)
	}
	many := make([]fingerprint.ID, rangesync.MaxMissingPerPeer+1)
	for i := range many {
		many[i][0], many[i][1], many[i][2] = 0xaa, byte(i>>8), byte(i)
	}
	later := now.Add(time.Hour)
	b.Relisted(peer, many, later)
	if n := b.Relisted(peer, many[:1], later); n != 0 {
		t.Fatal("the oldest of more than MaxMissingPerPeer kept")
	}
	if n := b.Relisted(peer, many[2:3], later); n != 1 {
		t.Fatal("a recent one dropped")
	}

	if b.Offend(peer, now) || !b.Offend(peer, now.Add(time.Minute)) {
		t.Fatal("offenses: want a first, then a repeat")
	}
	if b.Offend(peer, now.Add(time.Minute+rangesync.ShortfallWindow)) {
		t.Fatal("an offense past the window repeats")
	}
	b.Clear(peer)
	if b.Offend(peer, now.Add(2*time.Minute+rangesync.ShortfallWindow)) {
		t.Fatal("an offense after Clear repeats")
	}
	if b.Excuse(peer, r2, rangeOf("111"), after, now.Add(rangesync.RootExcuseInterval+time.Minute)) {
		t.Fatal("Clear renewed the excuse")
	}

	for _, tc := range []struct {
		from, to fingerprint.Range
		want     bool
	}{
		{r0, r1, true},
		{r0, r2, true},
		{r0, rangeOf("111"), false},
		{r1, r0, false},
		{r1, r1, false},
		{r1, rangeOf("01"), false},
	} {
		if got := rangesync.PlausibleStep(tc.from, tc.to); got != tc.want {
			t.Errorf("step %s to %s plausible %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

// TestFetchShortfallBlatant checks that a fetch whose peer falls short of at
// least half of at least BlatantMinIDs requested IDs is a violation at once:
// a peer that listed junk is scored, disconnected and asked for one Want
// only. Fewer requested IDs are not judged at once, and IDs left missing the
// first time do not count.
func TestFetchShortfallBlatant(t *testing.T) {
	t.Parallel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	ids := idsOf(t, stampedChunks(t, 4*rangesync.MaxWantIDs)...)
	res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
	if !errors.Is(err, rangesync.ErrInvalidMessage) || !errors.Is(err, rangesync.ErrFetchShortfall) {
		t.Fatalf("got %v, want %v and %v", err, rangesync.ErrInvalidMessage, rangesync.ErrFetchShortfall)
	}
	if n := len(sentWants(t, a, b.addr)); n != 1 {
		t.Fatalf("%d wants, want 1", n)
	}
	assertChunkIDs(t, res.Undelivered, ids[:rangesync.MaxWantIDs]...)
	assertShortfallScored(t, a, b.addr, 1, true)
	assertNoClaims(t, a)

	c := newTestNode(t)
	d := newTestNode(t)
	connect0(c, d)
	sendRoot(t, d, c, whole)
	small := missingIDs(t, rangesync.BlatantMinIDs-1, whole)
	if _, err := c.svc.FetchWithResult(context.Background(), d.addr, small); err != nil {
		t.Fatal(err)
	}
	assertNotScored(t, c, d.addr)
}

// TestFetchShortfallRelisted checks the offenses of IDs a peer leaves missing
// again (an inconsistent store, whose index lists chunks it lost, or junk
// listed again): the first time they do not count; above the tolerance the
// second time they are a first offense, scored without a disconnect; a third
// time within ShortfallWindow is a violation. Past the window the first
// misses are forgotten.
func TestFetchShortfallRelisted(t *testing.T) {
	t.Parallel()

	t.Run("offenses", func(t *testing.T) {
		t.Parallel()

		held := validChunks(2)
		a := newTestNode(t)
		b := newTestNode(t, held...)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		missing := missingIDs(t, 5, whole)

		res, err := a.svc.FetchWithResult(context.Background(), b.addr, append(idsOf(t, held...), missing...))
		if err != nil || res.Stored != len(held) {
			t.Fatalf("%+v, %v", res, err)
		}
		assertChunkIDs(t, res.Undelivered, missing...)
		assertNotScored(t, a, b.addr)

		_, err = a.svc.FetchWithResult(context.Background(), b.addr, missing)
		if !errors.Is(err, rangesync.ErrFetchShortfall) || errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("second time: got %v, want a first offense", err)
		}
		assertShortfallScored(t, a, b.addr, 1, false)
		assertNoClaims(t, a)

		_, err = a.svc.FetchWithResult(context.Background(), b.addr, missing)
		if !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("third time: got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		assertShortfallScored(t, a, b.addr, 2, true)
	})

	t.Run("window", func(t *testing.T) {
		t.Parallel()

		clock := newFakeClock()
		a := clock.set(newTestNode(t))
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		missing := missingIDs(t, 5, whole)
		for range 2 {
			if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missing); err != nil {
				t.Fatal(err)
			}
			clock.Add(rangesync.ShortfallWindow)
		}
		assertNotScored(t, a, b.addr)
	})
}

// TestFetchShortfallInvalidStamps checks that stamps invalid on every node
// (here signatures that do not recover, through postage.ValidStamp) count at
// once: up to the tolerance nothing, above it a first offense scored without
// a disconnect, and another within ShortfallWindow a violation. Past the
// window an offense is a first one again.
func TestFetchShortfallInvalidStamps(t *testing.T) {
	t.Parallel()

	chs, validStamp := signedChunks(t, 19)
	bad := make([]swarm.Chunk, len(chs))
	for i, ch := range chs {
		bad[i] = badSignature(ch)
	}
	clock := newFakeClock()
	a := clock.set(newTestNodeOpts(t, nodeOpts{validStamp: validStamp}))
	b := newTestNode(t, bad...)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	fetch := func(chs []swarm.Chunk) error {
		t.Helper()
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...))
		if res.Rejected != len(chs) {
			t.Fatalf("rejected %d, want %d", res.Rejected, len(chs))
		}
		return err
	}

	if err := fetch(bad[:4]); err != nil {
		t.Fatal(err)
	}
	assertNotScored(t, a, b.addr)
	if err := fetch(bad[4:9]); !errors.Is(err, rangesync.ErrFetchShortfall) || errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("got %v, want a first offense", err)
	}
	assertShortfallScored(t, a, b.addr, 1, false)
	clock.Add(rangesync.ShortfallWindow)
	if err := fetch(bad[9:14]); errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("past the window: got %v, want a first offense", err)
	}
	assertShortfallScored(t, a, b.addr, 2, false)
	if err := fetch(bad[14:19]); !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("again within the window: got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	assertShortfallScored(t, a, b.addr, 3, true)
}

// TestFetchShortfallHonestChurn checks that an honest peer under churn is
// never scored for what it does not deliver.
func TestFetchShortfallHonestChurn(t *testing.T) {
	t.Parallel()

	// In-place rewrites (SOCs, GSOCs, re-stamped uploads): the listed stamp
	// of an address was replaced before the fetch, every round, and more
	// than the tolerance at once. The replaced IDs are never listed again.
	t.Run("in-place rewrites", func(t *testing.T) {
		t.Parallel()

		a := newTestNode(t)
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		for round := range 3 {
			fresh := validChunks(2 + 6)
			for _, ch := range fresh {
				if err := b.store.ReservePutter().Put(context.Background(), ch); err != nil {
					t.Fatal(err)
				}
			}
			ids := idsOf(t, fresh[:2]...)
			for _, ch := range fresh[2:] { // listed with the stamp it had before
				ids = append(ids, chunkIDOf(t, swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(postagetesting.MustNewStamp())))
			}
			res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
			if err != nil || res.Stored != 2 || len(res.Undelivered) != 6 {
				t.Fatalf("round %d: %+v, %v", round, res, err)
			}
		}
		assertNotScored(t, a, b.addr)
	})

	// Correlated batch eviction: the peer evicted part of a listing (one
	// batch) before the fetch, and every neighbor fetching from it at once
	// sees the same shortfall. The evicted IDs are never listed again.
	t.Run("correlated batch eviction", func(t *testing.T) {
		t.Parallel()

		held := validChunks(10)
		b := newTestNode(t, held...)
		evicted := missingIDs(t, 9, whole)
		ids := append(idsOf(t, held...), evicted...)
		requesters := make([]*testNode, 0, 2)
		for range 2 {
			a := newTestNode(t)
			connect0(a, b)
			sendRoot(t, b, a, whole)
			requesters = append(requesters, a)
		}
		var wg sync.WaitGroup
		for _, a := range requesters {
			wg.Go(func() {
				res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
				if err != nil || res.Stored != len(held) {
					t.Errorf("%+v, %v", res, err)
				}
			})
		}
		wg.Wait()
		for _, a := range requesters {
			assertNotScored(t, a, b.addr)
		}
	})

	// A radius increase while the fetch runs: the peer's root narrows by a
	// bit and it evicts the half it left, which is most of the fetch.
	t.Run("radius increase", func(t *testing.T) {
		t.Parallel()

		upper := rangeOf("1")
		var a, b *testNode
		var once sync.Once
		a = newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
			once.Do(func() { sendRoot(t, b, a, upper) })
			return ch, nil
		}})
		delivered := validChunks(2)
		b = newTestNodeOpts(t, nodeOpts{addr: addrIn(t, upper)}, delivered...)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		left := missingIDs(t, rangesync.BlatantMinIDs-2, rangeOf("0"))
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, append(idsOf(t, delivered...), left...))
		if err != nil || res.Stored != len(delivered) {
			t.Fatalf("%+v, %v", res, err)
		}
		assertChunkIDs(t, res.Undelivered, left...)
		assertNotScored(t, a, b.addr)
	})

	// Not the peer's doing: IDs another fetch held, stamps this node could
	// not check, rejections that depend on the chain state the two nodes
	// have seen (an unknown or expired batch, an index past the batch depth
	// this node knows) or on this node's reserve (an older stamp than it
	// holds), and a few IDs evicted between the listing and the fetch.
	t.Run("causes on this node", func(t *testing.T) {
		t.Parallel()

		delivered := validChunks(2)
		held := validChunks(6)
		unverified := validChunks(4)
		unknown := validChunks(2)
		index := validChunks(2)
		older := validChunks(2)
		fails := make(map[string]error)
		for _, ch := range unverified {
			fails[ch.Address().ByteString()] = errors.New("leveldb: closed")
		}
		for _, ch := range unknown {
			fails[ch.Address().ByteString()] = fmt.Errorf("batchstore get: %w, %w", storage.ErrNotFound, postage.ErrNotFound)
		}
		for _, ch := range index {
			fails[ch.Address().ByteString()] = postage.ErrInvalidIndex
		}
		isOlder := make(map[string]bool)
		for _, ch := range older {
			isOlder[ch.Address().ByteString()] = true
		}
		a := newTestNodeOpts(t, nodeOpts{
			validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
				if err := fails[ch.Address().ByteString()]; err != nil {
					return nil, err
				}
				return ch, nil
			},
			putHook: func(ch swarm.Chunk) error {
				if isOlder[ch.Address().ByteString()] {
					return storage.ErrOverwriteNewerChunk
				}
				return nil
			},
		})
		var holds []swarm.Chunk
		for _, set := range [][]swarm.Chunk{delivered, held, unverified, unknown, index, older} {
			holds = append(holds, set...)
		}
		b := newTestNode(t, holds...)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		other := swarm.RandAddress(t)
		for _, id := range idsOf(t, held...) {
			if r := a.svc.WantSet().Claim(fpID(id), other); r != wantset.Granted {
				t.Fatal(r)
			}
		}
		evicted := missingIDs(t, 4, whole)
		all := append(idsOf(t, holds...), evicted...)
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, all)
		if err != nil {
			t.Fatal(err)
		}
		if res.Stored != len(delivered) || res.Rejected != len(unknown)+len(index)+len(older) || len(res.Held) != len(held) || len(res.Unverified) != len(unverified) {
			t.Fatalf("%+v", res)
		}
		assertChunkIDs(t, res.Undelivered, evicted...)
		assertNotScored(t, a, b.addr)
	})
}

// TestFetchShortfallExcuseOnce checks that one root move per peer per
// RootExcuseInterval excuses the IDs that left the root: a second move within
// the interval does not, so a peer that lists junk away from its overlay and
// narrows its root before the fetch ends is caught (here blatantly); after
// the interval a move excuses again.
func TestFetchShortfallExcuseOnce(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	var (
		a, b *testNode
		mu   sync.Mutex
		next *fingerprint.Range // b's root to send at the next delivery
	)
	a = clock.set(newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
		mu.Lock()
		root := next
		next = nil
		mu.Unlock()
		if root != nil {
			sendRoot(t, b, a, *root)
		}
		return ch, nil
	}}))
	delivered := chunksInRange(6, rangeOf("111"))
	b = newTestNodeOpts(t, nodeOpts{addr: addrIn(t, rangeOf("111"))}, delivered...)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	// fetch requests chs and junk in away, and has b move its root to root
	// at the first delivery
	fetch := func(chs []swarm.Chunk, root, away string) error {
		t.Helper()
		r := rangeOf(root)
		mu.Lock()
		next = &r
		mu.Unlock()
		_, err := a.svc.FetchWithResult(context.Background(), b.addr, append(idsOf(t, chs...), missingIDs(t, rangesync.BlatantMinIDs-2, rangeOf(away))...))
		return err
	}

	if err := fetch(delivered[0:2], "1", "0"); err != nil {
		t.Fatalf("first move: %v", err)
	}
	assertNotScored(t, a, b.addr)
	if err := fetch(delivered[2:4], "11", "10"); !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("second move within the interval: got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	assertShortfallScored(t, a, b.addr, 1, true)
	clock.Add(rangesync.RootExcuseInterval)
	if err := fetch(delivered[4:6], "111", "110"); err != nil {
		t.Fatalf("a move after the interval: %v", err)
	}
	assertShortfallScored(t, a, b.addr, 1, true)
}

// TestFetchSkipsOutsidePeerRoot checks that a fetch does not request IDs
// outside the peer's root by its last Status (its radius grew since it
// listed them): they are Undelivered, cost no fetch rate and do not count
// against the peer.
func TestFetchSkipsOutsidePeerRoot(t *testing.T) {
	t.Parallel()

	upper := rangeOf("1")
	held := chunksInRange(2, upper)
	a := newTestNode(t)
	b := newTestNodeOpts(t, nodeOpts{addr: addrIn(t, upper)}, held...)
	connect0(a, b)
	sendRoot(t, b, a, upper)
	outside := missingIDs(t, 5, rangeOf("0"))

	res, err := a.svc.FetchWithResult(context.Background(), b.addr, append(idsOf(t, held...), outside...))
	if err != nil || res.Stored != len(held) {
		t.Fatalf("%+v, %v", res, err)
	}
	assertChunkIDs(t, res.Undelivered, outside...)
	wants := sentWants(t, a, b.addr)
	if len(wants) != 1 || len(wants[0].IDs) != len(held) {
		t.Fatalf("wants %v, want one of the %d held IDs", wants, len(held))
	}
	assertNotScored(t, a, b.addr)
	assertNoClaims(t, a)
}

// TestFetchShortfallStatusGone checks that a fetch whose peer's Status is
// gone (the peer disconnected meanwhile, which forgets it) is not judged:
// without the root it could not tell the IDs a radius increase excuses.
func TestFetchShortfallStatusGone(t *testing.T) {
	t.Parallel()

	var a, b *testNode
	var once sync.Once
	a = newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
		once.Do(func() {
			if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
				t.Error(err)
			}
		})
		return ch, nil
	}})
	delivered := validChunks(2)
	b = newTestNode(t, delivered...)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	left := missingIDs(t, rangesync.BlatantMinIDs-2, whole)

	if _, err := a.svc.FetchWithResult(context.Background(), b.addr, append(idsOf(t, delivered...), left...)); err != nil {
		t.Fatal(err)
	}
	assertNotScored(t, a, b.addr)
}

// TestFetchShortfallAfterViolation checks that a fetch that ends with a
// violation (here a holder that stalls its Want) is reported once: the
// stamps invalid on every node that the holder delivered before it stalled,
// above the tolerance, are not judged on top of it.
func TestFetchShortfallAfterViolation(t *testing.T) {
	t.Parallel()

	const bad = 5
	chs, validStamp := signedChunks(t, bad+1)
	deliveries := make([]*pb.Delivery, 0, bad)
	for _, ch := range chs[:bad] {
		deliveries = append(deliveries, delivery(t, badSignature(ch)))
	}
	a := newTestNodeOpts(t, nodeOpts{validStamp: validStamp, fetchTimeout: 100 * time.Millisecond})
	b := newTestNode(t)
	stall := make(chan struct{})
	t.Cleanup(func() { close(stall) })
	fakeHolder(a, b, func(_ *pb.Want, w protobuf.Writer) {
		for _, d := range deliveries {
			_ = w.WriteMsg(d)
		}
		<-stall
	})
	b.rec.SetProtocols(a.svc.Protocol())
	sendRoot(t, b, a, whole)

	ids := make([]storer.ChunkID, 0, bad+1)
	for _, ch := range chs[:bad] {
		ids = append(ids, chunkIDOf(t, badSignature(ch)))
	}
	ids = append(ids, chunkIDOf(t, chs[bad]))
	res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
	if !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	if res.Rejected != bad {
		t.Fatalf("rejected %d, want %d", res.Rejected, bad)
	}
	assertFailed(t, a.scorer, b.addr)
}

// resetFetchStreamer opens fetch streams that end in a reset where the peer
// closes them, as libp2p's do when the peer resets the stream: the
// deliveries before still arrive. hook, if set, runs before each reset;
// while off is set, streams close as usual.
type resetFetchStreamer struct {
	p2p.StreamerDisconnecter
	hook func()
	off  *atomic.Bool
}

func (s resetFetchStreamer) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	st, err := s.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
	if err != nil || stream != rangesync.FetchStreamName || s.off != nil && s.off.Load() {
		return st, err
	}
	return resetOnEOFStream{Stream: st, hook: s.hook}, nil
}

type resetOnEOFStream struct {
	p2p.Stream
	hook func()
}

func (s resetOnEOFStream) Read(p []byte) (int, error) {
	n, err := s.Stream.Read(p)
	if errors.Is(err, io.EOF) {
		if s.hook != nil {
			s.hook()
		}
		return n, errPeerReset
	}
	return n, err
}

// resetStreams returns a nodeOpts streamer of resetFetchStreamer.
func resetStreams(hook func(), off *atomic.Bool) func(p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
	return func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
		return resetFetchStreamer{StreamerDisconnecter: s, hook: hook, off: off}
	}
}

// TestFetchResetBudget checks that fetch streams a peer resets with IDs
// undelivered are charged to a budget per peer: MaxFetchResets of them, then
// one per FetchResetInterval, are not scored (an honest peer resets a stream
// that fails on its side), the next one is a violation. A disconnect clears
// the budget.
func TestFetchResetBudget(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: resetStreams(nil, nil)}))
	b := newTestNode(t)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	fetch := func() error {
		t.Helper()
		_, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole))
		if err == nil {
			t.Fatal("fetch from a reset stream succeeded")
		}
		return err
	}
	for i := range rangesync.MaxFetchResets {
		if err := fetch(); errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("reset %d: %v", i+1, err)
		}
	}
	clock.Add(rangesync.FetchResetInterval) // one more refills
	if err := fetch(); errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("reset after the refill: %v", err)
	}
	assertNotScored(t, a, b.addr)
	if err := fetch(); !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("reset past the budget: got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	assertFailed(t, a.scorer, b.addr)

	if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
		t.Fatal(err)
	}
	sendRoot(t, b, a, whole)
	for i := range rangesync.MaxFetchResets {
		if err := fetch(); errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("reset %d after a reconnect: %v", i+1, err)
		}
	}
}

// TestFetchResetAfterOneDelivery checks that a delivery does not make a
// reset free: a peer that lists junk, delivers one chunk (here one this node
// rejects, an unknown batch, which does not count against it) and resets
// the stream is charged, so it is a violation past the budget. The IDs lost
// to the reset are not judged blatant.
func TestFetchResetAfterOneDelivery(t *testing.T) {
	t.Parallel()

	a := newTestNodeOpts(t, nodeOpts{
		streamer: resetStreams(nil, nil),
		validStamp: func(swarm.Chunk) (swarm.Chunk, error) {
			return nil, fmt.Errorf("batchstore get: %w, %w", storage.ErrNotFound, postage.ErrNotFound)
		},
	})
	// the fetch credit (credit.go) refuses this peer after its first Want
	// (TestFetchCreditOneChunkThenReset): the reset budget is tested on its
	// own here
	a.svc.DisableFetchCredit()
	chs := chunksInRange(rangesync.MaxFetchResets+1, whole)
	b := newTestNode(t, chs...)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	for i, ch := range chs {
		ids := append(idsOf(t, ch), missingIDs(t, rangesync.MaxWantIDs-1, whole)...)
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
		if res.Rejected != 1 {
			t.Fatalf("fetch %d: rejected %d, want 1", i, res.Rejected)
		}
		if got, want := errors.Is(err, rangesync.ErrInvalidMessage), i == rangesync.MaxFetchResets; got != want || err == nil {
			t.Fatalf("fetch %d: %v (a violation: %v, want %v)", i, err, got, want)
		}
	}
	assertFailed(t, a.scorer, b.addr)
}

// TestFetchResetHonest checks that resets which are not the peer's doing
// are never charged: fetches this node ends (its context), streams that
// fail because the peer disconnected (its Status is gone), and the streams
// of a peer already reported for a violation, which is being disconnected
// (one score, not one per fetch that fails with it).
func TestFetchResetHonest(t *testing.T) {
	t.Parallel()

	const fetches = rangesync.MaxFetchResets + 2

	t.Run("this node's cancellations", func(t *testing.T) {
		t.Parallel()

		a := newTestNode(t)
		b := newTestNode(t)
		wanted, release := make(chan struct{}), make(chan struct{})
		t.Cleanup(func() { close(release) })
		fakeHolder(a, b, func(*pb.Want, protobuf.Writer) {
			wanted <- struct{}{}
			<-release
		})
		b.rec.SetProtocols(a.svc.Protocol())
		sendRoot(t, b, a, whole)
		for range fetches {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				_, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, 1, whole))
				done <- err
			}()
			<-wanted
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want %v", err, context.Canceled)
			}
		}
		assertNotScored(t, a, b.addr)
	})

	t.Run("the peer disconnected", func(t *testing.T) {
		t.Parallel()

		var (
			a, b       *testNode
			disconnect atomic.Bool
		)
		disconnect.Store(true)
		a = newTestNodeOpts(t, nodeOpts{streamer: resetStreams(func() {
			if !disconnect.Load() {
				return
			}
			if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
				t.Error(err)
			}
		}, nil)})
		b = newTestNode(t)
		connect0(a, b)
		reset := func() {
			t.Helper()
			if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Fatalf("got %v, want a reset", err)
			}
		}
		for range fetches {
			sendRoot(t, b, a, whole) // the reconnect
			reset()
		}
		// nothing was left charged: after the last reconnect, the whole
		// budget remains for the peer's own resets
		sendRoot(t, b, a, whole)
		disconnect.Store(false)
		for range rangesync.MaxFetchResets {
			reset()
		}
		assertNotScored(t, a, b.addr)
	})

	t.Run("after a violation", func(t *testing.T) {
		t.Parallel()

		var off atomic.Bool
		off.Store(true)
		a := newTestNodeOpts(t, nodeOpts{streamer: resetStreams(nil, &off)})
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		// a blatant shortfall: a violation, while the peer stays connected
		// in this test
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, rangesync.MaxWantIDs, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		off.Store(false)
		for range fetches {
			if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Fatalf("got %v, want a reset", err)
			}
		}
		assertFailed(t, a.scorer, b.addr)
	})
}

// TestFetchShortfallExcuseOscillation checks that a root that swings back
// and forth gets one move excused: after a radius increase is excused, the
// same increase again, once the root moved back, does not excuse a later
// fetch, whose junk away from the peer's overlay is then blatant.
func TestFetchShortfallExcuseOscillation(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	var (
		a, b *testNode
		mu   sync.Mutex
		next *fingerprint.Range // b's root to send at the next delivery
	)
	a = clock.set(newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
		mu.Lock()
		root := next
		next = nil
		mu.Unlock()
		if root != nil {
			sendRoot(t, b, a, *root)
		}
		return ch, nil
	}}))
	delivered := chunksInRange(4, rangeOf("1"))
	b = newTestNodeOpts(t, nodeOpts{addr: addrIn(t, rangeOf("1"))}, delivered...)
	connect0(a, b)
	upper := rangeOf("1")
	swing := func(chs []swarm.Chunk) error {
		t.Helper()
		sendRoot(t, b, a, whole)
		mu.Lock()
		next = &upper
		mu.Unlock()
		_, err := a.svc.FetchWithResult(context.Background(), b.addr, append(idsOf(t, chs...), missingIDs(t, rangesync.BlatantMinIDs-2, rangeOf("0"))...))
		return err
	}

	if err := swing(delivered[:2]); err != nil {
		t.Fatalf("first swing: %v", err)
	}
	assertNotScored(t, a, b.addr)
	clock.Add(time.Second)
	if err := swing(delivered[2:]); !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("second swing: got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	assertShortfallScored(t, a, b.addr, 1, true)
}

// TestFetchShortfallAfterPeerViolation checks that once a peer was reported
// for a violation, which disconnects it, its other fetches within
// ViolationGrace (one minute) are not judged or recorded: they fail with it,
// and one violation is one score.
func TestFetchShortfallAfterPeerViolation(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	a := clock.set(newTestNode(t))
	// the fetch credit (credit.go) would refuse the junk after the first
	// fetch: the shortfall rules are tested on their own here
	a.svc.DisableFetchCredit()
	b := newTestNode(t)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, rangesync.MaxWantIDs, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	missing := missingIDs(t, 5, whole)
	for range 3 { // an offense the second time, a violation the third, if judged
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missing); err != nil {
			t.Fatal(err)
		}
	}
	assertShortfallScored(t, a, b.addr, 1, true)

	// the grace is one minute: just before it the peer is still not judged
	clock.Add(time.Minute - time.Second)
	if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missing); err != nil {
		t.Fatalf("within the grace: %v", err)
	}
	assertShortfallScored(t, a, b.addr, 1, true)

	// past the grace the peer is judged again (here it stayed connected):
	// missing once more, the IDs are a first offense
	clock.Add(time.Second)
	for range 2 {
		_, _ = a.svc.FetchWithResult(context.Background(), b.addr, missing)
	}
	fails, _ := a.scorer.get(b.addr)
	if len(fails) != 2 || !errors.Is(fails[1], rangesync.ErrFetchShortfall) || errors.Is(fails[1], rangesync.ErrInvalidMessage) {
		t.Fatalf("scorer fails %v, want a first offense after the violation", fails)
	}
}

// errOpenFailed is a fetch stream open that fails on the peer's side, as
// libp2p's NewStream does when the peer resets the headers exchange.
var errOpenFailed = errors.New("send headers: read message: stream reset")

// withoutStream returns spec without the stream name.
func withoutStream(spec p2p.ProtocolSpec, name string) p2p.ProtocolSpec {
	spec.StreamSpecs = slices.DeleteFunc(slices.Clone(spec.StreamSpecs), func(s p2p.StreamSpec) bool { return s.Name == name })
	return spec
}

// errInboundFull is the open of a fetch stream to a peer whose inbound
// streams are all taken: its resource manager resets the new stream, which
// libp2p's NewStream reports from the headers exchange as a remote reset.
var errInboundFull = errors.New("send headers: read message: stream reset (remote): code: 0x1002")

// errOutboundFull is the open of a fetch stream that this node's own resource
// manager refuses (its outbound streams are all taken).
var errOutboundFull = errors.New("new stream for peerid: create stream: failed to open stream: transient: cannot reserve outbound stream: resource limit exceeded")

// fetchStreamErr returns a nodeOpts streamErr that fails fetch stream opens
// with err while fail returns true; nil fail: always.
func fetchStreamErr(err error, fail func() bool) func(swarm.Address, string, string, string) error {
	return func(_ swarm.Address, _, _, stream string) error {
		if stream == rangesync.FetchStreamName && (fail == nil || fail()) {
			return err
		}
		return nil
	}
}

// countOpens is a streamer that counts the fetch stream opens it is asked
// for, whatever they end with.
type countOpens struct {
	p2p.StreamerDisconnecter
	opens *atomic.Int64
}

func (c countOpens) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	if stream == rangesync.FetchStreamName {
		c.opens.Add(1)
	}
	return c.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
}

// stallOpens is a streamer whose fetch stream opens wait until their context
// ends while stall is set (a peer that never answers the headers exchange),
// and that counts them.
type stallOpens struct {
	p2p.StreamerDisconnecter
	stall atomic.Bool
	opens atomic.Int64
}

func newStallOpens() *stallOpens {
	s := new(stallOpens)
	s.stall.Store(true)
	return s
}

// wrap is a nodeOpts streamer of s.
func (s *stallOpens) wrap(x p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
	s.StreamerDisconnecter = x
	return s
}

func (s *stallOpens) NewStream(ctx context.Context, addr swarm.Address, hd p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	if stream != rangesync.FetchStreamName || !s.stall.Load() {
		return s.StreamerDisconnecter.NewStream(ctx, addr, hd, protocol, version, stream)
	}
	s.opens.Add(1)
	<-ctx.Done()
	return nil, fmt.Errorf("send headers: %w", ctx.Err())
}

// TestFetchStreamOpenNotCharged checks that a fetch stream that fails to open
// is never charged: the stream opens before the Want takes its fetch rate and
// claims its IDs, so a failed open spent neither. However often the open
// fails (the peer resets or stalls the headers exchange, its inbound streams
// are full, it serves no fetch stream, or this node's own resource manager
// refuses it), the peer is neither scored nor disconnected, nothing was
// requested (no Undelivered), no claim is held and no fetch rate was taken.
// Each open here is a probe after the backoff of the last one. Every fetch
// runs this path, a verifier's among them.
func TestFetchStreamOpenNotCharged(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error // the open's error; nil: the peer serves no fetch stream
	}{
		{"headers reset", errOpenFailed},
		{"headers stalled", fmt.Errorf("send headers: read message: %w", context.DeadlineExceeded)},
		{"inbound streams full", errInboundFull},
		{"this node's resource limit", errOutboundFull},
		{"no fetch handler", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var opens atomic.Int64
			clock := newFakeClock()
			o := nodeOpts{
				fetchRate: 1, // the burst is not refilled meanwhile
				streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
					return countOpens{StreamerDisconnecter: s, opens: &opens}
				},
			}
			if tc.err != nil {
				o.streamErr = fetchStreamErr(tc.err, nil)
			}
			a := clock.set(newTestNodeOpts(t, o))
			b := newTestNode(t)
			connect0(a, b)
			if tc.err == nil {
				a.rec.SetProtocols(withoutStream(b.svc.Protocol(), rangesync.FetchStreamName))
			}
			sendRoot(t, b, a, whole)
			const fetches = 4 * rangesync.MaxFetchResets
			for i := range fetches {
				ids := missingIDs(t, 2, whole)
				res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
				if err == nil || errors.Is(err, rangesync.ErrInvalidMessage) || errors.Is(err, rangesync.ErrOpenBackoff) {
					t.Fatalf("fetch %d: got %v, want an unscored failed open", i, err)
				}
				if len(res.Undelivered) != 0 {
					t.Fatalf("fetch %d: undelivered %v, want none requested", i, res.Undelivered)
				}
				assertNoClaims(t, a, ids...)
				clock.Add(rangesync.MaxOpenBackoff) // past the backoff
			}
			if got := opens.Load(); got != fetches {
				t.Fatalf("%d opens, want %d", got, fetches)
			}
			assertNotScored(t, a, b.addr)
			if !a.svc.DrainFetchLimiter() {
				t.Fatal("failed opens took fetch rate")
			}
		})
	}
}

// TestFetchStreamOpenHeld checks a fetch stream open that the peer holds up
// past the fetch timeout. libp2p's NewStream obeys only the fetch's context
// in its wait for identify, its protocol negotiation and the headers
// exchange, so a peer can hold an open for the whole timeout. The open ends
// with the context's error and is neither a stall violation nor charged: the
// Want takes no fetch rate and claims nothing until its stream is open, so
// only the stream slot is held meanwhile. It backs off the peer's opens:
// fetches fail at once (ErrOpenBackoff) for OpenBackoffBase, then one probes,
// and a held probe doubles the backoff.
func TestFetchStreamOpenHeld(t *testing.T) {
	t.Parallel()

	h := newHeldOpens()
	h.hold.Store(true)
	clock := newFakeClock()
	a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: h.wrap, fetchTimeout: 100 * time.Millisecond, fetchRate: 1}))
	b := newTestNode(t)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	held := func(i int) {
		t.Helper()
		ids := missingIDs(t, 2, whole)
		type result struct {
			res rangesync.FetchResult
			err error
		}
		done := make(chan result, 1)
		go func() {
			res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
			done <- result{res, err}
		}()
		<-h.entered
		if l := a.svc.WantSet().Len(); l != 0 {
			t.Fatalf("open %d: %d claims held while the stream opens, want none", i, l)
		}
		r := <-done
		if !errors.Is(r.err, context.DeadlineExceeded) || errors.Is(r.err, rangesync.ErrInvalidMessage) {
			t.Fatalf("open %d: got %v, want an unscored %v", i, r.err, context.DeadlineExceeded)
		}
		if len(r.res.Undelivered) != 0 {
			t.Fatalf("open %d: undelivered %v, want none requested", i, r.res.Undelivered)
		}
		assertNoClaims(t, a, ids...)
	}
	backedOff := func() {
		t.Helper()
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 2, whole)); !errors.Is(err, rangesync.ErrOpenBackoff) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrOpenBackoff)
		}
		if n := len(h.entered); n != 0 {
			t.Fatalf("%d opens during the backoff", n)
		}
	}
	held(0)
	backedOff()
	clock.Add(rangesync.OpenBackoffBase)
	held(1) // the probe
	clock.Add(rangesync.OpenBackoffBase)
	backedOff() // doubled
	clock.Add(rangesync.OpenBackoffBase)
	held(2)
	assertNotScored(t, a, b.addr)
	if !a.svc.DrainFetchLimiter() {
		t.Fatal("held opens took fetch rate")
	}
}

// TestFetchStalledOpensBackOff checks the fetch calls of a peer that stalls
// every fetch stream open, from the review's proof of concept: its served
// sessions' fetches of the same junk IDs arrive faster than two stalled
// opens per fetch timeout drain them, and nothing is claimed, so they would
// never be merged, nor scored. The first stalled open backs off the peer's
// opens, so the calls queued behind it and the later ones fail at once,
// unscored and without a claim: the calls in flight stay few instead of
// growing with the arrivals.
func TestFetchStalledOpensBackOff(t *testing.T) {
	t.Parallel()

	const arrivals = 80 // one per 25 ms, 2 s
	st := newStallOpens()
	clock := newFakeClock() // the backoff does not end meanwhile
	a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: st.wrap, fetchTimeout: 100 * time.Millisecond}))
	b := newTestNode(t)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	ids := missingIDs(t, 256, whole) // the same junk listing every session

	var inFlight, maxInFlight, backedOff atomic.Int64
	var wg sync.WaitGroup
	for range arrivals {
		wg.Go(func() {
			n := inFlight.Add(1)
			defer inFlight.Add(-1)
			for m := maxInFlight.Load(); n > m && !maxInFlight.CompareAndSwap(m, n); m = maxInFlight.Load() {
			}
			_, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
			if err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Errorf("got %v, want an unscored failed fetch", err)
			}
			if errors.Is(err, rangesync.ErrOpenBackoff) {
				backedOff.Add(1)
			}
		})
		time.Sleep(25 * time.Millisecond)
	}
	wg.Wait()
	// before the first stalled open ends (100 ms) a few calls arrive; after
	// it every call fails at once (generous for a loaded scheduler)
	if m := maxInFlight.Load(); m > 12 {
		t.Fatalf("%d fetch calls in flight at once, want a few", m)
	}
	if o := st.opens.Load(); o > rangesync.MaxFetchStreamsOut {
		t.Fatalf("%d opens, want at most %d (the ones before the backoff)", o, rangesync.MaxFetchStreamsOut)
	}
	if n := backedOff.Load(); n < arrivals-12 {
		t.Fatalf("%d of %d calls backed off", n, arrivals)
	}
	assertNoClaims(t, a, ids...)
	assertNotScored(t, a, b.addr)
}

// TestFetchStreamWaitersBounded checks the bound on the fetch calls that
// wait for a peer's stream slots: past MaxFetchWaiters a call fails at once
// (ErrFetchQueueFull), unscored and without a claim. When the opens holding
// the slots fail, the waiters fail at once too: the opens back off.
func TestFetchStreamWaitersBounded(t *testing.T) {
	t.Parallel()

	const extra = 3
	h := newHeldOpens()
	h.hold.Store(true)
	a := newTestNodeOpts(t, nodeOpts{streamer: h.wrap}) // held opens keep their slots
	b := newTestNode(t)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	total := rangesync.MaxFetchStreamsOut + rangesync.MaxFetchWaiters + extra
	errs := make(chan error, total)
	for range total {
		go func() {
			_, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole))
			errs <- err
		}()
	}
	next := func() error {
		t.Helper()
		select {
		case err := <-errs:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("fetch did not return")
			return nil
		}
	}
	for range extra {
		if err := next(); !errors.Is(err, rangesync.ErrFetchQueueFull) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrFetchQueueFull)
		}
	}
	for range rangesync.MaxFetchStreamsOut {
		<-h.entered
	}
	close(h.release) // the held opens fail
	backedOff := 0
	for range total - extra {
		err := next()
		if err == nil || errors.Is(err, rangesync.ErrInvalidMessage) || errors.Is(err, rangesync.ErrFetchQueueFull) {
			t.Fatalf("got %v, want a failed open or a backoff", err)
		}
		if errors.Is(err, rangesync.ErrOpenBackoff) {
			backedOff++
		}
	}
	if backedOff != rangesync.MaxFetchWaiters {
		t.Fatalf("%d waiters backed off, want %d", backedOff, rangesync.MaxFetchWaiters)
	}
	assertNoClaims(t, a)
	assertNotScored(t, a, b.addr)
}

// scriptedOpens is a streamer whose fetch stream opens each wait for the
// test: an open sends a channel on opens and waits for the error it gets on
// it (nil: it opens) or for its context.
type scriptedOpens struct {
	p2p.StreamerDisconnecter
	opens chan chan error
}

func (s scriptedOpens) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	if stream != rangesync.FetchStreamName {
		return s.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
	}
	c := make(chan error, 1)
	select {
	case s.opens <- c:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case err := <-c:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("send headers: %w", ctx.Err())
	}
	return s.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
}

// TestFetchBackoffSlotsBusy checks that a fetch call fails at once while the
// peer's opens back off, without waiting for one of its stream slots: here
// both are taken, by a fetch whose holder is slow and by the probe after the
// backoff, which may hold its slot for the fetch timeout.
func TestFetchBackoffSlotsBusy(t *testing.T) {
	t.Parallel()

	sc := scriptedOpens{opens: make(chan chan error)}
	clock := newFakeClock()
	a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
		sc.StreamerDisconnecter = s
		return sc
	}}))
	b := newTestNode(t)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	wanted, stall := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(stall) })
	fakeHolder(a, b, func(*pb.Want, protobuf.Writer) {
		close(wanted)
		<-stall
	})
	fetch := func() <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole))
			done <- err
		}()
		return done
	}

	_ = fetch()
	(<-sc.opens) <- nil // opens; its holder stalls
	<-wanted
	failed := fetch()
	(<-sc.opens) <- errOpenFailed
	if err := <-failed; err == nil || errors.Is(err, rangesync.ErrOpenBackoff) {
		t.Fatalf("got %v, want a failed open", err)
	}
	clock.Add(rangesync.OpenBackoffBase)
	probe := fetch()
	probeOpen := <-sc.opens // both slots are taken now
	select {
	case err := <-fetch():
		if !errors.Is(err, rangesync.ErrOpenBackoff) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrOpenBackoff)
		}
	case <-sc.opens:
		t.Fatal("an open while the probe runs")
	case <-time.After(time.Second):
		t.Fatal("a fetch waits for a slot while the opens back off")
	}
	probeOpen <- errOpenFailed
	if err := <-probe; err == nil {
		t.Fatal("the probe opened")
	}
	assertNotScored(t, a, b.addr)
}

// TestOpenGate checks the backoff of a peer's fetch stream opens: a failed
// open backs them off for the base, doubling with each failed probe up to
// the maximum; opens that began before the failure count with it; after the
// backoff one open probes at a time; a probe this node aborts leaves the
// backoff as it was; a success or Clear ends it. A probe that ends after
// Clear (a disconnect) leaves a newer backoff of the peer alone (T4.5 part 2c
// fix round 2): it neither ends that backoff's probe, so two probes never
// run at once, nor counts as its failure.
func TestOpenGate(t *testing.T) {
	t.Parallel()

	const base, maxBackoff = time.Second, 4 * time.Second
	g := rangesync.NewOpenGate(base, maxBackoff)
	p := swarm.RandAddress(t)
	now := time.Unix(1_000_000, 0)
	begin := func(at time.Time, wantProbe, wantOK bool) rangesync.OpenProbe {
		t.Helper()
		if ok := g.Allowed(p, at); ok != wantOK {
			t.Fatalf("allowed at %v: %v, want %v", at.Sub(now), ok, wantOK)
		}
		probe, ok := g.Begin(p, at)
		if probe.IsProbe() != wantProbe || ok != wantOK {
			t.Fatalf("begin at %v: probe %v ok %v, want %v %v", at.Sub(now), probe.IsProbe(), ok, wantProbe, wantOK)
		}
		return probe
	}

	// two opens in flight fail: one backoff of base
	o1 := begin(now, false, true)
	o2 := begin(now, false, true)
	g.End(p, now, rangesync.OpenFailed, o1)
	g.End(p, now, rangesync.OpenFailed, o2)
	begin(now.Add(base-1), false, false)
	// the probe; a second open waits for it
	probe := begin(now.Add(base), true, true)
	begin(now.Add(base), false, false)
	// an aborted probe leaves the backoff
	g.End(p, now.Add(base), rangesync.OpenAborted, probe)
	probe = begin(now.Add(base), true, true)
	// a failed probe doubles it, up to the maximum
	at := now.Add(base)
	for _, wait := range []time.Duration{2 * base, 4 * base, maxBackoff, maxBackoff} {
		g.End(p, at, rangesync.OpenFailed, probe)
		begin(at.Add(wait-1), false, false)
		at = at.Add(wait)
		probe = begin(at, true, true)
	}
	// a success ends it
	g.End(p, at, rangesync.OpenSucceeded, probe)
	o1 = begin(at, false, true)
	g.End(p, at, rangesync.OpenFailed, o1)
	begin(at, false, false)
	g.Clear(p)
	begin(at, false, true)

	// a probe that ends after Clear and a newer backoff
	at = at.Add(time.Hour)
	o1 = begin(at, false, true)
	g.End(p, at, rangesync.OpenFailed, o1)
	stale := begin(at.Add(base), true, true)
	g.Clear(p) // a disconnect
	o1 = begin(at.Add(base), false, true)
	g.End(p, at.Add(base), rangesync.OpenFailed, o1) // after the reconnect
	at = at.Add(2 * base)
	probe = begin(at, true, true)
	for _, outcome := range []rangesync.OpenOutcome{rangesync.OpenAborted, rangesync.OpenFailed} {
		g.End(p, at, outcome, stale)
		begin(at, false, false) // the newer probe still runs alone
	}
	g.End(p, at, rangesync.OpenFailed, probe)
	begin(at.Add(2*base-1), false, false)
	begin(at.Add(2*base), true, true) // doubled once, by the newer probe only
}

// TestFetchInboundFullNotDisconnected is the honest-safety check of failed
// opens. A peer whose inbound streams stay full for minutes (its system-wide
// inbound streams, which one third-party connection can fill, as there is no
// per-peer cap) and that holds each fetch stream open for the whole fetch
// timeout fails every open of its neighbors, the fetches queued behind the
// stream slots included. No neighbor scores or disconnects it. Each one's
// opens back off on its clock: one probe per backoff, doubling, and the
// fetches in between fail at once. Each neighbor fetches from the peer once
// its streams free up and the backoff ends.
func TestFetchInboundFullNotDisconnected(t *testing.T) {
	t.Parallel()

	const (
		neighbors = 3
		span      = 5 * time.Minute
		step      = 5 * time.Second
		burst     = 3 * rangesync.MaxFetchStreamsOut // queued behind the slots
		// probes after 0, 5, 15, 35, 75 and 155 s (rangesync.OpenBackoffBase
		// doubling), and the second slot's open at the start
		maxOpens = 7
	)
	chs := validChunks(neighbors)
	b := newTestNode(t, chs...)
	clock := newFakeClock()
	as := make([]*testNode, neighbors)
	stalls := make([]*stallOpens, neighbors)
	for i := range as {
		stalls[i] = newStallOpens()
		as[i] = clock.set(newTestNodeOpts(t, nodeOpts{streamer: stalls[i].wrap, fetchTimeout: 100 * time.Millisecond}))
		connect0(as[i], b)
		sendRoot(t, b, as[i], whole) // b's recorder now opens streams to as[i]
	}
	for elapsed := time.Duration(0); elapsed < span; elapsed += step {
		var wg sync.WaitGroup
		for _, a := range as {
			for range burst {
				wg.Go(func() {
					ids := missingIDs(t, 2, whole)
					if _, err := a.svc.FetchWithResult(context.Background(), b.addr, ids); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
						t.Errorf("got %v, want an unscored failed open", err)
					}
				})
			}
		}
		wg.Wait()
		if t.Failed() {
			t.FailNow()
		}
		clock.Add(step)
	}
	for i, a := range as {
		assertNotScored(t, a, b.addr)
		if got := stalls[i].opens.Load(); got == 0 || got > maxOpens {
			t.Fatalf("neighbor %d opened %d streams in %s, want 1 to %d", i, got, span, maxOpens)
		}
		stalls[i].stall.Store(false) // the streams free up
	}
	clock.Add(rangesync.MaxOpenBackoff)
	for i, a := range as {
		ids := idsOf(t, chs[i])
		if res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids); err != nil || res.Stored != 1 {
			t.Fatalf("neighbor %d after the streams freed up: stored %d, %v", i, res.Stored, err)
		}
		assertNoClaims(t, a, ids...)
	}
}

// TestFetchInterval (T6.5 part 2 fix round 1): the node-wide fetch limiter's
// interval is time.Second over Options.FetchRate, DefaultFetchRate's for zero
// or less, and never 0: a rate above MaxFetchRate is capped to it, where
// above 1e9 the interval was 0, which the limiter reads as no limit.
func TestFetchInterval(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		rate int
		want time.Duration
	}{
		{rate: 0, want: time.Millisecond},
		{rate: -1, want: time.Millisecond},
		{rate: rangesync.DefaultFetchRate, want: time.Millisecond},
		{rate: 1, want: time.Second},
		{rate: 250, want: 4 * time.Millisecond},
		{rate: rangesync.MaxFetchRate, want: time.Microsecond},
		{rate: rangesync.MaxFetchRate + 1, want: time.Microsecond},
		{rate: 1 << 20, want: time.Microsecond}, // the sims' lifted limit
		{rate: 2_000_000_000, want: time.Microsecond},
		{rate: math.MaxInt, want: time.Microsecond},
	} {
		if got := rangesync.FetchInterval(tc.rate); got != tc.want {
			t.Errorf("rate %d: interval %v, want %v", tc.rate, got, tc.want)
		}
	}
}

// TestFetchLimiterWaitAfterOpen checks the order of a Want's waits: its
// stream opens first, then it waits for the fetch rate with the stream open
// and no claim held, and the fetch timeout starts with the claims. The
// peer's handler waits for the Want meanwhile (it has no deadline to read
// it), and serves it once it arrives, however long after the open.
func TestFetchLimiterWaitAfterOpen(t *testing.T) {
	t.Parallel()

	opened := make(chan struct{}, 1)
	opens := func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
		return openSignal{StreamerDisconnecter: s, opened: opened}
	}
	chs := validChunks(1)
	// a token per 500 ms once the burst is gone: the wait outlasts the timeout
	a := newTestNodeOpts(t, nodeOpts{streamer: opens, fetchRate: 2, fetchTimeout: 100 * time.Millisecond})
	b := newTestNode(t, chs...)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	if !a.svc.DrainFetchLimiter() {
		t.Fatal("fetch limiter burst not taken")
	}
	ids := idsOf(t, chs...)
	type result struct {
		res rangesync.FetchResult
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
		done <- result{res, err}
	}()
	<-opened
	if l := a.svc.WantSet().Len(); l != 0 {
		t.Fatalf("%d claims held when the stream opened, want none until the fetch rate", l)
	}
	r := <-done
	if r.err != nil || r.res.Stored != 1 {
		t.Fatalf("stored %d, %v", r.res.Stored, r.err)
	}
	assertNotScored(t, a, b.addr)
	records, err := a.rec.Records(b.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if err := r.Err(); err != nil {
			t.Fatalf("peer's fetch handler: %v", err)
		}
	}
}

// openSignal is a streamer that signals each fetch stream it opened.
type openSignal struct {
	p2p.StreamerDisconnecter
	opened chan struct{}
}

func (o openSignal) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	s, err := o.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
	if err == nil && stream == rangesync.FetchStreamName {
		select {
		case o.opened <- struct{}{}:
		default:
		}
	}
	return s, err
}

// heldOpens is a streamer whose fetch stream opens wait while hold is set:
// until release is closed, then they fail with errOpenFailed (a transient
// failure on the peer's side), or until their context ends, then they fail
// with its error (as libp2p's headers exchange does). entered receives a
// value for each open that waits; maxWaiting is the most that waited at once.
type heldOpens struct {
	p2p.StreamerDisconnecter
	hold                atomic.Bool
	entered             chan struct{}
	release             chan struct{}
	waiting, maxWaiting atomic.Int32
}

func newHeldOpens() *heldOpens {
	return &heldOpens{entered: make(chan struct{}, 64), release: make(chan struct{})}
}

// wrap is a nodeOpts streamer of h.
func (h *heldOpens) wrap(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
	h.StreamerDisconnecter = s
	return h
}

func (h *heldOpens) NewStream(ctx context.Context, addr swarm.Address, hd p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	if stream != rangesync.FetchStreamName || !h.hold.Load() {
		return h.StreamerDisconnecter.NewStream(ctx, addr, hd, protocol, version, stream)
	}
	n := h.waiting.Add(1)
	defer h.waiting.Add(-1)
	for m := h.maxWaiting.Load(); n > m && !h.maxWaiting.CompareAndSwap(m, n); m = h.maxWaiting.Load() {
	}
	h.entered <- struct{}{}
	select {
	case <-h.release:
		return nil, errOpenFailed
	case <-ctx.Done():
		return nil, fmt.Errorf("send headers: %w", ctx.Err())
	}
}

// TestFetchDisconnectClearsOpenBackoff (T4.5 part 2c fix round 2) checks
// that a disconnect clears the backoff of the peer's fetch stream opens
// through the service's disconnect hook: after a failed open the peer's
// fetches fail at once while the backoff runs, but once it disconnected and
// reconnected its next fetch opens a stream at once. FetchOpensBackOff
// reports the backoff.
func TestFetchDisconnectClearsOpenBackoff(t *testing.T) {
	t.Parallel()

	clock := newFakeClock()
	h := newHeldOpens()
	h.hold.Store(true)
	close(h.release) // every held open fails at once
	a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: h.wrap}))
	chs := validChunks(1)
	b := newTestNode(t, chs...)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	backsOff := func(want bool) {
		t.Helper()
		if got := a.svc.FetchOpensBackOff(b.addr); got != want {
			t.Fatalf("opens back off %v, want %v", got, want)
		}
	}

	backsOff(false)
	if _, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...)); err == nil || errors.Is(err, rangesync.ErrOpenBackoff) {
		t.Fatalf("got %v, want a failed open", err)
	}
	backsOff(true)
	h.hold.Store(false) // the peer's opens would succeed now
	if _, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...)); !errors.Is(err, rangesync.ErrOpenBackoff) {
		t.Fatalf("got %v, want %v", err, rangesync.ErrOpenBackoff)
	}
	if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
		t.Fatal(err)
	}
	sendRoot(t, b, a, whole) // the reconnect, within the backoff
	backsOff(false)
	if res, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...)); err != nil || res.Stored != 1 {
		t.Fatalf("after the reconnect: stored %d, %v", res.Stored, err)
	}
	assertNotScored(t, a, b.addr)
}

// TestFetchStreamOpenHonest checks that stream opens that fail for no fault
// of the peer are not charged (no failed open is, TestFetchStreamOpenNotCharged):
//   - an open that finds the peer gone (p2p.ErrPeerNotFound), whose Status
//     the disconnect hook has not cleared yet;
//   - an open this node ends (its context, Close);
//   - a transient failure fails at most MaxFetchStreamsOut opens at once (the
//     fetches queued behind them hold no stream slot and open after it).
func TestFetchStreamOpenHonest(t *testing.T) {
	t.Parallel()

	t.Run("the peer is gone", func(t *testing.T) {
		t.Parallel()

		var openErr atomic.Pointer[error]
		gone := fmt.Errorf("new stream: %w", p2p.ErrPeerNotFound)
		openErr.Store(&gone)
		a := newTestNodeOpts(t, nodeOpts{streamErr: func(_ swarm.Address, _, _, stream string) error {
			if stream == rangesync.FetchStreamName {
				return *openErr.Load()
			}
			return nil
		}})
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		fail := func() {
			t.Helper()
			if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) || errors.Is(err, rangesync.ErrOpenBackoff) {
				t.Fatalf("got %v, want a failed open, not backed off", err)
			}
		}
		for range 2 * rangesync.MaxFetchResets {
			fail() // a peer that is gone does not back off the opens
		}
		openErr.Store(&errOpenFailed)
		fail()
		assertNotScored(t, a, b.addr)
	})

	t.Run("a peer that disconnects while its open fails", func(t *testing.T) {
		t.Parallel()

		// the disconnect hook clears the backoff before the failure is
		// recorded, which must not back off the reconnected peer's opens
		h := newHeldOpens()
		h.hold.Store(true)
		a := newTestNodeOpts(t, nodeOpts{streamer: h.wrap})
		chs := validChunks(1)
		b := newTestNode(t, chs...)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		done := make(chan error, 1)
		go func() {
			_, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole))
			done <- err
		}()
		<-h.entered
		if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
			t.Fatal(err)
		}
		h.hold.Store(false)
		close(h.release)
		if err := <-done; err == nil || errors.Is(err, rangesync.ErrOpenBackoff) {
			t.Fatalf("got %v, want a failed open", err)
		}
		sendRoot(t, b, a, whole) // the reconnect
		if res, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs...)); err != nil || res.Stored != 1 {
			t.Fatalf("after the reconnect: stored %d, %v", res.Stored, err)
		}
		assertNotScored(t, a, b.addr)
	})

	t.Run("this node's cancellations", func(t *testing.T) {
		t.Parallel()

		h := newHeldOpens()
		h.hold.Store(true)
		a := newTestNodeOpts(t, nodeOpts{streamer: h.wrap})
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		for range rangesync.MaxFetchResets + 2 {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				_, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, 1, whole))
				done <- err
			}()
			// an open this node canceled does not back off the next one
			select {
			case <-h.entered:
			case err := <-done:
				t.Fatalf("fetch did not open: %v", err)
			}
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want %v", err, context.Canceled)
			}
		}
		assertNotScored(t, a, b.addr)

		done := make(chan error, 1)
		go func() {
			_, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole))
			done <- err
		}()
		<-h.entered
		if err := a.svc.Close(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want a canceled open", err)
		}
		assertNotScored(t, a, b.addr)
	})

	t.Run("transient failures", func(t *testing.T) {
		t.Parallel()

		// a transient failure fails the opens in flight, at most
		// MaxFetchStreamsOut, and backs off the peer's opens for
		// OpenBackoffBase, unscored: the fetches queued behind them fail at
		// once without a claim, and the fetches after the backoff succeed
		const fetches = 3 * rangesync.MaxFetchStreamsOut
		clock := newFakeClock()
		h := newHeldOpens()
		a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: h.wrap}))
		chs := validChunks(fetches + 2)
		b := newTestNode(t, chs...)
		connect0(a, b)
		sendRoot(t, b, a, whole)

		h.hold.Store(true)
		errs := make(chan error, fetches)
		for _, ch := range chs[:fetches] {
			go func() {
				_, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, ch))
				errs <- err
			}()
		}
		for range rangesync.MaxFetchStreamsOut {
			<-h.entered
		}
		h.hold.Store(false) // the opens after the failure succeed
		close(h.release)
		failed, backedOff := 0, 0
		for range fetches {
			err := <-errs
			switch {
			case err == nil || errors.Is(err, rangesync.ErrInvalidMessage):
				t.Fatalf("got %v, want an unscored failure", err)
			case errors.Is(err, rangesync.ErrOpenBackoff):
				backedOff++
			default:
				failed++
			}
		}
		if failed != rangesync.MaxFetchStreamsOut || h.maxWaiting.Load() != rangesync.MaxFetchStreamsOut || backedOff != fetches-rangesync.MaxFetchStreamsOut {
			t.Fatalf("%d opens failed, %d at once, %d fetches backed off", failed, h.maxWaiting.Load(), backedOff)
		}
		assertNoClaims(t, a, idsOf(t, chs[:fetches]...)...)

		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, chs[fetches])); !errors.Is(err, rangesync.ErrOpenBackoff) {
			t.Fatalf("during the backoff: got %v, want %v", err, rangesync.ErrOpenBackoff)
		}
		clock.Add(rangesync.OpenBackoffBase)
		for _, ch := range chs[fetches:] {
			if res, err := a.svc.FetchWithResult(context.Background(), b.addr, idsOf(t, ch)); err != nil || res.Stored != 1 {
				t.Fatalf("after the backoff: stored %d, %v", res.Stored, err)
			}
		}
		assertNotScored(t, a, b.addr)
	})
}

// hookClock is a fakeClock that runs a hook once, at a later reading. The
// hook stands in for the disconnect hook landing between a rule's look at
// the peer's Status and its record: the rules read the clock in between.
type hookClock struct {
	*fakeClock
	mu   sync.Mutex
	skip int
	hook func()
}

func newHookClock() *hookClock { return &hookClock{fakeClock: newFakeClock()} }

// at arms hook to run at the reading after the next skip readings.
func (c *hookClock) at(skip int, hook func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skip, c.hook = skip, hook
}

func (c *hookClock) Now() time.Time {
	c.mu.Lock()
	var run func()
	if c.hook != nil {
		if c.skip == 0 {
			run, c.hook = c.hook, nil
		} else {
			c.skip--
		}
	}
	c.mu.Unlock()
	if run != nil {
		run()
	}
	return c.fakeClock.Now()
}

// set makes c the clock of n's shortfall rules and returns n.
func (c *hookClock) set(n *testNode) *testNode {
	n.svc.SetNow(c.Now)
	return n
}

// TestFetchForgetIfGone checks that what the rules record for a peer while
// it disconnects is forgotten: a disconnect that lands after their look at
// the peer's Status, so after the disconnect hook cleared the peer's
// entries, must not leave a reset charge, missing IDs or a violation mark
// for the next connection.
func TestFetchForgetIfGone(t *testing.T) {
	t.Parallel()

	disconnecter := func(a, b **testNode) func() {
		return func() {
			if err := (*a).svc.Protocol().DisconnectIn(p2p.Peer{Address: (*b).addr}); err != nil {
				t.Error(err)
			}
		}
	}
	resetsAfterReconnect := func(t *testing.T, a, b *testNode, off *atomic.Bool) {
		t.Helper()
		sendRoot(t, b, a, whole) // the reconnect
		off.Store(false)
		for i := range rangesync.MaxFetchResets {
			if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Fatalf("reset %d after the reconnect: got %v, want an unscored reset", i+1, err)
			}
		}
	}

	t.Run("reset charge", func(t *testing.T) {
		t.Parallel()

		var (
			a, b  *testNode
			off   atomic.Bool
			armed atomic.Bool
		)
		clock := newHookClock()
		armed.Store(true)
		a = clock.set(newTestNodeOpts(t, nodeOpts{streamer: resetStreams(func() {
			if armed.CompareAndSwap(true, false) {
				clock.at(0, disconnecter(&a, &b)) // in chargeReset
			}
		}, &off)}))
		b = newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want a reset", err)
		}
		resetsAfterReconnect(t, a, b, &off)
		assertNotScored(t, a, b.addr)
	})

	t.Run("missing ids", func(t *testing.T) {
		t.Parallel()

		var a, b *testNode
		clock := newHookClock()
		a = clock.set(newTestNode(t))
		b = newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		missing := missingIDs(t, 5, whole)
		// the fetch reads the clock at its start, in the judgment after its
		// Want and in its end judgment, which records the missing IDs
		clock.at(2, disconnecter(&a, &b))
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missing); err != nil {
			t.Fatal(err)
		}
		sendRoot(t, b, a, whole) // the reconnect
		// missing for the first time on this connection: not counted
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missing); err != nil {
			t.Fatalf("got %v, want the ids not counted", err)
		}
		assertNotScored(t, a, b.addr)
	})

	t.Run("violation mark", func(t *testing.T) {
		t.Parallel()

		var (
			a, b *testNode
			off  atomic.Bool
		)
		off.Store(true)
		clock := newHookClock()
		a = clock.set(newTestNodeOpts(t, nodeOpts{streamer: resetStreams(nil, &off)}))
		// the fetch credit (credit.go) would refuse the fetches after the
		// blatant one: the marks are tested on their own here
		a.svc.DisableFetchCredit()
		b = newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		// a blatant shortfall: the fetch reads the clock at its start, in the
		// judgment after its Want and when it marks the violation
		clock.at(2, disconnecter(&a, &b))
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, rangesync.MaxWantIDs, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		resetsAfterReconnect(t, a, b, &off)
		// the mark did not outlive the connection: past the budget is a
		// violation again
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("reset past the budget: got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		if fails, _ := a.scorer.get(b.addr); len(fails) != 2 {
			t.Fatalf("scorer fails %v, want 2", fails)
		}
	})
}

// TestViolationMarks checks the marks of violations: a peer without a
// Status (the disconnect hook cleared it) is not marked, so that a mark
// cannot outlive its connection and exempt the next one from the rules for
// ViolationGrace, and a peer already marked is not reported again, on
// either side: one violation, one score.
func TestViolationMarks(t *testing.T) {
	t.Parallel()

	unsolicited := func(t *testing.T, a, b *testNode) {
		t.Helper()
		fakeHolder(a, b, func(_ *pb.Want, w protobuf.Writer) {
			_ = w.WriteMsg(delivery(t, validChunks(1)[0]))
		})
	}

	t.Run("no mark without a status", func(t *testing.T) {
		t.Parallel()

		var off atomic.Bool
		off.Store(true)
		clock := newFakeClock()
		a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: resetStreams(nil, &off)}))
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
			t.Fatal(err)
		}
		// a violation after the disconnect hook ran, as a second stalled Want
		unsolicited(t, a, b)
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		connect0(a, b)
		sendRoot(t, b, a, whole) // the reconnect
		off.Store(false)
		for i := range rangesync.MaxFetchResets {
			if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Fatalf("reset %d: got %v, want an unscored reset", i+1, err)
			}
		}
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("reset past the budget: got %v, want %v (the peer is not exempt)", err, rangesync.ErrInvalidMessage)
		}
		if fails, _ := a.scorer.get(b.addr); len(fails) != 2 {
			t.Fatalf("scorer fails %v, want 2", fails)
		}
	})

	t.Run("one score per violation", func(t *testing.T) {
		t.Parallel()

		a := newTestNode(t)
		// the fetch credit (credit.go) would refuse the fetch after the
		// blatant one: the marks are tested on their own here
		a.svc.DisableFetchCredit()
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		// a blatant shortfall, then an unsolicited delivery while the peer is
		// being disconnected (it stays connected in this test)
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, rangesync.MaxWantIDs, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		unsolicited(t, a, b)
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
		assertFailed(t, a.scorer, b.addr)

		// the responder side: a marked peer's invalid Want still disconnects
		// it, unscored; an unmarked one's is scored
		p := newRawPeerIn(t, a, whole)
		if err := p.sendStatus(t, rangesync.EncodeStatus(rangesync.Status{Root: whole, LeafBits: serviceLeafBits, Mapping: fingerprint.DefaultMapperID})); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			s := p.open(t, rangesync.FetchStreamName)
			if err := protobuf.NewWriter(s).WriteMsg(&pb.Want{}); err != nil {
				t.Fatal(err)
			}
			_ = s.FullClose()
			records := p.records(t, rangesync.FetchStreamName)
			assertDisconnect(t, records[len(records)-1].Err())
		}
		assertFailed(t, a.scorer, p.addr)
	})

	// the same peer breaks the protocol on both sides of a: as the holder
	// of a fetch of a, and as the requester of a fetch a serves
	invalidWant := func(t *testing.T, a, b *testNode) {
		t.Helper()
		s, err := b.rec.NewStream(context.Background(), a.addr, nil, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
		if err != nil {
			t.Fatal(err)
		}
		if err := protobuf.NewWriter(s).WriteMsg(&pb.Want{}); err != nil {
			t.Fatal(err)
		}
		_ = s.FullClose()
		records, err := b.rec.Records(a.addr, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName)
		if err != nil {
			t.Fatal(err)
		}
		assertDisconnect(t, records[len(records)-1].Err())
	}
	// an unsolicited delivery: a marked peer is not judged by the shortfall
	// rules, but a protocol violation still disconnects it
	unsolicitedFetch := func(t *testing.T, a, b *testNode) {
		t.Helper()
		unsolicited(t, a, b)
		if _, err := a.svc.FetchWithResult(context.Background(), b.addr, missingIDs(t, 1, whole)); !errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
		}
	}
	for _, tc := range []struct {
		name          string
		first, second func(t *testing.T, a, b *testNode)
	}{
		{"requester mark, then responder violation", unsolicitedFetch, invalidWant},
		{"responder mark, then requester violation", invalidWant, unsolicitedFetch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := newTestNode(t)
			b := newTestNode(t)
			connect0(a, b)
			sendRoot(t, b, a, whole)
			tc.first(t, a, b)
			tc.second(t, a, b) // b stays connected in this test
			assertFailed(t, a.scorer, b.addr)
		})
	}
}
