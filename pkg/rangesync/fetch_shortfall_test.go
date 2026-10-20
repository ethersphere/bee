// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"fmt"
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
// root move per peer is excused per RootExcuseInterval (the same move again
// too), a missing ID counts when the peer leaves it missing again within
// ShortfallWindow (at most MaxMissingPerPeer remembered), an offense repeats
// one within ShortfallWindow, and Clear forgets the missing IDs and
// offenses, not the excuse. It also checks which root moves are plausible.
func TestShortfallBook(t *testing.T) {
	t.Parallel()

	b := rangesync.NewShortfallBook()
	peer, other := swarm.RandAddress(t), swarm.RandAddress(t)
	now := time.Unix(1_000_000, 0)
	r0, r1, r2 := rangeOf(""), rangeOf("1"), rangeOf("11")

	if !b.Excuse(peer, r0, r1, now) || !b.Excuse(peer, r0, r1, now.Add(time.Minute)) {
		t.Fatal("first move, or the same again, not excused")
	}
	if b.Excuse(peer, r1, r2, now.Add(time.Minute)) {
		t.Fatal("a second move excused within the interval")
	}
	if !b.Excuse(other, r0, r1, now) {
		t.Fatal("another peer's move not excused")
	}
	if !b.Excuse(peer, r1, r2, now.Add(rangesync.RootExcuseInterval)) {
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
	if b.Excuse(peer, r2, rangeOf("111"), now.Add(rangesync.RootExcuseInterval+time.Minute)) {
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

// resetFetchStreamer opens fetch streams whose reads fail, as libp2p's do
// when the peer reset the stream.
type resetFetchStreamer struct{ p2p.StreamerDisconnecter }

func (s resetFetchStreamer) NewStream(ctx context.Context, addr swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	st, err := s.StreamerDisconnecter.NewStream(ctx, addr, h, protocol, version, stream)
	if err != nil || stream != rangesync.FetchStreamName {
		return st, err
	}
	return resetReadStream{st}, nil
}

type resetReadStream struct{ p2p.Stream }

func (resetReadStream) Read([]byte) (int, error) { return 0, errPeerReset }

// TestFetchResetBudget checks that fetch streams a peer resets without a
// delivery are charged to a budget per peer: MaxFetchResets of them are not
// scored (an honest peer resets a stream that fails on its side), the next
// one is a violation. A disconnect clears the budget.
func TestFetchResetBudget(t *testing.T) {
	t.Parallel()

	a := newTestNodeOpts(t, nodeOpts{streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
		return resetFetchStreamer{s}
	}})
	b := newTestNode(t)
	connect0(a, b)
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
	assertNotScored(t, a, b.addr)
	if err := fetch(); !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("reset past the budget: got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	assertFailed(t, a.scorer, b.addr)

	if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
		t.Fatal(err)
	}
	if err := fetch(); errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("reset after a reconnect: %v", err)
	}
}
