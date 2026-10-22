// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The per-peer fetch credit (T4.7 part 4, credit.go): a peer's share of the
// fetch rate refills only with the chunks it delivered, and the credit
// survives reconnects.

// wantedIDs returns the number of IDs a asked peer for, in all its Wants.
func wantedIDs(t *testing.T, a *testNode, peer swarm.Address) int {
	t.Helper()
	if _, err := a.rec.Records(peer, rangesync.ProtocolName, rangesync.ProtocolVersion, rangesync.FetchStreamName); errors.Is(err, streamtest.ErrRecordsNotFound) {
		return 0
	}
	n := 0
	for _, w := range sentWants(t, a, peer) {
		n += len(w.IDs)
	}
	return n
}

// requested returns the IDs a fetch requested from its peer, by its result:
// those it stored, those the peer delivered and this node rejected, and
// those it did not deliver (the IDs outside the peer's root were not
// requested). The IDs must be fresh: an ID rejected before is counted as
// rejected without a request.
func requested(res rangesync.FetchResult) int {
	return res.Stored + res.Rejected + len(res.Unverified) + len(res.Undelivered) - res.OutsideRoot
}

// countingOpens returns a nodeOpts streamer that counts the fetch stream
// opens in opens.
func countingOpens(opens *atomic.Int64) func(p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
	return func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
		return countOpens{StreamerDisconnecter: s, opens: opens}
	}
}

// unknownBatch is a stamp validator that knows no batch: the stamps of a
// batch this node has not seen, or of one made up.
func unknownBatch(swarm.Chunk) (swarm.Chunk, error) {
	return nil, fmt.Errorf("batchstore get: %w, %w", storage.ErrNotFound, postage.ErrNotFound)
}

// route lets a, created with routes (nodeOpts.routes), open streams to b
// next to its other peers, and b to a. It must be called before a opens
// streams.
func route(routes map[string]p2p.ProtocolSpec, a, b *testNode) {
	routes[b.addr.String()] = b.svc.Protocol()
	b.rec.SetProtocols(a.svc.Protocol())
}

// ones returns the range of the first n bits set.
func ones(n int) fingerprint.Range { return rangeOf(strings.Repeat("1", n)) }

// TestFetchCreditJunk checks that a peer that lists junk runs out of fetch
// credit on every path the shortfall rules leave unscored (the residual of
// T4.5 part 2b fix round 3): it is asked for CreditInitial IDs in all, then
// for nothing, without a stream open, fetch rate or claim, and its IDs are
// reported NoCredit; then for one ID per CreditProbeInterval. A failed open
// costs no credit. No path scores it.
func TestFetchCreditJunk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// spent checks that a's credit of b is spent: a fetch of ids from b
	// requests nothing and opens no stream, and after CreditProbeInterval
	// one ID is requested (a probe) and the rest refused.
	spent := func(t *testing.T, clock *fakeClock, a, b *testNode, opens *atomic.Int64, ids func() []storer.ChunkID) {
		t.Helper()
		if c := a.svc.FetchCredit(b.addr); c != 0 {
			t.Fatalf("credit %d left, want 0", c)
		}
		wanted, opened := wantedIDs(t, a, b.addr), opens.Load()
		next := ids()
		res, err := a.svc.FetchWithResult(ctx, b.addr, next)
		if !errors.Is(err, rangesync.ErrFetchCredit) {
			t.Fatalf("fetch with the credit spent: got %v, want %v", err, rangesync.ErrFetchCredit)
		}
		assertChunkIDs(t, res.NoCredit, next...)
		if got := wantedIDs(t, a, b.addr); got != wanted || opens.Load() != opened {
			t.Fatalf("fetch with the credit spent requested %d IDs and opened %d streams", got-wanted, opens.Load()-opened)
		}
		assertNoClaims(t, a)

		clock.Add(rangesync.CreditProbeInterval)
		next = ids()
		res, err = a.svc.FetchWithResult(ctx, b.addr, next)
		if got := wantedIDs(t, a, b.addr) - wanted; got != 1 {
			t.Fatalf("probe requested %d IDs, want 1 (%v)", got, err)
		}
		if errors.Is(err, rangesync.ErrFetchCredit) && len(res.NoCredit) != len(next)-1 {
			t.Fatalf("probe: %d IDs refused for the credit, want %d", len(res.NoCredit), len(next)-1)
		}
	}

	t.Run("small fresh junk", func(t *testing.T) {
		t.Parallel()

		clock := newFakeClock()
		var opens atomic.Int64
		a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: countingOpens(&opens)}))
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		// under blatantMinIDs per fetch, never listed again: unscored
		const per = rangesync.BlatantMinIDs - 1
		for i := 0; ; i++ {
			if i > rangesync.CreditInitial/per {
				t.Fatal("the credit was not spent")
			}
			before := wantedIDs(t, a, b.addr)
			res, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, per, whole))
			if got := wantedIDs(t, a, b.addr) - before; got+len(res.NoCredit) != per {
				t.Fatalf("fetch %d: requested %d and refused %d of %d", i, got, len(res.NoCredit), per)
			}
			if errors.Is(err, rangesync.ErrFetchCredit) {
				break
			}
			if err != nil {
				t.Fatalf("fetch %d: %v", i, err)
			}
		}
		if got := wantedIDs(t, a, b.addr); got != rangesync.CreditInitial {
			t.Fatalf("requested %d junk IDs, want %d", got, rangesync.CreditInitial)
		}
		spent(t, clock, a, b, &opens, func() []storer.ChunkID { return missingIDs(t, per, whole) })
		assertNotScored(t, a, b.addr)
	})

	t.Run("resets", func(t *testing.T) {
		t.Parallel()

		clock := newFakeClock()
		var opens atomic.Int64
		reset := resetStreams(nil, nil)
		a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: func(s p2p.StreamerDisconnecter) p2p.StreamerDisconnecter {
			return reset(countingOpens(&opens)(s))
		}}))
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		// a Want that the peer resets: one reset charged, not judged
		if _, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, rangesync.MaxWantIDs, whole)); err == nil || errors.Is(err, rangesync.ErrInvalidMessage) || errors.Is(err, rangesync.ErrFetchCredit) {
			t.Fatalf("got %v, want an unscored reset", err)
		}
		if got := wantedIDs(t, a, b.addr); got != rangesync.CreditInitial {
			t.Fatalf("requested %d junk IDs, want %d", got, rangesync.CreditInitial)
		}
		spent(t, clock, a, b, &opens, func() []storer.ChunkID { return missingIDs(t, rangesync.MaxWantIDs, whole) })
		// two resets charged of the budget's MaxFetchResets: unscored
		assertNotScored(t, a, b.addr)
	})

	t.Run("failed opens", func(t *testing.T) {
		t.Parallel()

		clock := newFakeClock()
		a := clock.set(newTestNodeOpts(t, nodeOpts{streamErr: fetchStreamErr(errOpenFailed, nil)}))
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		for i := range 5 {
			_, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, rangesync.MaxWantIDs, whole))
			if err == nil || errors.Is(err, rangesync.ErrFetchCredit) {
				t.Fatalf("open %d: got %v, want the open's error", i, err)
			}
			clock.Add(rangesync.MaxOpenBackoff)
		}
		// nothing was requested, so nothing was spent
		if c := a.svc.FetchCredit(b.addr); c != rangesync.CreditInitial {
			t.Fatalf("credit %d after failed opens, want %d", c, rangesync.CreditInitial)
		}
		assertNotScored(t, a, b.addr)
	})

	t.Run("unknown-batch junk", func(t *testing.T) {
		t.Parallel()

		clock := newFakeClock()
		var opens atomic.Int64
		a := clock.set(newTestNodeOpts(t, nodeOpts{validStamp: unknownBatch, streamer: countingOpens(&opens)}))
		// delivered and rejected (an unknown batch depends on the chain
		// state this node has seen): unscored. At most 16 deliveries per
		// Want: streamtest deadlocks past them.
		const per = 12
		chs, _ := signedChunks(t, (rangesync.CreditInitial/per+3)*per)
		b := newTestNode(t, chs...)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		next := func() []storer.ChunkID {
			ids := idsOf(t, chs[:per]...)
			chs = chs[per:]
			return ids
		}
		for i := 0; ; i++ {
			if i > rangesync.CreditInitial/per {
				t.Fatal("the credit was not spent")
			}
			res, err := a.svc.FetchWithResult(ctx, b.addr, next())
			if res.Rejected+len(res.NoCredit) != per {
				t.Fatalf("fetch %d: rejected %d, refused %d of %d", i, res.Rejected, len(res.NoCredit), per)
			}
			if errors.Is(err, rangesync.ErrFetchCredit) {
				break
			}
			if err != nil {
				t.Fatalf("fetch %d: %v", i, err)
			}
		}
		if got := wantedIDs(t, a, b.addr); got != rangesync.CreditInitial {
			t.Fatalf("requested %d junk IDs, want %d", got, rangesync.CreditInitial)
		}
		spent(t, clock, a, b, &opens, next)
		assertNotScored(t, a, b.addr)
	})

	t.Run("root moves", func(t *testing.T) {
		t.Parallel()

		// the peer moves its root by one bit during every fetch, once per
		// RootExcuseInterval, so that the junk it listed in the half it left
		// is excused: unscored
		clock := newFakeClock()
		var (
			opens atomic.Int64
			a, b  *testNode
			mu    sync.Mutex
			hook  func()
		)
		a = clock.set(newTestNodeOpts(t, nodeOpts{
			streamer: countingOpens(&opens),
			hasHook: func(swarm.Address) {
				mu.Lock()
				h := hook
				mu.Unlock()
				if h != nil {
					h()
				}
			},
		}))
		b = newTestNodeOpts(t, nodeOpts{addr: addrIn(t, ones(8))})
		connect0(a, b)
		sendRoot(t, b, a, whole)
		// moveDuring returns n junk IDs in the half of root that b leaves,
		// and has b move to the other half once a claimed them: the first
		// look-up of the reserve after the n of the candidates.
		moveDuring := func(depth, n int) []storer.ChunkID {
			calls := 0
			mu.Lock()
			hook = func() {
				if calls++; calls == n+1 {
					sendRoot(t, b, a, ones(depth+1))
				}
			}
			mu.Unlock()
			return missingIDs(t, n, rangeOf(strings.Repeat("1", depth)+"0"))
		}
		res, err := a.svc.FetchWithResult(ctx, b.addr, moveDuring(0, rangesync.MaxWantIDs))
		if err != nil || len(res.Undelivered) != rangesync.MaxWantIDs {
			t.Fatalf("excused fetch: %+v, %v", res, err)
		}
		assertNotScored(t, a, b.addr)
		if c := a.svc.FetchCredit(b.addr); c != 0 {
			t.Fatalf("credit %d left, want 0", c)
		}
		// the next move, past the excuse interval: the probe, one ID, is
		// all that is requested; the rest left the root with the move
		clock.Add(rangesync.RootExcuseInterval)
		wanted := wantedIDs(t, a, b.addr)
		res, err = a.svc.FetchWithResult(ctx, b.addr, moveDuring(1, rangesync.MaxWantIDs))
		if err != nil || res.OutsideRoot != rangesync.MaxWantIDs-1 {
			t.Fatalf("excused fetch with the credit spent: %+v, %v", res, err)
		}
		if got := wantedIDs(t, a, b.addr) - wanted; got != 1 {
			t.Fatalf("excused fetch with the credit spent requested %d IDs, want the probe", got)
		}
		// and no more until the next probe
		opened := opens.Load()
		res, err = a.svc.FetchWithResult(ctx, b.addr, moveDuring(2, rangesync.MaxWantIDs))
		if !errors.Is(err, rangesync.ErrFetchCredit) || len(res.NoCredit) != rangesync.MaxWantIDs || opens.Load() != opened {
			t.Fatalf("fetch with the credit spent: %d refused, %d opens, %v", len(res.NoCredit), opens.Load()-opened, err)
		}
		assertNotScored(t, a, b.addr)
	})
}

// TestFetchCreditReconnect checks that the credit is kept by overlay, so that
// neither reconnects nor the book's bound renew it: a peer that reconnects
// after every fetch (which clears its reset budget, its shortfall memory and
// its open backoff) runs out of credit as one that stays, and so does one the
// book forgot, which resumes its debt.
func TestFetchCreditReconnect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	clock := newFakeClock()
	routes := make(map[string]p2p.ProtocolSpec)
	a := clock.set(newTestNodeOpts(t, nodeOpts{routes: routes}))
	const capacity = 4
	a.svc.SetFetchCreditCapacity(capacity)
	b := newTestNode(t)
	route(routes, a, b)
	sendRoot(t, b, a, whole)
	reconnect := func() {
		t.Helper()
		if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
			t.Fatal(err)
		}
		sendRoot(t, b, a, whole)
	}
	const per = rangesync.BlatantMinIDs - 1
	for i := 0; ; i++ {
		if i > rangesync.CreditInitial/per {
			t.Fatal("the credit was not spent")
		}
		_, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, per, whole))
		if errors.Is(err, rangesync.ErrFetchCredit) {
			break
		}
		if err != nil {
			t.Fatalf("fetch %d: %v", i, err)
		}
		reconnect()
	}
	if got := wantedIDs(t, a, b.addr); got != rangesync.CreditInitial {
		t.Fatalf("requested %d junk IDs over reconnects, want %d", got, rangesync.CreditInitial)
	}
	reconnect()
	if _, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, per, whole)); !errors.Is(err, rangesync.ErrFetchCredit) {
		t.Fatalf("after a reconnect: got %v, want %v", err, rangesync.ErrFetchCredit)
	}

	// capacity other peers asked for IDs make the book forget b
	others := make([]*testNode, capacity)
	held := validChunks(capacity)
	for i := range others {
		others[i] = newTestNode(t, held[i])
		route(routes, a, others[i])
	}
	for i, c := range others {
		ch := held[i : i+1]
		if n, err := a.svc.Fetch(ctx, c.addr, idsOf(t, ch...)); err != nil || n != 1 {
			t.Fatalf("honest peer: stored %d, %v", n, err)
		}
	}
	if a.svc.FetchCreditKnown(b.addr) {
		t.Fatal("the book keeps more overlays than its capacity")
	}
	reconnect()
	wanted := wantedIDs(t, a, b.addr)
	if _, err := a.svc.FetchWithResult(ctx, b.addr, missingIDs(t, per, whole)); !errors.Is(err, rangesync.ErrFetchCredit) {
		t.Fatalf("forgotten and back: got %v, want %v", err, rangesync.ErrFetchCredit)
	}
	if got := wantedIDs(t, a, b.addr); got != wanted {
		t.Fatalf("forgotten and back: requested %d IDs", got-wanted)
	}
	assertNotScored(t, a, b.addr)
}

// TestFetchCreditOneChunkThenReset is the one-chunk-then-reset pin (T4.5 part
// 2c round 2, Low 2): once per SyncRateWindow the peer lists a catch-up's
// worth of IDs, delivers one chunk this node lacks and resets the stream,
// within its reset budget, unscored. The credit bounds what that costs this
// node: the first fetch requests CreditInitial IDs, and from then on every
// fetch requests two (one stored, one not delivered; the stored chunk gives
// back the two units), not a full Want. It does not stop the stored chunk,
// which keeps a catch-up rate above 0, as an honest slow peer's would (the
// drip, T4.7 part 3): a Want of the IDs the credit covers, all delivered, is
// what an honest peer sends.
func TestFetchCreditOneChunkThenReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	clock := newFakeClock()
	a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: resetStreams(nil, nil)}))
	const cycles = 5
	chs := validChunks(cycles)
	b := newTestNode(t, chs...)
	connect0(a, b)
	sendRoot(t, b, a, whole)
	for i, ch := range chs {
		ids := append(idsOf(t, ch), missingIDs(t, rangesync.CatchUpMinDiff-1, whole)...)
		before := wantedIDs(t, a, b.addr)
		res, err := a.svc.FetchWithResult(ctx, b.addr, ids)
		if err == nil || errors.Is(err, rangesync.ErrInvalidMessage) || errors.Is(err, rangesync.ErrFetchCredit) {
			t.Fatalf("cycle %d: got %v, want an unscored reset", i, err)
		}
		if res.Stored != 1 {
			t.Fatalf("cycle %d: stored %d, want 1", i, res.Stored)
		}
		want := 2
		if i == 0 {
			want = rangesync.CreditInitial
		}
		if got := wantedIDs(t, a, b.addr) - before; got != want {
			t.Fatalf("cycle %d: requested %d IDs, want %d", i, got, want)
		}
		if c := a.svc.FetchCredit(b.addr); c != rangesync.CreditStoredRefund {
			t.Fatalf("cycle %d: credit %d, want %d", i, c, rangesync.CreditStoredRefund)
		}
		clock.Add(rangesync.SyncRateWindow)
	}
	assertNotScored(t, a, b.addr)
}

// TestFetchCreditNewPeer checks that a new honest peer can start fetching:
// with the pool's credit it gets a full Want at once, and with the pool
// drained by an attacker's new overlays it gets its first probe
// CreditProbeInterval after it was first asked, and then its credit doubles
// with each Want it delivers, within one fetch.
func TestFetchCreditNewPeer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// at most 16 deliveries per Want: streamtest deadlocks past them
	const n = 15
	sizes := func(t *testing.T, a, b *testNode) []int {
		t.Helper()
		wants := sentWants(t, a, b.addr)
		out := make([]int, 0, len(wants))
		for _, w := range wants {
			out = append(out, len(w.IDs))
		}
		return out
	}

	t.Run("pool", func(t *testing.T) {
		t.Parallel()
		chs := validChunks(n)
		a := newTestNode(t)
		b := newTestNode(t, chs...)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		if stored, err := a.svc.Fetch(ctx, b.addr, idsOf(t, chs...)); err != nil || stored != n {
			t.Fatalf("stored %d, %v", stored, err)
		}
		if got := sizes(t, a, b); len(got) != 1 {
			t.Fatalf("Wants of %v, want one", got)
		}
		if c := a.svc.FetchCredit(b.addr); c != rangesync.CreditInitial+n {
			t.Fatalf("credit %d, want %d", c, rangesync.CreditInitial+n)
		}
	})

	t.Run("pool drained", func(t *testing.T) {
		t.Parallel()
		clock := newFakeClock()
		chs := validChunks(n)
		a := clock.set(newTestNode(t))
		a.svc.DrainFetchCreditPool()
		b := newTestNode(t, chs...)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		ids := idsOf(t, chs...)
		res, err := a.svc.FetchWithResult(ctx, b.addr, ids)
		if !errors.Is(err, rangesync.ErrFetchCredit) || len(res.NoCredit) != n {
			t.Fatalf("new peer with the pool drained: %+v, %v", res, err)
		}
		clock.Add(rangesync.CreditProbeInterval)
		if stored, err := a.svc.Fetch(ctx, b.addr, ids); err != nil || stored != n {
			t.Fatalf("after the probe interval: stored %d, %v", stored, err)
		}
		// the probe, then doubling: 1, 2, 4, 8 units
		if got, want := sizes(t, a, b), []int{1, 2, 4, 8}; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("Wants of %v, want %v", got, want)
		}
		assertNotScored(t, a, b.addr)
	})
}

// TestFetchCreditHonestChurn checks that an honest peer with occasional
// misses and resets is never held up by its credit and scored no more than
// without it: over many fetches with an evicted ID every fourth fetch, half
// the IDs gone every 25th (a radius increase) and a reset after half the
// deliveries every tenth, every fetch requests all its IDs in one Want, no ID
// is refused for the credit, the same chunks are stored, and the peer is
// never scored, with the credit as without it.
func TestFetchCreditHonestChurn(t *testing.T) {
	t.Parallel()

	type outcome struct {
		wants, stored, refused, fails int
	}
	run := func(t *testing.T, credit bool) outcome {
		t.Helper()
		ctx := context.Background()
		const (
			fetches = 100
			per     = 12 // streamtest deadlocks past 16 deliveries per Want
		)
		clock := newFakeClock()
		var off atomic.Bool
		off.Store(true)
		a := clock.set(newTestNodeOpts(t, nodeOpts{streamer: resetStreams(nil, &off)}))
		if !credit {
			a.svc.DisableFetchCredit()
		}
		b := newTestNode(t)
		connect0(a, b)
		sendRoot(t, b, a, whole)
		var o outcome
		for i := 1; i <= fetches; i++ {
			chs := validChunks(per)
			held := chs
			switch {
			case i%25 == 0:
				held = chs[:per/2] // a radius increase: half evicted
			case i%4 == 0:
				held = chs[1:] // one evicted between the listing and the fetch
			}
			reset := i%10 == 0
			if reset {
				held = held[:len(held)/2] // a restart after half its deliveries
			}
			for _, ch := range held {
				if err := b.store.ReservePutter().Put(ctx, ch); err != nil {
					t.Fatal(err)
				}
			}
			off.Store(!reset)
			res, err := a.svc.FetchWithResult(ctx, b.addr, idsOf(t, chs...))
			if errors.Is(err, rangesync.ErrInvalidMessage) || errors.Is(err, rangesync.ErrFetchCredit) || err != nil && !reset {
				t.Fatalf("fetch %d: %v", i, err)
			}
			o.stored += res.Stored
			o.refused += len(res.NoCredit)
			clock.Add(30 * time.Second)
		}
		for _, w := range sentWants(t, a, b.addr) {
			if len(w.IDs) != per {
				t.Fatalf("a Want of %d IDs, want %d", len(w.IDs), per)
			}
			o.wants++
		}
		fails, _ := a.scorer.get(b.addr)
		o.fails = len(fails)
		if credit {
			// at the cap, but for the last fetch's reset
			if c := a.svc.FetchCredit(b.addr); c < rangesync.CreditMax-per {
				t.Errorf("honest peer's credit %d, want about %d", c, rangesync.CreditMax)
			}
		}
		return o
	}
	with, without := run(t, true), run(t, false)
	t.Logf("with the credit %+v, without %+v", with, without)
	if with != without {
		t.Fatalf("with the credit %+v, without %+v", with, without)
	}
	if with.fails != 0 || with.refused != 0 {
		t.Fatalf("honest peer scored %d times, %d IDs refused", with.fails, with.refused)
	}
}

// TestFetchCreditLimiterStress runs fetches from honest peers and from peers
// with no credit, all at once, behind a slow node-wide fetch limiter:
// nothing deadlocks, every honest chunk is stored, and the fetches from the
// peers with no credit are refused at once, without waiting for the limiter
// that the honest fetches queue on.
func TestFetchCreditLimiterStress(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	const (
		honest, liars = 3, 2
		calls, per    = 8, 12 // streamtest deadlocks past 16 deliveries per Want
	)
	// the limiter's burst covers MaxWantIDs: the rest of the honest fetches
	// wait about (honest × calls × per − MaxWantIDs) / 50 s, 0.8 s
	clock := newFakeClock() // no probe comes due
	routes := make(map[string]p2p.ProtocolSpec)
	a := clock.set(newTestNodeOpts(t, nodeOpts{fetchRate: 50, routes: routes}))
	type peer struct {
		n   *testNode
		ids [][]storer.ChunkID
	}
	peers := make([]peer, 0, honest)
	for range honest {
		chs := validChunks(calls*per + 1)
		p := peer{n: newTestNode(t, chs...)}
		route(routes, a, p.n)
		sendRoot(t, p.n, a, whole)
		// asked first, it draws its initial credit from the pool
		if n, err := a.svc.Fetch(ctx, p.n.addr, idsOf(t, chs[0])); err != nil || n != 1 {
			t.Fatalf("honest peer: stored %d, %v", n, err)
		}
		chs = chs[1:]
		for c := range calls {
			p.ids = append(p.ids, idsOf(t, chs[c*per:(c+1)*per]...))
		}
		peers = append(peers, p)
	}
	// the liars are asked after an attacker drained the pool: no credit
	a.svc.DrainFetchCreditPool()
	spent := make([]*testNode, 0, liars)
	for range liars {
		l := newTestNode(t)
		route(routes, a, l)
		sendRoot(t, l, a, whole)
		spent = append(spent, l)
	}
	var (
		wg      sync.WaitGroup
		stored  atomic.Int64
		slowest atomic.Int64
	)
	for _, p := range peers {
		for _, ids := range p.ids {
			wg.Go(func() {
				n, err := a.svc.Fetch(ctx, p.n.addr, ids)
				if err != nil {
					t.Errorf("honest fetch: %v", err)
				}
				stored.Add(int64(n))
			})
		}
	}
	for _, l := range spent {
		for range 2 * calls {
			wg.Go(func() {
				start := time.Now()
				_, err := a.svc.FetchWithResult(ctx, l.addr, missingIDs(t, per, whole))
				if !errors.Is(err, rangesync.ErrFetchCredit) {
					t.Errorf("fetch with no credit: got %v, want %v", err, rangesync.ErrFetchCredit)
				}
				for d := int64(time.Since(start)); ; {
					old := slowest.Load()
					if d <= old || slowest.CompareAndSwap(old, d) {
						break
					}
				}
			})
		}
	}
	wg.Wait()
	if got, want := stored.Load(), int64(honest*calls*per); got != want {
		t.Fatalf("stored %d honest chunks, want %d", got, want)
	}
	// a refusal takes no fetch rate
	if d := time.Duration(slowest.Load()); d > 400*time.Millisecond {
		t.Fatalf("a refused fetch took %v: it waited for the limiter", d)
	}
	for _, p := range peers {
		assertNotScored(t, a, p.n.addr)
	}
}

// TestFetchCreditBootstrapNotSlowed compares the scaled bootstrap of an empty
// node from two and from four peers (TestNeighborhoodBootstrapScaled's
// harness, with ten times its node-wide fetch rate) with the credit and
// without: the credit does not slow it.
func TestFetchCreditBootstrapNotSlowed(t *testing.T) {
	t.Parallel()

	for _, nodes := range []int{3, 5} {
		t.Run(fmt.Sprintf("%d peers", nodes-1), func(t *testing.T) {
			t.Parallel()
			var caught [2]time.Duration
			t.Cleanup(func() {
				t.Logf("caught up with the credit after %v, without after %v", caught[0], caught[1])
				if caught[0] == 0 || caught[1] == 0 {
					return // a run failed
				}
				if caught[0] > caught[1]+caught[1]/20+time.Second {
					t.Errorf("caught up with the credit after %v, without after %v", caught[0], caught[1])
				}
			})
			for i, name := range []string{"credit", "no credit"} {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					o := scaledBootstrap(8000, 7)
					o.Nodes = nodes
					o.FetchClockRate *= 10 // so that the peers' deliveries count, not only the node-wide rate
					o.NoFetchCredit = i == 1
					f := runBootstrap(t, o)
					t.Log(f)
					f.check(t)
					caught[i] = f.res.CaughtUp
				})
			}
		})
	}
}

// junkRun runs a peer b that lists junk to a on path for d of a's clock and
// returns the junk IDs a requested from b and whether b was scored. a
// fetches from b as often as the path lets the peer list: a served session
// per PeerSessionInterval and a round session per MinRoundInterval (small
// fresh junk, unknown-batch stamps), a Want reset within b's reset budget
// (resets), or a root move per RootExcuseInterval (root moves). With cycle,
// b reconnects after every fetch. With credit false, a runs without the
// fetch credit, as before T4.7 part 4.
func junkRun(t *testing.T, path string, credit, cycle bool, d time.Duration) (junk int, scored bool) {
	t.Helper()
	ctx := context.Background()
	clock := newFakeClock()
	var (
		off  atomic.Bool // set: fetch streams close as usual
		hook atomic.Pointer[func()]
	)
	off.Store(true)
	o := nodeOpts{
		fetchRate: 1 << 20,
		streamer:  resetStreams(nil, &off),
		hasHook: func(swarm.Address) {
			if h := hook.Load(); h != nil {
				(*h)()
			}
		},
	}
	every := peerFetchInterval()
	const perBatch = 12 // streamtest deadlocks past 16 deliveries per Want
	switch path {
	case "unknown batch":
		o.validStamp = unknownBatch
	case "resets":
		every = rangesync.PeerSessionInterval
	case "root moves":
		every = rangesync.RootExcuseInterval
	}
	a := clock.set(newTestNodeOpts(t, o))
	if !credit {
		a.svc.DisableFetchCredit()
	}
	b := newTestNodeOpts(t, nodeOpts{addr: addrIn(t, ones(16))})
	b.svc.DisableDeliveryLimit() // on the real clock
	connect0(a, b)
	depth := 0
	sendRoot(t, b, a, ones(depth))
	// the peer's view of its reset budget (a's fetchResets)
	budget := rate.NewLimiter(rate.Every(rangesync.FetchResetInterval), rangesync.MaxFetchResets)
	for start := clock.Now(); clock.Now().Sub(start) < d; clock.Add(every) {
		now := clock.Now()
		var ids []storer.ChunkID
		switch path {
		case "small":
			ids = missingIDs(t, rangesync.BlatantMinIDs-1, ones(depth))
		case "unknown batch":
			chs, _ := signedChunks(t, perBatch)
			for _, ch := range chs {
				if err := b.store.ReservePutter().Put(ctx, ch); err != nil {
					t.Fatal(err)
				}
			}
			ids = idsOf(t, chs...)
		case "resets":
			if !budget.AllowN(now, 1) {
				continue // past its budget the reset would be a violation
			}
			off.Store(false)
			ids = missingIDs(t, rangesync.MaxWantIDs, ones(depth))
		case "root moves":
			n := rangesync.MaxWantIDs
			ids = missingIDs(t, n, rangeOf(strings.Repeat("1", depth)+"0"))
			calls := 0
			moved := depth + 1
			f := func() {
				if calls++; calls == n+1 {
					sendRoot(t, b, a, ones(moved))
				}
			}
			hook.Store(&f)
		}
		res, err := a.svc.FetchWithResult(ctx, b.addr, ids)
		hook.Store(nil)
		off.Store(true)
		junk += requested(res) - res.Stored
		if errors.Is(err, rangesync.ErrInvalidMessage) {
			t.Fatalf("%s: the peer was found out: %v", path, err)
		}
		if path == "root moves" {
			if st, ok := a.svc.LastStatus(b.addr); ok && st.Root.Bits > uint8(depth) {
				depth = int(st.Root.Bits)
			}
		}
		if cycle {
			if err := a.svc.Protocol().DisconnectIn(p2p.Peer{Address: b.addr}); err != nil {
				t.Fatal(err)
			}
			sendRoot(t, b, a, ones(depth))
			budget = rate.NewLimiter(rate.Every(rangesync.FetchResetInterval), rangesync.MaxFetchResets)
		}
	}
	fails, _ := a.scorer.get(b.addr)
	return junk, len(fails) > 0
}

// peerFetchInterval is how often a peer can have this node fetch what it
// lists: a served session per PeerSessionInterval and a session of this
// node's rounds per MinRoundInterval, 0.6 fetches per second.
func peerFetchInterval() time.Duration {
	served, rounds := float64(rangesync.PeerSessionInterval), float64(rangesync.MinRoundInterval)
	return time.Duration(served * rounds / (served + rounds))
}

// TestFetchCreditJunkResidual measures the junk IDs a peer that lists junk
// has this node request, unscored, per path of the T4.5 part 2b fix round 3
// residual, without the credit (before T4.7 part 4) and with it, staying
// connected and reconnecting after every fetch, over 30 minutes. With the
// credit, every path is bounded by CreditInitial plus a probe per
// CreditProbeInterval (0.1 IDs/s), reconnects add nothing, and no path is
// scored. Failed opens request nothing either way (TestFetchCreditJunk).
func TestFetchCreditJunkResidual(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	t.Parallel()

	const d = 30 * time.Minute
	bound := rangesync.CreditInitial + int(d/rangesync.CreditProbeInterval) + 1
	for _, path := range []string{"small", "resets", "unknown batch", "root moves"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			for _, credit := range []bool{false, true} {
				for _, cycle := range []bool{false, true} {
					t.Run(fmt.Sprintf("credit=%v/reconnects=%v", credit, cycle), func(t *testing.T) {
						t.Parallel()
						junk, scored := junkRun(t, path, credit, cycle, d)
						t.Logf("%s, credit %v, reconnects %v: %d junk IDs in %v, %.2f IDs/s, scored %v", path, credit, cycle, junk, d, float64(junk)/d.Seconds(), scored)
						if scored {
							t.Error("the peer was scored")
						}
						if credit && junk > bound {
							t.Errorf("%d junk IDs, over %d", junk, bound)
						}
					})
				}
			}
		})
	}
}

// withTimestamp returns ch with its stamp's timestamp set to ts: another
// version of the same stamp index, older or newer.
func withTimestamp(ch swarm.Chunk, ts uint64) swarm.Chunk {
	st := ch.Stamp()
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, ts)
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(postage.NewStamp(st.BatchID(), st.Index(), b, st.Sig()))
}

// newerWins is a reserve put hook that keeps the newest version of a chunk,
// as the reserve keeps the newest stamp of a stamp index: a version older
// than the one stored fails with storage.ErrOverwriteNewerChunk.
func newerWins() func(swarm.Chunk) error {
	var (
		mu     sync.Mutex
		newest = make(map[string]uint64)
	)
	return func(ch swarm.Chunk) error {
		ts := binary.BigEndian.Uint64(ch.Stamp().Timestamp())
		k := ch.Address().ByteString()
		mu.Lock()
		defer mu.Unlock()
		if n, ok := newest[k]; ok && ts < n {
			return storage.ErrOverwriteNewerChunk
		}
		newest[k] = ts
		return nil
	}
}

// fetchAll fetches ids from peer, per IDs per call, and checks that every
// fetch ends without an error and with each ID stored or rejected, as
// rejected says.
func fetchAll(t *testing.T, a *testNode, peer swarm.Address, ids []storer.ChunkID, rejected bool) {
	t.Helper()
	const per = 12 // streamtest deadlocks past 16 deliveries per Want
	for i := 0; i < len(ids); i += per {
		next := ids[i:min(i+per, len(ids))]
		res, err := a.svc.FetchWithResult(context.Background(), peer, next)
		want := res.Stored
		if rejected {
			want = res.Rejected
		}
		if err != nil || want != len(next) {
			t.Fatalf("fetch %d of %d IDs (rejected %v): %+v, %v", i/per, len(next), rejected, res, err)
		}
	}
}

// TestFetchCreditStaleStamps is review Medium 1 of T4.7 part 4, the stale
// stamps of §7.7: a delivery older than the version the reserve holds
// (storage.ErrOverwriteNewerChunk) gave nothing back, so an attacker with one
// mutable batch, who uploads chunks to the neighborhood and delivers newer
// versions of them to this node alone through a Sybil, drained this node's
// honest neighbors with their older versions, one unit each, and this node
// then refused their real chunks. Such a delivery is now neutral: the honest
// neighbor keeps its credit over more older versions than its credit could
// pay for at the cap, and its real chunks are fetched as before. Nobody is
// scored.
func TestFetchCreditStaleStamps(t *testing.T) {
	t.Parallel()

	const pairs = rangesync.CreditMax + rangesync.CreditInitial + 50
	chs := validChunks(pairs + 12)
	older, newer := make([]swarm.Chunk, 0, len(chs)), make([]swarm.Chunk, 0, pairs)
	for _, ch := range chs[:pairs] {
		older, newer = append(older, withTimestamp(ch, 1)), append(newer, withTimestamp(ch, 2))
	}
	lacked := chs[pairs:] // chunks this node lacks
	routes := make(map[string]p2p.ProtocolSpec)
	a := newTestNodeOpts(t, nodeOpts{routes: routes, putHook: newerWins(), fetchRate: 1 << 20})
	sybil := newTestNode(t, newer...)
	honest := newTestNode(t, append(older, lacked...)...)
	for _, b := range []*testNode{sybil, honest} {
		b.svc.DisableDeliveryLimit() // on the real clock
		route(routes, a, b)
		sendRoot(t, b, a, whole)
	}
	// the Sybil delivers the newer versions to this node alone
	fetchAll(t, a, sybil.addr, idsOf(t, newer...), false)
	// the honest neighbor lists the older ones, which this node lacks: each
	// is delivered and rejected
	fetchAll(t, a, honest.addr, idsOf(t, older...), true)
	if c := a.svc.FetchCredit(honest.addr); c != rangesync.CreditInitial {
		t.Fatalf("honest neighbor's credit %d after %d older versions, want %d", c, pairs, rangesync.CreditInitial)
	}
	fetchAll(t, a, honest.addr, idsOf(t, lacked...), false)
	assertNotScored(t, a, honest.addr)
	assertNotScored(t, a, sybil.addr)
}

// TestFetchCreditChainState is review Medium 1 of T4.7 part 4, the stamps
// rejected for the chain state this node has seen (an unknown or expired
// batch, an index past the batch depth): they gave nothing back, so the
// honest neighbors that had not seen a batch expire yet lost a unit for each
// of its chunks they delivered, and with a large batch ran out. A delivery of
// an ID this node held itself, until the batch expired here first and its
// chunks were evicted, is now neutral, once per eviction; any other is
// neutral while the peer has an excuse left, one per chunk it stored.
func TestFetchCreditChainState(t *testing.T) {
	t.Parallel()

	t.Run("expired here first", func(t *testing.T) {
		t.Parallel()
		const n = rangesync.CreditMax + rangesync.CreditInitial + 50
		chs := validChunks(n)
		batch := make(map[string]bool, n)
		for _, ch := range chs {
			batch[ch.Address().ByteString()] = true
		}
		var expired atomic.Bool
		a := newTestNodeOpts(t, nodeOpts{fetchRate: 1 << 20, validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
			if expired.Load() && batch[ch.Address().ByteString()] {
				return unknownBatch(ch)
			}
			return ch, nil
		}}, chs...)
		// rejections expire at once, so that an ID can be delivered again
		a.svc.SetWantSet(wantset.New(rangesync.ClaimDeadline, time.Nanosecond))
		if err := a.svc.Start(); err != nil {
			t.Fatal(err)
		}
		b := newTestNode(t, chs...)  // it has not seen the batch expire yet
		b.svc.DisableDeliveryLimit() // on the real clock
		connect0(a, b)
		sendRoot(t, b, a, whole)

		// the batch expires here first: the reserve evicts its chunks
		expired.Store(true)
		ids := idsOf(t, chs...)
		for _, id := range ids {
			if err := a.store.RemoveID(id.Address, id.StampHash); err != nil {
				t.Fatal(err)
			}
		}
		// the neighbor still lists them: each is delivered and rejected
		fetchAll(t, a, b.addr, ids, true)
		if c := a.svc.FetchCredit(b.addr); c != rangesync.CreditInitial {
			t.Fatalf("neighbor's credit %d after %d chunks of a batch expired here first, want %d", c, n, rangesync.CreditInitial)
		}
		assertNotScored(t, a, b.addr)

		// each eviction excuses one delivery: delivered again, they cost
		// the peer, as any chain-state rejection of a peer with no excuse
		const per = 12
		fetchAll(t, a, b.addr, ids[:per], true)
		if c := a.svc.FetchCredit(b.addr); c != rangesync.CreditInitial-per {
			t.Fatalf("credit %d after the evicted IDs delivered again, want %d", c, rangesync.CreditInitial-per)
		}
	})

	t.Run("excuses", func(t *testing.T) {
		t.Parallel()
		// chunks of a batch diluted (an index past the depth this node
		// knows) or created (a batch it does not know yet) on a chain state
		// this node has not seen, which it never held
		const per = 12
		chs, skew := validChunks(per), validChunks(3*per)
		kind := make(map[string]int, len(skew))
		for i, ch := range skew {
			kind[ch.Address().ByteString()] = 1 + i%2
		}
		a := newTestNodeOpts(t, nodeOpts{validStamp: func(ch swarm.Chunk) (swarm.Chunk, error) {
			switch kind[ch.Address().ByteString()] {
			case 1:
				return nil, postage.ErrInvalidIndex
			case 2:
				return unknownBatch(ch)
			}
			return ch, nil
		}})
		b := newTestNode(t, append(chs, skew...)...)
		connect0(a, b)
		sendRoot(t, b, a, whole)

		// the peer delivers chunks this node lacks: each stored gives
		// CreditStoredRefund and an excuse
		fetchAll(t, a, b.addr, idsOf(t, chs...), false)
		credit := rangesync.CreditInitial + per*(rangesync.CreditStoredRefund-1)
		if c := a.svc.FetchCredit(b.addr); c != credit {
			t.Fatalf("credit %d, want %d", c, credit)
		}
		// the first per rejections are excused, the others cost a unit each
		fetchAll(t, a, b.addr, idsOf(t, skew...), true)
		if c, want := a.svc.FetchCredit(b.addr), credit-2*per; c != want {
			t.Fatalf("credit %d after %d chain-state rejections and %d excuses, want %d", c, len(skew), per, want)
		}
		assertNotScored(t, a, b.addr)
	})
}
