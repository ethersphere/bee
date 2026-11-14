// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestSameSlotRestampHonorsOtherStamps pits the two payload-settlement paths
// against each other. A single owner chunk is re-stamped in the same batch
// slot with a newer timestamp (the in-place replacement path) while a stamp
// under another batch with a much higher timestamp already holds the address
// (the cross-stamp path). Whatever the rule, the outcome must not depend on
// arrival order.
func TestSameSlotRestampHonorsOtherStamps(t *testing.T) {
	t.Parallel()

	signer := getSigner(t)
	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	v1A := newTestSOC(t, signer, id, []byte("v1")).WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 1))
	v1B := newTestSOC(t, signer, id, []byte("v1")).WithStamp(postagetesting.MustNewFields(batchB.ID, 0, 100))
	v2A := newTestSOC(t, signer, id, []byte("v2")).WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 2))

	t.Run("all orders converge", func(t *testing.T) {
		t.Parallel()
		assertOrderConvergence(t, []swarm.Chunk{v1A, v1B, v2A}, false)
	})

	t.Run("same slot re-stamp cannot outrank a higher stamp under another batch", func(t *testing.T) {
		t.Parallel()
		h := newPutHarness(t)

		h.put(v1B) // timestamp 100 under batch B holds the address
		h.put(v1A) // timestamp 1 under batch A, same payload

		// timestamp 2 under batch A replaces the slot of v1A. Batch B still
		// pays for the address with timestamp 100, which outranks 2 on the
		// cross-stamp path, so the payload must stay v1.
		h.put(v2A)

		h.expectPayload(v1B.Address(), v1B)
		h.expectEntry(v1B, true)
		h.expectEntry(v2A, true)
		h.expectEntry(v1A, false)
		h.expectSumAgainst(v1B, v1B)
		h.expectSumAgainst(v2A, v1B)
	})
}

// TestSlotReuseAfterSettlementDiverges uses only legitimate mutable-batch
// behavior: the owner updates a single owner chunk with a newer stamp, then
// reuses that stamp slot for an unrelated chunk with an even newer timestamp
// (allowed on mutable batches). The stamp that decided the payload is gone,
// but the decision it made is never revisited, so nodes that saw the slot
// reuse before the update keep the old payload while the others keep the new
// one.
func TestSlotReuseAfterSettlementDiverges(t *testing.T) {
	t.Parallel()

	signer := getSigner(t)
	batchA := postagetesting.MustNewBatch()
	batchB := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	v1 := newTestSOC(t, signer, id, []byte("v1")).WithStamp(postagetesting.MustNewFields(batchB.ID, 0, 1))
	v2 := newTestSOC(t, signer, id, []byte("v2")).WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 5))
	reuse := newTestCAC(t, []byte("unrelated upload")).WithStamp(postagetesting.MustNewFields(batchA.ID, 0, 6))

	assertOrderConvergence(t, []swarm.Chunk{v1, v2, reuse}, true)
}

func strictConvergence() bool {
	return os.Getenv("RESERVE_STRICT_CONVERGENCE") != ""
}

// TestRandomOrderConvergence is a randomized version of the convergence table:
// small sets of chunks drawn from a pool of two divergent payloads at one SOC
// address (plus an unrelated CAC that can occupy a stamp slot) and a handful of
// stamps across two batches, two indices and three timestamps. Every set is
// applied in every order and must end in the same reserve state.
//
// With every stamp in its own (batch, index) slot and no other chunk
// competing for those slots, the outcome is order independent and the
// subtest enforces it. Once two stamps share a slot, a
// slot collision can remove the stamp that decided the payload without the
// decision being revisited, and the outcome depends on arrival order; that
// subtest reports divergences as known unresolved unless strict mode is on.
func TestRandomOrderConvergence(t *testing.T) {
	t.Parallel()

	signer := getSigner(t)
	batches := [][]byte{postagetesting.MustNewBatch().ID, postagetesting.MustNewBatch().ID}
	id := make([]byte, swarm.HashSize)

	payloads := []struct {
		name  string
		chunk swarm.Chunk
	}{
		{"soc1", newTestSOC(t, signer, id, []byte("soc payload one"))},
		{"soc2", newTestSOC(t, signer, id, []byte("soc payload two"))},
		{"cac", newTestCAC(t, []byte("unrelated cac payload"))},
	}

	for _, tc := range []struct {
		name          string
		distinctSlots bool
		enforce       bool
	}{
		{"distinct slots", true, true},
		{"shared slots", false, strictConvergence()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rng := rand.New(rand.NewSource(20260904)) //nolint:gosec // deterministic test input
			const sets = 60
			divergent := 0

			for set := 0; set < sets; set++ {
				// a small pool of stamps; the same stamp object may be reused
				// across payloads, and (unless slots are distinct) two stamps
				// may share every field but the signature.
				slots := rng.Perm(4)
				nStamps := 2 + rng.Intn(2)
				stamps := make([]*postage.Stamp, nStamps)
				stampDesc := make([]string, nStamps)
				for i := range stamps {
					slot := rng.Intn(4)
					if tc.distinctSlots {
						slot = slots[i]
					}
					b, idx, ts := slot/2, slot%2, 1+rng.Intn(3)
					stamps[i] = postagetesting.MustNewFields(batches[b], uint64(idx), uint64(ts))
					stampDesc[i] = fmt.Sprintf("s%d(batch%c idx%d ts%d)", i, 'A'+rune(b), idx, ts)
				}

				nChunks := 3 + rng.Intn(2)
				chunks := make([]swarm.Chunk, nChunks)
				desc := make([]string, nChunks)
				for i := range chunks {
					// bias towards the two SOC payloads; the CAC shows up now and
					// then, except when slots must stay distinct: a CAC sharing a
					// stamp with the SOC is itself a slot collision.
					p := rng.Intn(4)
					if p > 2 || tc.distinctSlots {
						p = rng.Intn(2)
					}
					st := rng.Intn(nStamps)
					chunks[i] = swarm.NewChunk(payloads[p].chunk.Address(), payloads[p].chunk.Data()).WithStamp(stamps[st])
					desc[i] = fmt.Sprintf("%s@%s", payloads[p].name, stampDesc[st])
				}

				perms := permutations(nChunks)
				first := runOrder(t, chunks, perms[0])
				for _, perm := range perms[1:] {
					fp := runOrder(t, chunks, perm)
					if fp == first {
						continue
					}
					divergent++
					msg := fmt.Sprintf("set %d diverges: %s\norder %v ends with:\n%s\n\norder %v ends with:\n%s",
						set, strings.Join(desc, ", "), perms[0], first, perm, fp)
					if tc.enforce {
						t.Error(msg)
					} else if divergent <= 3 {
						t.Logf("KNOWN UNRESOLVED (set RESERVE_STRICT_CONVERGENCE=1 to enforce):\n%s", msg)
					}
					break
				}
				if divergent >= 10 && tc.enforce {
					break
				}
			}
			t.Logf("%d of %d sets order dependent", divergent, sets)
		})
	}
}

// TestMissingSumEntrySelfHeals covers an index inconsistency: the reserve holds
// the entry (BatchRadiusItem) but the companion ChunkSumItem is missing, as a
// crashed migration or repair could leave behind. Re-putting the very same
// chunk must not be turned into a divergence conflict; the reserve should
// either restore the sum or stay a no-op, but never reject its own chunk.
func TestMissingSumEntrySelfHeals(t *testing.T) {
	t.Parallel()

	signer := getSigner(t)
	batch := postagetesting.MustNewBatch()

	for _, tc := range []struct {
		name  string
		chunk swarm.Chunk
	}{
		{"cac", newTestCAC(t, []byte("cac payload")).WithStamp(postagetesting.MustNewFields(batch.ID, 0, 1))},
		{"soc", newTestSOC(t, signer, make([]byte, swarm.HashSize), []byte("soc payload")).WithStamp(postagetesting.MustNewFields(batch.ID, 1, 1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newPutHarness(t)
			ch := tc.chunk

			h.put(ch)
			sum, err := storage.ChunkSum(ch)
			if err != nil {
				t.Fatal(err)
			}
			err = h.ts.Run(h.ctx, func(s transaction.Store) error {
				return s.IndexStore().Delete(&reserve.ChunkSumItem{Address: ch.Address(), Sum: sum})
			})
			if err != nil {
				t.Fatal(err)
			}

			h.put(ch)

			h.expectSize(1)
			h.expectPayload(ch.Address(), ch)
			h.expectSumAgainst(ch, ch)
		})
	}
}

// TestForeignStampWithMaxTimestampPinsPayload documents a consequence of
// settling divergent single owner chunks by stamp timestamp when stamp
// timestamps are neither bounded nor tied to the SOC owner: anyone holding
// any batch can re-stamp an old payload with the maximum timestamp, after
// which the owner's later updates under new stamps are recorded but never
// become the served payload.
func TestForeignStampWithMaxTimestampPinsPayload(t *testing.T) {
	t.Parallel()

	h := newPutHarness(t)
	owner := getSigner(t)
	ownerBatch := postagetesting.MustNewBatch()
	foreignBatch := postagetesting.MustNewBatch()
	id := make([]byte, swarm.HashSize)

	v1 := newTestSOC(t, owner, id, []byte("v1"))
	v2 := newTestSOC(t, owner, id, []byte("v2"))
	addr := v1.Address()

	ownerV1 := swarm.NewChunk(addr, v1.Data()).WithStamp(postagetesting.MustNewFields(ownerBatch.ID, 0, 10))
	pin := swarm.NewChunk(addr, v1.Data()).WithStamp(postagetesting.MustNewFields(foreignBatch.ID, 0, math.MaxUint64))
	ownerV2 := swarm.NewChunk(addr, v2.Data()).WithStamp(postagetesting.MustNewFields(ownerBatch.ID, 1, 11))

	h.put(ownerV1)
	h.put(pin)
	h.put(ownerV2)

	// current behavior: the foreign stamp decides, the owner's update is
	// stored as an entry that serves v1.
	h.expectPayload(addr, ownerV1)
	h.expectEntry(ownerV2, true)
	h.expectSumAgainst(ownerV2, ownerV1)
	h.expectNoSum(ownerV2)
}

// TestConcurrentPutsKeepInvariants hammers one SOC address from several
// goroutines with divergent payloads under a handful of stamps, optionally
// racing evictions of one batch, and checks the cross-index invariants at
// the end: every entry has a payload, every entry's sum matches the payload
// it serves, and the sum index is exactly the live set.
func TestConcurrentPutsKeepInvariants(t *testing.T) {
	t.Parallel()

	for _, withEviction := range []bool{false, true} {
		name := "puts only"
		if withEviction {
			name = "puts with concurrent eviction"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newPutHarness(t)
			signer := getSigner(t)
			batchA := postagetesting.MustNewBatch()
			batchB := postagetesting.MustNewBatch()
			id := make([]byte, swarm.HashSize)

			payloads := []swarm.Chunk{
				newTestSOC(t, signer, id, []byte("p1")),
				newTestSOC(t, signer, id, []byte("p2")),
				newTestSOC(t, signer, id, []byte("p3")),
			}
			stamps := []*postage.Stamp{
				postagetesting.MustNewFields(batchA.ID, 0, 1),
				postagetesting.MustNewFields(batchA.ID, 1, 2),
				postagetesting.MustNewFields(batchB.ID, 0, 2),
				postagetesting.MustNewFields(batchB.ID, 1, 3),
			}

			const workers, putsPerWorker = 8, 40
			var wg sync.WaitGroup
			errs := make(chan error, workers*putsPerWorker+putsPerWorker)

			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func(seed int64) {
					defer wg.Done()
					rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic test input
					for i := 0; i < putsPerWorker; i++ {
						p := payloads[rng.Intn(len(payloads))]
						s := stamps[rng.Intn(len(stamps))]
						ch := swarm.NewChunk(p.Address(), p.Data()).WithStamp(s.Clone())
						if err := h.r.Put(context.Background(), ch); err != nil && !benignPutErr(err) {
							errs <- err
						}
					}
				}(int64(w))
			}
			if withEviction {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < putsPerWorker; i++ {
						if _, err := h.r.EvictBatchBin(context.Background(), batchB.ID, math.MaxInt, swarm.MaxBins); err != nil {
							errs <- err
						}
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Errorf("concurrent op: %v", err)
			}

			// fingerprinting asserts the invariants as a side effect.
			_ = reserveFingerprint(t, h.ts)

			entries := 0
			err := h.ts.IndexStore().Iterate(storage.Query{Factory: func() storage.Item { return &reserve.BatchRadiusItem{} }},
				func(storage.Result) (bool, error) {
					entries++
					return false, nil
				})
			if err != nil {
				t.Fatal(err)
			}
			if got := h.r.Size(); got != entries {
				t.Fatalf("reserve size %d does not match %d live entries", got, entries)
			}
		})
	}
}
