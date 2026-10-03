// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

// putHarness bundles a fresh reserve with assertion helpers that inspect the
// index entries Put writes. Every helper fails the test on unexpected state so
// the cases below read as a plain sequence of puts and expectations.
type putHarness struct {
	t        *testing.T
	ctx      context.Context
	r        *reserve.Reserve
	ts       transaction.Storage
	baseAddr swarm.Address
}

func newPutHarness(t *testing.T) *putHarness {
	t.Helper()
	baseAddr := swarm.RandAddress(t)
	ts := internal.NewInmemStorage()
	r, err := reserve.New(baseAddr, ts, 0, kademlia.NewTopologyDriver(), log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	return &putHarness{t: t, ctx: context.Background(), r: r, ts: ts, baseAddr: baseAddr}
}

// put hands a clone to the reserve. The in-memory chunk store keeps the
// object it is given and Reserve.Get re-stamps the stored object in place, so
// putting the test's own chunk would let later reads rewrite its stamp.
func (h *putHarness) put(ch swarm.Chunk) {
	h.t.Helper()
	if err := h.r.Put(h.ctx, cloneChunks([]swarm.Chunk{ch})[0]); err != nil {
		h.t.Fatalf("put %s: %v", ch.Address(), err)
	}
}

func (h *putHarness) putErr(ch swarm.Chunk, want error) {
	h.t.Helper()
	err := h.r.Put(h.ctx, cloneChunks([]swarm.Chunk{ch})[0])
	if !errors.Is(err, want) {
		h.t.Fatalf("put %s: got %v, want %v", ch.Address(), err, want)
	}
}

func (h *putHarness) stampHash(ch swarm.Chunk) []byte {
	h.t.Helper()
	hash, err := ch.Stamp().Hash()
	if err != nil {
		h.t.Fatal(err)
	}
	return hash
}

// hasEntry reports whether the reserve holds an entry for the chunk's
// address under the chunk's own stamp.
func (h *putHarness) hasEntry(ch swarm.Chunk) bool {
	h.t.Helper()
	has, err := h.r.Has(ch.Address(), ch.Stamp().BatchID(), h.stampHash(ch))
	if err != nil {
		h.t.Fatal(err)
	}
	return has
}

func (h *putHarness) expectEntry(ch swarm.Chunk, present bool) {
	h.t.Helper()
	if got := h.hasEntry(ch); got != present {
		h.t.Fatalf("entry for %s under batch %x: present=%v, want %v", ch.Address(), ch.Stamp().BatchID()[:4], got, present)
	}
}

// entry loads the BatchRadiusItem and ChunkBinItem written for the chunk's
// address under the chunk's own stamp.
func (h *putHarness) entry(ch swarm.Chunk) (*reserve.BatchRadiusItem, *reserve.ChunkBinItem) {
	h.t.Helper()
	bin := swarm.Proximity(h.baseAddr.Bytes(), ch.Address().Bytes())
	bri := &reserve.BatchRadiusItem{Bin: bin, BatchID: ch.Stamp().BatchID(), Address: ch.Address(), StampHash: h.stampHash(ch)}
	if err := h.ts.IndexStore().Get(bri); err != nil {
		h.t.Fatalf("batch radius item for %s: %v", ch.Address(), err)
	}
	cbi := &reserve.ChunkBinItem{Bin: bin, BinID: bri.BinID}
	if err := h.ts.IndexStore().Get(cbi); err != nil {
		h.t.Fatalf("chunk bin item for %s bin id %d: %v", ch.Address(), bri.BinID, err)
	}
	return bri, cbi
}

// expectSumAgainst asserts that the entry stored for ch (its address and
// stamp) advertises the sum of the given payload, which is what the node
// will deliver for that entry, and that the sum index agrees.
func (h *putHarness) expectSumAgainst(ch, payload swarm.Chunk) {
	h.t.Helper()
	want, err := storage.ChunkSumFromParts(ch.Stamp().BatchID(), h.stampHash(ch), payload)
	if err != nil {
		h.t.Fatal(err)
	}
	_, cbi := h.entry(ch)
	if !bytes.Equal(cbi.Sum, want) {
		h.t.Fatalf("entry sum for %s under batch %x does not match the stored payload", ch.Address(), ch.Stamp().BatchID()[:4])
	}
	has, err := h.r.HasSum(ch.Address(), want)
	if err != nil {
		h.t.Fatal(err)
	}
	if !has {
		h.t.Fatalf("sum index missing entry for %s", ch.Address())
	}

	// what pullsync would deliver for this entry must recompute to the
	// advertised sum, otherwise the receiver rejects it as unsolicited.
	got, err := h.r.Get(h.ctx, ch.Address(), ch.Stamp().BatchID(), h.stampHash(ch))
	if err != nil {
		h.t.Fatal(err)
	}
	delivered, err := storage.ChunkSum(got)
	if err != nil {
		h.t.Fatal(err)
	}
	if !bytes.Equal(delivered, want) {
		h.t.Fatalf("delivered chunk for %s recomputes to a different sum than advertised", ch.Address())
	}
}

func (h *putHarness) expectNoSum(ch swarm.Chunk) {
	h.t.Helper()
	sum, err := storage.ChunkSum(ch)
	if err != nil {
		h.t.Fatal(err)
	}
	has, err := h.r.HasSum(ch.Address(), sum)
	if err != nil {
		h.t.Fatal(err)
	}
	if has {
		h.t.Fatalf("sum index unexpectedly holds the sum of a payload the node does not store at %s", ch.Address())
	}
}

func (h *putHarness) expectPayload(addr swarm.Address, want swarm.Chunk) {
	h.t.Helper()
	got, err := h.ts.ChunkStore().Get(h.ctx, addr)
	if err != nil {
		h.t.Fatalf("chunkstore get %s: %v", addr, err)
	}
	if !bytes.Equal(got.Data(), want.Data()) {
		h.t.Fatalf("chunkstore holds a different payload at %s than expected", addr)
	}
}

func (h *putHarness) expectNoPayload(addr swarm.Address) {
	h.t.Helper()
	_, err := h.ts.ChunkStore().Get(h.ctx, addr)
	if !errors.Is(err, storage.ErrNotFound) {
		h.t.Fatalf("chunkstore get %s: got %v, want %v", addr, err, storage.ErrNotFound)
	}
}

func (h *putHarness) expectSize(want int) {
	h.t.Helper()
	if got := h.r.Size(); got != want {
		h.t.Fatalf("reserve size: got %d, want %d", got, want)
	}
}

func (h *putHarness) expectSumCount(want int) {
	h.t.Helper()
	got, err := h.ts.IndexStore().Count(&reserve.ChunkSumItem{})
	if err != nil {
		h.t.Fatal(err)
	}
	if got != want {
		h.t.Fatalf("chunk sum items: got %d, want %d", got, want)
	}
}

func (h *putHarness) evictBatch(batchID []byte) {
	h.t.Helper()
	if _, err := h.r.EvictBatchBin(h.ctx, batchID, math.MaxInt, swarm.MaxBins); err != nil {
		h.t.Fatal(err)
	}
}

// stampsByHash returns the two stamps ordered so that the first has the
// lexicographically lower hash, which is the one the equal-timestamp
// tie-breaks favor.
func stampsByHash(t *testing.T, a, b *postage.Stamp) (low, high *postage.Stamp) {
	t.Helper()
	ha, err := a.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hb, err := b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(ha, hb) < 0 {
		return a, b
	}
	return b, a
}

// TestPutIdempotent covers the early return in putChunk: a chunk the reserve
// already holds under the very same stamp and with the very same content is a
// no-op, for both chunk types.
func TestPutIdempotent(t *testing.T) {
	t.Parallel()

	signer := getSigner(t)
	batch := postagetesting.MustNewBatch()

	for _, tc := range []struct {
		name  string
		chunk func(t *testing.T) swarm.Chunk
	}{
		{
			name: "cac",
			chunk: func(t *testing.T) swarm.Chunk {
				t.Helper()
				return newTestCAC(t, []byte("cac payload")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
			},
		},
		{
			name: "soc",
			chunk: func(t *testing.T) swarm.Chunk {
				t.Helper()
				return newTestSOC(t, signer, make([]byte, swarm.HashSize), []byte("soc payload")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newPutHarness(t)
			ch := tc.chunk(t)

			h.put(ch)
			bri, _ := h.entry(ch)
			firstBinID := bri.BinID

			h.put(ch)

			h.expectSize(1)
			h.expectSumCount(1)
			bri, _ = h.entry(ch)
			if bri.BinID != firstBinID {
				t.Fatalf("bin id changed on a no-op put: %d -> %d", firstBinID, bri.BinID)
			}
			h.expectSumAgainst(ch, ch)
		})
	}
}

// TestPutSOCSamePayloadUnderSecondStamp covers a single owner chunk arriving
// under a second batch with byte-identical content. The payload is stored once
// and referenced twice; each stamp gets its own entry and sum; evicting one
// batch leaves the chunk deliverable under the other.
func TestPutSOCSamePayloadUnderSecondStamp(t *testing.T) {
	t.Parallel()

	h := newPutHarness(t)
	signer := getSigner(t)
	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	chA := newTestSOC(t, signer, id, []byte("payload")).WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 1))
	chB := newTestSOC(t, signer, id, []byte("payload")).WithStamp(postagetesting.MustNewFields(batchB.ID, 0, 1))
	addr := chA.Address()

	h.put(chA)
	h.put(chB)

	h.expectSize(2)
	h.expectSumCount(2)
	h.expectEntry(chA, true)
	h.expectEntry(chB, true)
	h.expectSumAgainst(chA, chA)
	h.expectSumAgainst(chB, chA)

	h.evictBatch(batchA.ID)

	h.expectSize(1)
	h.expectSumCount(1)
	h.expectEntry(chA, false)
	h.expectEntry(chB, true)
	h.expectPayload(addr, chA)
	h.expectSumAgainst(chB, chA)

	h.evictBatch(batchB.ID)

	h.expectSize(0)
	h.expectSumCount(0)
	h.expectEntry(chB, false)
	h.expectNoPayload(addr)
}

// TestPutSOCLosingPayloadUnderNewStamp covers the putSOC branch where the
// address already holds a payload under another stamp and that payload wins
// the tie-break. The incoming payload is dropped, but its stamp is a valid
// claim on the address and is recorded against the payload the node keeps,
// with the entry sum computed from that payload. The stamp keeps the chunk
// alive after the winning stamp's batch is evicted.
func TestPutSOCLosingPayloadUnderNewStamp(t *testing.T) {
	t.Parallel()

	h := newPutHarness(t)
	signer := getSigner(t)
	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	newer := newTestSOC(t, signer, id, []byte("newer")).WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 2))
	older := newTestSOC(t, signer, id, []byte("older")).WithStamp(postagetesting.MustNewFields(batchB.ID, 0, 1))
	addr := newer.Address()

	h.put(newer)
	h.put(older)

	h.expectSize(2)
	h.expectSumCount(2)
	h.expectPayload(addr, newer)
	h.expectEntry(newer, true)
	h.expectEntry(older, true)
	h.expectSumAgainst(newer, newer)
	h.expectSumAgainst(older, newer)
	h.expectNoSum(older)

	// the same losing payload offered again under the same stamp is a
	// same-stamp divergence and is rejected without touching the state.
	h.putErr(older, storage.ErrOverwriteNewerChunk)
	h.expectPayload(addr, newer)
	h.expectSize(2)
	h.expectSumCount(2)

	h.evictBatch(batchA.ID)

	h.expectSize(1)
	h.expectEntry(newer, false)
	h.expectEntry(older, true)
	h.expectPayload(addr, newer)
	h.expectSumAgainst(older, newer)
}

// TestPutSOCCrossStampEqualTimestamp covers two single owner chunks that
// share an address under different batches at the same stamp timestamp. The
// payload stamped with the lexicographically lower stamp hash wins in either
// arrival order and both entries end up advertising it.
func TestPutSOCCrossStampEqualTimestamp(t *testing.T) {
	t.Parallel()

	signer := getSigner(t)
	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	low, high := stampsByHash(t, postagetesting.MustNewFields(batchA.ID, 0, 5), postagetesting.MustNewFields(batchB.ID, 0, 5))
	winner := newTestSOC(t, signer, id, []byte("payload one")).WithStamp(low)
	loser := newTestSOC(t, signer, id, []byte("payload two")).WithStamp(high)

	for _, tc := range []struct {
		name  string
		order []swarm.Chunk
	}{
		{"winner first", []swarm.Chunk{winner, loser}},
		{"loser first", []swarm.Chunk{loser, winner}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newPutHarness(t)

			for _, ch := range tc.order {
				h.put(ch)
			}

			h.expectSize(2)
			h.expectSumCount(2)
			h.expectPayload(winner.Address(), winner)
			h.expectEntry(winner, true)
			h.expectEntry(loser, true)
			h.expectSumAgainst(winner, winner)
			h.expectSumAgainst(loser, winner)
			h.expectNoSum(loser)
		})
	}
}

// TestPutSOCMultiStampHighestTimestamp covers an address that already carries
// several stamps. An incoming payload is compared against the highest stamp
// timestamp present, not against whichever stamp happens to be looked at, on
// both the new-stamp path (putSOC) and the same-stamp path (resolveDivergence).
func TestPutSOCMultiStampHighestTimestamp(t *testing.T) {
	t.Parallel()

	h := newPutHarness(t)
	signer := getSigner(t)
	id := make([]byte, swarm.HashSize)
	newBatch := func() []byte { return postagetesting.MustNewBatch().ID }

	stampA := postagetesting.MustNewFields(newBatch(), 0, 1)
	p1 := newTestSOC(t, signer, id, []byte("p1")).WithStamp(stampA)
	p2 := newTestSOC(t, signer, id, []byte("p2")).WithStamp(postagetesting.MustNewFields(newBatch(), 0, 5))
	p3 := newTestSOC(t, signer, id, []byte("p3")).WithStamp(postagetesting.MustNewFields(newBatch(), 0, 3))
	p4 := newTestSOC(t, signer, id, []byte("p4")).WithStamp(postagetesting.MustNewFields(newBatch(), 0, 6))
	p5 := newTestSOC(t, signer, id, []byte("p5")).WithStamp(stampA)
	addr := p1.Address()

	h.put(p1)
	h.put(p2)
	h.expectPayload(addr, p2)
	h.expectSumAgainst(p1, p2)

	// timestamp 3 is newer than stamp A but older than stamp B: the stored
	// payload stays and the new stamp is recorded against it.
	h.put(p3)
	h.expectSize(3)
	h.expectPayload(addr, p2)
	h.expectEntry(p3, true)
	h.expectSumAgainst(p3, p2)
	h.expectNoSum(p3)

	// timestamp 6 beats every stored stamp: payload replaced, all sums refreshed.
	h.put(p4)
	h.expectSize(4)
	h.expectSumCount(4)
	h.expectPayload(addr, p4)
	for _, ch := range []swarm.Chunk{p1, p2, p3, p4} {
		h.expectSumAgainst(ch, p4)
	}

	// a divergent payload re-offered under the oldest stamp is judged against
	// the highest stored timestamp and rejected.
	h.putErr(p5, storage.ErrOverwriteNewerChunk)
	h.expectSize(4)
	h.expectSumCount(4)
	h.expectPayload(addr, p4)
	h.expectNoSum(p5)
}

// TestPutSOCSameSlotEqualTimestampDistinctSignatures covers a stamp index
// collision where both chunks share the address, batch, index and timestamp
// but carry different signatures, hence different stamp hashes. Since
// f2d7856d the last write wins here (the earlier stamp-hash tie-break was
// dropped). With deterministic signatures the case cannot arise from valid
// stamps of one batch owner, so what matters is that the indexes stay
// consistent: exactly one entry for the slot, serving the last payload, with a
// fresh bin ID and a single sum row.
func TestPutSOCSameSlotEqualTimestampDistinctSignatures(t *testing.T) {
	t.Parallel()

	signer := getSigner(t)
	batch := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	first := newTestSOC(t, signer, id, []byte("payload one")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 7))
	second := newTestSOC(t, signer, id, []byte("payload two")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 7))

	for _, tc := range []struct {
		name  string
		order []swarm.Chunk
	}{
		{"one then two", []swarm.Chunk{first, second}},
		{"two then one", []swarm.Chunk{second, first}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newPutHarness(t)
			earlier, last := tc.order[0], tc.order[1]

			h.put(earlier)
			_, cbi := h.entry(earlier)
			earlierBinID := cbi.BinID

			h.put(last)

			h.expectSize(1)
			h.expectSumCount(1)
			h.expectPayload(last.Address(), last)
			h.expectEntry(last, true)
			h.expectEntry(earlier, false)
			h.expectSumAgainst(last, last)
			h.expectNoSum(earlier)

			// the replaced slot gets a fresh bin id so peers past the old one
			// are offered the replacement.
			_, cbi = h.entry(last)
			if cbi.BinID <= earlierBinID {
				t.Fatalf("expected bin id to advance past %d, got %d", earlierBinID, cbi.BinID)
			}
			bin := swarm.Proximity(h.baseAddr.Bytes(), last.Address().Bytes())
			checkStore(t, h.ts.IndexStore(), &reserve.ChunkBinItem{Bin: bin, BinID: earlierBinID}, true)
		})
	}
}

// TestPutSOCSameSlotRestampRefreshesSumIndex covers a single owner chunk
// re-stamped in the same slot with a newer timestamp and different content.
// The entry is replaced in place and the sum index follows: the old sum is
// gone, the new one is present, and nothing else is left behind.
func TestPutSOCSameSlotRestampRefreshesSumIndex(t *testing.T) {
	t.Parallel()

	h := newPutHarness(t)
	signer := getSigner(t)
	batch := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	v1 := newTestSOC(t, signer, id, []byte("v1")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 3))
	v2 := newTestSOC(t, signer, id, []byte("v2")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 4))
	addr := v1.Address()

	h.put(v1)
	h.expectSumAgainst(v1, v1)

	h.put(v2)

	h.expectSize(1)
	h.expectSumCount(1)
	h.expectPayload(addr, v2)
	h.expectEntry(v1, false)
	h.expectEntry(v2, true)
	h.expectSumAgainst(v2, v2)
	h.expectNoSum(v1)
}

// TestPutSOCCollisionEvictsOtherAddressAndSettlesPayload combines the two
// conflicts putSOC can meet in one put: the incoming stamp's slot is held by a
// different chunk with an older timestamp, and the incoming address already
// holds a payload under another batch. The slot holder is removed, the newer
// incoming payload replaces the stored one, and the sibling entry under the
// other batch is re-summed against it.
func TestPutSOCCollisionEvictsOtherAddressAndSettlesPayload(t *testing.T) {
	t.Parallel()

	h := newPutHarness(t)
	signer := getSigner(t)
	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	socV1 := newTestSOC(t, signer, id, []byte("v1")).WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 1))
	slotHolder := newTestCAC(t, []byte("unrelated cac")).WithStamp(postagetesting.MustNewFields(batchB.ID, 0, 1))
	socV2 := newTestSOC(t, signer, id, []byte("v2")).WithStamp(postagetesting.MustNewFields(batchB.ID, 0, 2))
	addr := socV1.Address()

	h.put(socV1)
	h.put(slotHolder)
	h.expectSize(2)

	h.put(socV2)

	h.expectSize(2)
	h.expectSumCount(2)
	h.expectEntry(slotHolder, false)
	h.expectNoPayload(slotHolder.Address())
	h.expectNoSum(slotHolder)

	h.expectPayload(addr, socV2)
	h.expectEntry(socV1, true)
	h.expectEntry(socV2, true)
	h.expectSumAgainst(socV1, socV2)
	h.expectSumAgainst(socV2, socV2)
	h.expectNoSum(socV1)
}
