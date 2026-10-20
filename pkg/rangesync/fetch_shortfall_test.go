// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

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

// TestFetchShortfallScored checks §9 "bogus wants count against the peer":
// a peer whose fetch stream closes without IDs it was asked for (it listed
// them), or that delivers stamps invalid on every node (a signature that
// does not recover, here through postage.ValidStamp), is a violation once
// those exceed the tolerance (4 of a small fetch): it is reported to the
// PeerScorer once and disconnected, and its undelivered IDs are released
// for other peers. At the tolerance it is neither scored nor disconnected.
func TestFetchShortfallScored(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		missing, bad int
		scored       bool
	}{
		{name: "missing at the tolerance", missing: 4},
		{name: "missing above the tolerance", missing: 5, scored: true},
		{name: "bad signatures at the tolerance", bad: 4},
		{name: "bad signatures above the tolerance", bad: 5, scored: true},
		{name: "missing and bad signatures above the tolerance", missing: 3, bad: 2, scored: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			chs, validStamp := signedChunks(t, 2+tc.missing+tc.bad)
			held := chs[:2]
			missing := chs[2 : 2+tc.missing]
			var bad []swarm.Chunk
			for _, ch := range chs[2+tc.missing:] {
				bad = append(bad, badSignature(ch))
			}
			a := newTestNodeOpts(t, nodeOpts{validStamp: validStamp})
			b := newTestNode(t, append(append([]swarm.Chunk(nil), held...), bad...)...)
			connect0(a, b)

			ids := idsOf(t, append(append(append([]swarm.Chunk(nil), held...), missing...), bad...)...)
			res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
			if res.Stored != len(held) || res.Rejected != tc.bad {
				t.Fatalf("stored %d, rejected %d, want %d, %d", res.Stored, res.Rejected, len(held), tc.bad)
			}
			assertChunkIDs(t, res.Undelivered, idsOf(t, missing...)...)
			assertNoClaims(t, a)
			if !tc.scored {
				if err != nil {
					t.Fatal(err)
				}
				assertNotScored(t, a, b.addr)
				return
			}
			if !errors.Is(err, rangesync.ErrInvalidMessage) {
				t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
			}
			assertFailed(t, a.scorer, b.addr)
			if !a.rec.IsDisconnected(b.addr) {
				t.Fatal("peer not disconnected")
			}
		})
	}
}

// TestFetchShortfallHonestChurn checks that an honest peer under churn is
// never scored for what it does not deliver: a few IDs evicted between the
// listing and the fetch (within the tolerance), IDs outside its root after
// its radius grew (its Status shows the narrower root), IDs another fetch
// held (not requested), stamps this node could not check (Unverified), and
// rejections that depend on the chain state the two nodes have seen (an
// unknown or expired batch, an index past the batch depth this node knows)
// or on this node's reserve (an older stamp than the one it holds).
func TestFetchShortfallHonestChurn(t *testing.T) {
	t.Parallel()

	whole := fingerprint.NewRange(swarm.ZeroAddress, 0)
	upper := whole.Child(1, 1) // b's root after its radius grew
	lower := whole.Child(1, 0)
	evicted := chunksInRange(4, upper) // evicted from b's root: within the tolerance
	outside := chunksInRange(6, lower) // outside b's new root: evicted after the radius grew
	delivered := chunksInRange(2, upper)
	held := validChunks(6)       // claimed through another peer
	unverified := validChunks(4) // the batch store failed on this node
	unknown := validChunks(2)    // batch unknown or expired here
	index := validChunks(2)      // index past the depth this node knows
	older := validChunks(2)      // the reserve holds a newer stamp

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
	if err := b.svc.SendStatusWith(context.Background(), a.addr, rangesync.Status{Root: upper, LeafBits: serviceLeafBits, Mapping: fingerprint.DefaultMapperID}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "b's status", func() bool {
		st, ok := a.svc.LastStatus(b.addr)
		return ok && st.Root.Equal(upper)
	})
	other := swarm.RandAddress(t)
	for _, id := range idsOf(t, held...) {
		if r := a.svc.WantSet().Claim(fpID(id), other); r != wantset.Granted {
			t.Fatal(r)
		}
	}

	var all []storer.ChunkID
	for _, set := range [][]swarm.Chunk{evicted, outside, delivered, held, unverified, unknown, index, older} {
		all = append(all, idsOf(t, set...)...)
	}
	res, err := a.svc.FetchWithResult(context.Background(), b.addr, all)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stored != len(delivered) || res.Rejected != len(unknown)+len(index)+len(older) || len(res.Held) != len(held) || len(res.Unverified) != len(unverified) {
		t.Fatalf("%+v", res)
	}
	assertChunkIDs(t, res.Undelivered, idsOf(t, append(append([]swarm.Chunk(nil), evicted...), outside...)...)...)
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
	var deliveries []*pb.Delivery
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

	var ids []storer.ChunkID
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

// TestFetchShortfallStopsEarly checks that a fetch stops as soon as its
// peer's shortfall is above the tolerance of every ID it was passed: a peer
// that listed junk closes its first Want without a chunk, and the fetch is a
// violation then, without requesting the rest.
func TestFetchShortfallStopsEarly(t *testing.T) {
	t.Parallel()

	a := newTestNode(t)
	b := newTestNode(t)
	connect0(a, b)

	ids := idsOf(t, stampedChunks(t, 4*rangesync.MaxWantIDs)...)
	res, err := a.svc.FetchWithResult(context.Background(), b.addr, ids)
	if !errors.Is(err, rangesync.ErrInvalidMessage) {
		t.Fatalf("got %v, want %v", err, rangesync.ErrInvalidMessage)
	}
	if n := len(sentWants(t, a, b.addr)); n != 1 {
		t.Fatalf("%d wants, want 1", n)
	}
	assertChunkIDs(t, res.Undelivered, ids[:rangesync.MaxWantIDs]...)
	assertFailed(t, a.scorer, b.addr)
	assertNoClaims(t, a)
}
