// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/safe"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"golang.org/x/sync/errgroup"
	"resenje.org/multex"
)

const reserveScope = "reserve"

type Reserve struct {
	baseAddr     swarm.Address
	radiusSetter topology.SetStorageRadiuser
	logger       log.Logger

	capacity int
	size     atomic.Int64
	radius   atomic.Uint32

	multx *multex.Multex
	st    transaction.Storage
}

func New(
	baseAddr swarm.Address,
	st transaction.Storage,
	capacity int,
	radiusSetter topology.SetStorageRadiuser,
	logger log.Logger,
) (*Reserve, error) {
	rs := &Reserve{
		baseAddr:     baseAddr,
		st:           st,
		capacity:     capacity,
		radiusSetter: radiusSetter,
		logger:       logger.WithName(reserveScope).Register(),
		multx:        multex.New(),
	}

	err := st.Run(context.Background(), func(s transaction.Store) error {
		rItem := &radiusItem{}
		err := s.IndexStore().Get(rItem)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		rs.radius.Store(uint32(rItem.Radius))

		epochItem := &EpochItem{}
		err = s.IndexStore().Get(epochItem)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				err := s.IndexStore().Put(&EpochItem{Timestamp: uint64(time.Now().Unix())})
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		size, err := s.IndexStore().Count(&BatchRadiusItem{})
		if err != nil {
			return err
		}
		rs.size.Store(int64(size))
		return nil
	})

	return rs, err
}

// Put has to handle multiple possible scenarios.
//  1. Since the same chunk may belong to different postage stamp indices, the reserve will support one chunk to many postage
//     stamp indices relationship.
//  2. A new chunk that shares the same stamp index belonging to the same batch with an already stored chunk will overwrite
//     the existing chunk if the new chunk has a higher stamp timestamp (regardless of batch type).
//  3. A new chunk that has the same address belonging to the same stamp index with an already stored chunk will overwrite the existing chunk
//     if the new chunk has a higher stamp timestamp (regardless of batch type and chunk type, eg CAC & SOC).
//  4. Two different chunk addresses that share the same batch, stamp index and timestamp are settled by a tie-break:
//     the lexicographically lower chunk address wins. The loser is rejected; the winner replaces the stored chunk
//     through the usual remove-and-store path (including a fresh bin ID for pullsync).
//  5. Two single owner chunks that share an address under different stamps (any batch or stamp
//     index) settle on one shared payload: a strictly higher stamp timestamp replaces it; equal
//     timestamps are settled by the lexicographically lower wrapped chunk address, never by the
//     malleable stamp hash. Same-stamp divergence is handled by replaceDivergentSOC.
func (r *Reserve) Put(ctx context.Context, chunk swarm.Chunk) error {
	socReplaced, err := r.putChunk(ctx, chunk)
	if err != nil {
		return err
	}
	// When a winning SOC replaces the shared payload, refreshSiblingSums updates
	// the sums of all co-resident entries under other stamps. This is called
	// outside putChunk (after releasing the batch lock) to avoid multi-batch deadlocks.
	// The payload is already committed, so the refresh must not be abandoned
	// when the caller's context is canceled: the siblings would keep
	// advertising sums of the replaced payload.
	if socReplaced {
		return r.refreshSiblingSums(context.WithoutCancel(ctx), chunk.Address())
	}
	return nil
}

func (r *Reserve) putChunk(ctx context.Context, chunk swarm.Chunk) (socReplaced bool, err error) {
	r.multx.Lock(string(chunk.Stamp().BatchID()))
	defer r.multx.Unlock(string(chunk.Stamp().BatchID()))

	stampHash, err := chunk.Stamp().Hash()
	if err != nil {
		return false, fmt.Errorf("stamp hash: %w", err)
	}

	chunkType := storage.ChunkType(chunk)
	if chunkType == swarm.ChunkTypeUnspecified {
		return false, fmt.Errorf("chunk type unspecified: %w", storage.ErrInvalidChunk)
	}

	sum, err := storage.ChunkSumFromParts(chunk.Stamp().BatchID(), stampHash, chunk, chunkType)
	if err != nil {
		return false, fmt.Errorf("chunk sum: %w", err)
	}

	bin := swarm.Proximity(r.baseAddr.Bytes(), chunk.Address().Bytes())

	// check if the chunk with the same batch, stamp timestamp and index is already stored
	has, err := r.Has(chunk.Address(), chunk.Stamp().BatchID(), stampHash)
	if err != nil {
		return false, fmt.Errorf("has chunk %s: %w", chunk.Address(), err)
	}
	if has {
		// Address, batch and stamp all match, but two single owner chunks can
		// share those and still wrap different content. The sum tells them
		// apart: if it matches we already hold this exact chunk, otherwise the
		// chunks diverge and a tie-break decides which one the neighborhood
		// keeps.
		hasSum, err := r.HasSum(chunk.Address(), sum)
		if err != nil {
			return false, fmt.Errorf("has chunk sum %s: %w", chunk.Address(), err)
		}
		if hasSum {
			return false, nil
		}
		// A content addressed chunk cannot diverge: its address is the hash
		// of its payload, so the same address, batch and stamp means the same
		// chunk and only its sum index entry is missing. The chunk is already
		// stored; there is nothing to resolve.
		if chunkType != swarm.ChunkTypeSingleOwner {
			return false, nil
		}
		r.logger.Debug("same stamp divergent sum",
			"address", chunk.Address(),
			"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
			"sum", hex.EncodeToString(sum),
		)
		// when the tie-break winner replaced the shared payload, co-resident
		// entries under other stamps need their sums refreshed like on any
		// other single owner chunk replacement.
		return r.replaceDivergentSOC(ctx, chunk, sum, stampHash, bin)
	}

	// bin lock
	r.multx.Lock(strconv.Itoa(int(bin)))
	defer r.multx.Unlock(strconv.Itoa(int(bin)))

	var shouldIncReserveSize bool
	if chunkType == swarm.ChunkTypeSingleOwner {
		socReplaced, shouldIncReserveSize, err = r.putSOC(ctx, chunk, sum, stampHash, bin)
	} else {
		shouldIncReserveSize, err = r.putCAC(ctx, chunk, sum, stampHash, bin)
	}
	if err != nil {
		return false, err
	}
	if shouldIncReserveSize {
		r.size.Add(1)
	}
	return socReplaced, nil
}

func (r *Reserve) putSOC(ctx context.Context, chunk swarm.Chunk, sum, stampHash []byte, bin uint8) (socReplaced, shouldInc bool, err error) {
	err = r.st.Run(ctx, func(s transaction.Store) error {
		// Step 1: Check for a same-slot (batchID, stampIndex) collision.
		oldStampIndex, loaded, err := stampindex.LoadOrStore(s.IndexStore(), reserveScope, chunk)
		if err != nil {
			return fmt.Errorf("load or store stamp index for chunk %v: %w", chunk, err)
		}

		// If the slot is already occupied, settle the collision: either a
		// re-stamp of the same address, or slot reuse by a newer chunk at
		// another address.
		var sameSlotSameAddr bool
		if loaded {
			sameSlotSameAddr, err = checkStampIndexCollision(oldStampIndex, chunk)
			if err != nil {
				return fmt.Errorf("stamp index collision: %w", err)
			}
			if err := r.reassignStampSlot(ctx, s, chunk, oldStampIndex, bin, sameSlotSameAddr); err != nil {
				return err
			}
		}

		// Step 2: Check if this SOC address already holds a payload on disk
		// (under this or any sibling batch stamps).
		has, err := s.ChunkStore().Has(ctx, chunk.Address())
		if err != nil {
			return fmt.Errorf("chunkstore has %s: %w", chunk.Address(), err)
		}

		// The address may already hold a payload under other stamps. The
		// incoming stamp is new or re-stamped either way, so it always gets its
		// own reserve entry; the tie-break only decides which payload the
		// address keeps.
		var stored swarm.Chunk
		incomingWins := true
		if has {
			stored, err = s.ChunkStore().Get(ctx, chunk.Address())
			if err != nil {
				return fmt.Errorf("load stored chunk %s: %w", chunk.Address(), err)
			}
			// When evaluating cross-stamp divergence, fallback to stored stamp only
			// if this is not a same-slot collision at the same address.
			var fallbackStamp swarm.Stamp
			if !sameSlotSameAddr {
				fallbackStamp = stored.Stamp()
			}
			incomingWins, err = r.tiebreakSOC(s, chunk, stored, fallbackStamp)
			if err != nil {
				return err
			}
		}

		// Step 3: Determine entrySum. If the incoming chunk lost the tie-break,
		// the node retains the stored payload. Because stamps sign the address
		// rather than content, the incoming stamp remains a valid claim on that
		// address, but its reserve entry must advertise the stored payload's sum
		// so pullsync offers match the payload actually delivered to peers.
		entrySum := sum
		if has && !incomingWins {
			entrySum, err = storage.ChunkSumFromParts(chunk.Stamp().BatchID(), stampHash, stored, swarm.ChunkTypeSingleOwner)
			if err != nil {
				return fmt.Errorf("sum stored chunk %s: %w", chunk.Address(), err)
			}
		}

		// Step 4: Index the reserve entries (BatchRadiusItem, ChunkBinItem, ChunkSumItem).
		if err := r.storeReserveEntries(s, chunk, swarm.ChunkTypeSingleOwner, entrySum, stampHash, bin); err != nil {
			return err
		}

		// Step 5: Update ChunkStore payload and maintain reference counts.
		// Every reserve entry at this address holds one reference on the
		// shared payload.
		if !has {
			// First time this SOC address is stored; write the new payload.
			err = s.ChunkStore().Put(ctx, chunk)
		} else if incomingWins {
			// Incoming chunk outranked the stored payload; replace it on disk.
			// A new stamp also takes a reference; a same-slot re-stamp reuses
			// the reference of the entry it replaced.
			if !bytes.Equal(stored.Data(), chunk.Data()) {
				socReplaced = true
			}
			err = s.ChunkStore().Replace(ctx, chunk, !sameSlotSameAddr)
		} else if !sameSlotSameAddr {
			// Incoming chunk lost the tie-break under a new stamp. The stamp
			// signs the address, not the content, so it is still a valid claim
			// on this address and its entry was stored above against the
			// stored payload: dropping it would leave nodes with different
			// stamp sets depending on arrival order. Take a reference for the
			// entry so evicting the winning stamp's batch does not delete the
			// payload this entry serves.
			err = s.ChunkStore().Put(ctx, stored)
		}
		// Otherwise the incoming chunk lost a same-slot re-stamp: the stored
		// payload is retained and this slot already holds its reference.
		if err != nil {
			return fmt.Errorf("update chunkstore for %s: %w", chunk.Address(), err)
		}

		// Step 6: Only increment reserve size if a brand new stamp slot was added.
		shouldInc = !loaded
		return nil
	})
	return
}

// tiebreakSOC decides whether an incoming single owner chunk should
// replace the payload already stored at that address under another stamp.
//
// The incoming stamp timestamp is compared with the latest stored stamp
// timestamp on that address:
//   - a strictly newer timestamp wins and a strictly older one loses;
//   - on equal timestamps the lower wrapped CAC address wins.
//
// Stamp hashes are deliberately not compared: anyone can malleate a stamp
// signature into a valid twin with a different hash, which would let a
// relaying peer choose the payload each node keeps. The equal-timestamp
// outcome depends on content only, so every node keeps the same payload.
// Matching payloads are treated as a win so the new stamp can be indexed
// against the existing body without payload modification.
func (r *Reserve) tiebreakSOC(
	s transaction.Store,
	incoming swarm.Chunk,
	stored swarm.Chunk,
	fallbackStamp swarm.Stamp,
) (incomingWins bool, err error) {
	// If the underlying payloads are identical, there is no divergence.
	if bytes.Equal(stored.Data(), incoming.Data()) {
		return true, nil
	}

	prev, err := latestTimestamp(s.IndexStore(), incoming.Address(), fallbackStamp)
	if err != nil {
		return false, err
	}
	curr := binary.BigEndian.Uint64(incoming.Stamp().Timestamp())
	if curr != prev {
		return curr > prev, nil
	}

	wins, err := storage.ReplaceIncoming(stored, incoming)
	if err != nil {
		return false, fmt.Errorf("divergence tie-break for chunk %s: %w", incoming.Address(), err)
	}
	return wins, nil
}

// putCAC writes a content-addressed chunk to the reserve.
// Unlike SOCs, CAC chunks cannot diverge (their address is the cryptographic
// hash of their payload). If a same-slot collision points to the same address,
// only the index entries are rewritten without touching ChunkStore.
func (r *Reserve) putCAC(ctx context.Context, chunk swarm.Chunk, sum, stampHash []byte, bin uint8) (shouldInc bool, err error) {
	err = r.st.Run(ctx, func(s transaction.Store) error {
		oldStampIndex, loaded, err := stampindex.LoadOrStore(s.IndexStore(), reserveScope, chunk)
		if err != nil {
			return fmt.Errorf("load or store stamp index for chunk %v: %w", chunk, err)
		}

		if loaded {
			sameAddr, err := checkStampIndexCollision(oldStampIndex, chunk)
			if err != nil {
				return fmt.Errorf("stamp index collision: %w", err)
			}
			if err := r.reassignStampSlot(ctx, s, chunk, oldStampIndex, bin, sameAddr); err != nil {
				return err
			}
			if sameAddr {
				return r.storeReserveEntries(s, chunk, swarm.ChunkTypeContentAddressed, sum, stampHash, bin)
			}
		}

		if err := r.storeReserveEntries(s, chunk, swarm.ChunkTypeContentAddressed, sum, stampHash, bin); err != nil {
			return err
		}

		if err := s.ChunkStore().Put(ctx, chunk); err != nil {
			return err
		}

		shouldInc = !loaded
		return nil
	})
	return
}

// checkStampIndexCollision decides whether chunk may take the stamp slot
// (batch ID and stamp index) that LoadOrStore found occupied by old. It only
// compares the two claims and touches no storage. On success it reports
// whether the slot holds the same chunk address (a re-stamp) or another
// address (slot reuse); an error means the incoming chunk does not get the
// slot:
//   - an older timestamp loses with ErrOverwriteNewerChunk;
//   - a re-stamp of the same address needs a strictly newer timestamp. An
//     equal timestamp with a different stamp hash can be minted by anyone
//     from an existing stamp (signature malleability), so it never wins;
//   - an equal timestamp at another address is settled on the lower address;
//     the higher one loses with ErrDivergentChunkRejected.
func checkStampIndexCollision(old *stampindex.Item, chunk swarm.Chunk) (sameAddr bool, err error) {
	prev := binary.BigEndian.Uint64(old.StampTimestamp)
	curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
	sameAddr = old.ChunkAddress.Equal(chunk.Address())
	if prev > curr || (prev == curr && sameAddr) {
		return false, fmt.Errorf("overwrite same chunk. prev %d cur %d batch %s: %w", prev, curr, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
	}
	if prev == curr && bytes.Compare(chunk.Address().Bytes(), old.ChunkAddress.Bytes()) >= 0 {
		return false, fmt.Errorf("stamp index collision chunk %s lost tie-break: %w", chunk.Address(), storage.ErrDivergentChunkRejected)
	}
	return sameAddr, nil
}

// reassignStampSlot hands the stamp slot held by old to chunk, once
// checkStampIndexCollision allowed it, and points the stamp index at chunk.
//
//   - sameAddr (re-stamp): the old stamp's reserve entries (BatchRadiusItem,
//     ChunkBinItem, stamp rows) are removed. The payload stays: it is either
//     replaced or kept at this same address by the caller.
//   - otherwise (slot reuse): the older chunk at the other address is removed
//     completely, its metadata and its payload reference.
//
// The caller then writes the reserve entries for chunk and handles the
// chunkstore.
func (r *Reserve) reassignStampSlot(ctx context.Context, s transaction.Store, chunk swarm.Chunk, old *stampindex.Item, bin uint8, sameAddr bool) error {
	if sameAddr {
		if err := removeStampEntries(s, old, bin); err != nil {
			return fmt.Errorf("remove previous stamp of %s: %w", old.ChunkAddress, err)
		}
	} else {
		if err := r.removeChunk(ctx, s, old.ChunkAddress, old.BatchID, old.StampHash); err != nil {
			return fmt.Errorf("remove older chunk %s: %w", old.ChunkAddress, err)
		}
	}
	if err := stampindex.Store(s.IndexStore(), reserveScope, chunk); err != nil {
		return fmt.Errorf("update stamp index: %w", err)
	}
	return nil
}

// removeStampEntries removes the reserve entries of the stamp recorded in old
// (BatchRadiusItem, ChunkBinItem with its sum, stamp index and chunk stamp
// rows) without touching the payload in the chunkstore.
func removeStampEntries(s transaction.Store, old *stampindex.Item, bin uint8) error {
	oldStamp, err := chunkstamp.LoadWithStampHash(s.IndexStore(), reserveScope, old.ChunkAddress, old.StampHash)
	if err != nil {
		return err
	}

	item := &BatchRadiusItem{
		Bin:       bin,
		Address:   old.ChunkAddress,
		BatchID:   old.BatchID,
		StampHash: old.StampHash,
	}
	if err := s.IndexStore().Get(item); err != nil {
		return err
	}

	return errors.Join(
		s.IndexStore().Delete(item),
		deleteChunkBinItem(s.IndexStore(), item),
		stampindex.Delete(s.IndexStore(), reserveScope, oldStamp),
		chunkstamp.DeleteWithStamp(s.IndexStore(), reserveScope, item.Address, oldStamp),
	)
}

// storeReserveEntries writes the common set of reserve index entries for a
// chunk: chunkstamp, BatchRadiusItem, ChunkBinItem and ChunkSumItem and allocates a fresh bin ID via IncBinID.
// The stamp index is NOT written here because its lifecycle differs across call sites (LoadOrStore vs explicit Store after collision cleanup).
func (r *Reserve) storeReserveEntries(s transaction.Store, chunk swarm.Chunk, chunkType swarm.ChunkType, sum, stampHash []byte, bin uint8) error {
	binID, err := r.IncBinID(s.IndexStore(), bin)
	if err != nil {
		return err
	}

	return errors.Join(
		chunkstamp.Store(s.IndexStore(), reserveScope, chunk),
		s.IndexStore().Put(&BatchRadiusItem{
			Bin:       bin,
			BinID:     binID,
			Address:   chunk.Address(),
			BatchID:   chunk.Stamp().BatchID(),
			StampHash: stampHash,
		}),
		s.IndexStore().Put(&ChunkBinItem{
			Bin:       bin,
			BinID:     binID,
			Address:   chunk.Address(),
			BatchID:   chunk.Stamp().BatchID(),
			ChunkType: chunkType,
			StampHash: stampHash,
			Sum:       sum,
		}),
		s.IndexStore().Put(&ChunkSumItem{Address: chunk.Address(), Sum: [storage.ChunkSumSize]byte(sum)}),
	)
}

// refreshSiblingSums recomputes the divergence checksum of every reserve entry
// at the given address after its shared payload was replaced. Without the
// refresh, entries under other stamps keep advertising content the node no
// longer holds (peers reject the deliveries as unsolicited) and keep matching
// offers for content it cannot store.
//
// It must be called with no reserve locks held: each sibling entry is updated
// under its own batch lock, one at a time, mirroring the Put-vs-Eviction lock
// discipline. The sum is recomputed from the currently committed payload
// rather than the caller's chunk, so concurrent replacements converge on the
// content that committed last. Entries whose sum already matches, including
// the caller's own, are left untouched.
func (r *Reserve) refreshSiblingSums(ctx context.Context, addr swarm.Address) error {
	bin := swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes())

	// collect first: the underlying stores do not support writes during an
	// iteration. A stamp deleted between collection and its locked update is
	// skipped when its index entry turns up missing.
	var stamps []swarm.Stamp
	err := chunkstamp.IterateAll(r.st.IndexStore(), reserveScope, addr, func(stamp swarm.Stamp) (bool, error) {
		stamps = append(stamps, stamp)
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("iterate stamps for %s: %w", addr, err)
	}

	for _, stamp := range stamps {
		stampHash, err := stamp.Hash()
		if err != nil {
			return err
		}

		err = func() error {
			// batchID lock, refresh vs Put/Eviction
			r.multx.Lock(string(stamp.BatchID()))
			defer r.multx.Unlock(string(stamp.BatchID()))

			return r.st.Run(ctx, func(s transaction.Store) error {
				chunk, err := s.ChunkStore().Get(ctx, addr)
				if err != nil {
					if errors.Is(err, storage.ErrNotFound) {
						return nil
					}
					return err
				}

				sum, err := storage.ChunkSumFromParts(stamp.BatchID(), stampHash, chunk, swarm.ChunkTypeSingleOwner)
				if err != nil {
					return err
				}

				item := &BatchRadiusItem{Bin: bin, BatchID: stamp.BatchID(), Address: addr, StampHash: stampHash}
				err = s.IndexStore().Get(item)
				if err != nil {
					if errors.Is(err, storage.ErrNotFound) {
						return nil
					}
					return err
				}

				cbi := &ChunkBinItem{Bin: bin, BinID: item.BinID}
				err = s.IndexStore().Get(cbi)
				if err != nil {
					if errors.Is(err, storage.ErrNotFound) {
						return nil
					}
					return err
				}

				if bytes.Equal(cbi.Sum, sum) {
					r.logger.Debug("refreshSiblingSums sum unchanged",
						"address", addr, "batch_id", hex.EncodeToString(stamp.BatchID()),
						"stamp_index", hex.EncodeToString(stamp.Index()),
						"stamp_timestamp", binary.BigEndian.Uint64(stamp.Timestamp()),
						"bin_id", item.BinID, "sum", hex.EncodeToString(sum), "wrapped_chunk_address", wrappedAddrHex(chunk),
					)
					return nil
				}

				oldSum := cbi.Sum
				cbi.Sum = sum
				r.logger.Debug("refreshSiblingSums updating sum",
					"address", addr, "batch_id", hex.EncodeToString(stamp.BatchID()),
					"stamp_index", hex.EncodeToString(stamp.Index()),
					"stamp_timestamp", binary.BigEndian.Uint64(stamp.Timestamp()),
					"bin_id", item.BinID,
					"old_sum", hex.EncodeToString(oldSum),
					"new_sum", hex.EncodeToString(sum),
					"wrapped_chunk_address", wrappedAddrHex(chunk),
				)
				return errors.Join(
					s.IndexStore().Delete(&ChunkSumItem{Address: addr, Sum: [storage.ChunkSumSize]byte(oldSum)}),
					s.IndexStore().Put(cbi),
					s.IndexStore().Put(&ChunkSumItem{Address: addr, Sum: [storage.ChunkSumSize]byte(sum)}),
				)
			})
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// replaceDivergentSOC settles two single owner chunks that share an address,
// batch and stamp but wrap different content. Both are individually valid, so
// the protocol cannot pick between them; the choice is made here, in the
// storage layer. The incoming chunk loses to any newer stamp already stored at
// the address; otherwise the tie-break depends only on the two payloads (the
// lower wrapped chunk address wins). Every node in the neighborhood therefore
// converges on the same chunk no matter which one it received first.
//
// If the incoming chunk wins it replaces the stored one in place, reusing the
// existing stamp index and stamp entries, which are identical for both. The
// bin ID is bumped so that peers which already synced past the old bin ID are
// offered the replacement, propagating the resolution outwards.
//
// The reserve size is unchanged either way: one chunk goes in, one comes out.
// It reports whether the stored payload was replaced.
func (r *Reserve) replaceDivergentSOC(
	ctx context.Context,
	chunk swarm.Chunk,
	sum []byte,
	stampHash []byte,
	bin uint8,
) (replaced bool, err error) {
	// bin lock
	r.multx.Lock(strconv.Itoa(int(bin)))
	defer r.multx.Unlock(strconv.Itoa(int(bin)))

	err = r.st.Run(ctx, func(s transaction.Store) error {
		stored, err := s.ChunkStore().Get(ctx, chunk.Address())
		if err != nil {
			return fmt.Errorf("load diverging chunk %s: %w", chunk.Address(), err)
		}

		// The same payload under the same stamp has the same sum, so an
		// unmatched sum with identical payload means only the sum index
		// entry is missing. The chunk is already stored; there is nothing
		// to settle.
		if bytes.Equal(stored.Data(), chunk.Data()) {
			return nil
		}
		// ChunkStore returns payload only; stamp is in the chunkstamp index.
		stamp, err := chunkstamp.Load(s.IndexStore(), reserveScope, chunk.Address())
		if err != nil {
			return fmt.Errorf("load stamp for diverging chunk %s: %w", chunk.Address(), err)
		}

		// Verify timestamp precedence across all active co-resident stamps.
		// Equal timestamps fall through to the content tie-break; stamp
		// hashes are not compared (see tiebreakSOC).
		prev, err := latestTimestamp(s.IndexStore(), chunk.Address(), stamp)
		if err != nil {
			return err
		}
		curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
		if prev > curr {
			return fmt.Errorf("overwrite same chunk. prev %d cur %d batch %s: %w", prev, curr, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
		}

		wins, err := storage.ReplaceIncoming(stored, chunk)
		if err != nil {
			return fmt.Errorf("divergence tie-break for chunk %s: %w", chunk.Address(), err)
		}

		if !wins {
			r.logger.Debug(
				"discarding diverging chunk",
				"address", chunk.Address(),
				"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
				"stored_wrapped_chunk_address", wrappedAddrHex(stored),
				"incoming_wrapped_chunk_address", wrappedAddrHex(chunk),
			)
			return fmt.Errorf("diverging chunk %s lost tie-break: %w", chunk.Address(), storage.ErrDivergentChunkRejected)
		}

		item := &BatchRadiusItem{
			Bin:       bin,
			Address:   chunk.Address(),
			BatchID:   chunk.Stamp().BatchID(),
			StampHash: stampHash,
		}
		// load item to get the binID of the chunk being replaced
		if err := s.IndexStore().Get(item); err != nil {
			return err
		}

		// drop the bin and sum entries of the replaced chunk
		if err := deleteChunkBinItem(s.IndexStore(), item); err != nil {
			return err
		}

		binID, err := r.IncBinID(s.IndexStore(), bin)
		if err != nil {
			return err
		}

		r.logger.Debug(
			"replacing diverging chunk",
			"address", chunk.Address(),
			"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
			"old_bin_id", item.BinID,
			"new_bin_id", binID,
			"stored_wrapped_chunk_address", wrappedAddrHex(stored),
			"incoming_wrapped_chunk_address", wrappedAddrHex(chunk),
		)

		// the BatchRadiusItem key does not cover the binID, so putting it again
		// with the new binID overwrites the existing entry.
		item.BinID = binID
		err = errors.Join(
			s.IndexStore().Put(item),
			s.IndexStore().Put(&ChunkBinItem{
				Bin:       bin,
				BinID:     binID,
				Address:   chunk.Address(),
				BatchID:   chunk.Stamp().BatchID(),
				ChunkType: swarm.ChunkTypeSingleOwner,
				StampHash: stampHash,
				Sum:       sum,
			}),
			s.IndexStore().Put(&ChunkSumItem{Address: chunk.Address(), Sum: [storage.ChunkSumSize]byte(sum)}),
		)
		if err != nil {
			return err
		}

		// swap the payload without touching the reference count: the chunk
		// store entry is reused, only its content changes.
		replaced = true
		return s.ChunkStore().Replace(ctx, chunk, false)
	})
	return replaced && err == nil, err
}

// latestTimestamp returns the highest stamp timestamp among all reserve
// stamps at addr: a single owner chunk is stored once by address but may have
// several stamps, and an incoming version is compared against the latest of
// them. The result does not depend on iteration or arrival order. If addr has
// no indexed stamps, the fallback stamp's timestamp is returned, or zero
// without a fallback.
func latestTimestamp(store storage.IndexStore, addr swarm.Address, fallback swarm.Stamp) (uint64, error) {
	var latest uint64
	found := false
	err := chunkstamp.IterateAll(store, reserveScope, addr, func(stamp swarm.Stamp) (bool, error) {
		latest = max(latest, binary.BigEndian.Uint64(stamp.Timestamp()))
		found = true
		return false, nil
	})
	if err != nil {
		return 0, fmt.Errorf("iterate stamps of %s: %w", addr, err)
	}
	if !found && fallback != nil {
		return binary.BigEndian.Uint64(fallback.Timestamp()), nil
	}
	return latest, nil
}

func (r *Reserve) Has(addr swarm.Address, batchID []byte, stampHash []byte) (bool, error) {
	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr, StampHash: stampHash}
	return r.st.IndexStore().Has(item)
}

// HasSum reports whether the reserve holds a chunk at addr whose pullsync
// divergence checksum equals sum. Used by the pullsync want-decision.
func (r *Reserve) HasSum(addr swarm.Address, sum []byte) (bool, error) {
	if len(sum) != storage.ChunkSumSize {
		return false, fmt.Errorf("chunk sum length %d: %w", len(sum), storage.ErrInvalidChunk)
	}
	return r.st.IndexStore().Has(&ChunkSumItem{Address: addr, Sum: [storage.ChunkSumSize]byte(sum)})
}

func (r *Reserve) Get(ctx context.Context, addr swarm.Address, batchID []byte, stampHash []byte) (swarm.Chunk, error) {
	r.multx.Lock(string(batchID))
	defer r.multx.Unlock(string(batchID))

	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr, StampHash: stampHash}
	err := r.st.IndexStore().Get(item)
	if err != nil {
		return nil, err
	}

	stamp, err := chunkstamp.LoadWithStampHash(r.st.IndexStore(), reserveScope, addr, stampHash)
	if err != nil {
		return nil, err
	}

	ch, err := r.st.ChunkStore().Get(ctx, addr)
	if err != nil {
		return nil, err
	}

	return ch.WithStamp(stamp), nil
}

// EvictBatchBin evicts all chunks from bins upto the bin provided.
// Pinned chunks are protected from eviction to maintain data integrity.
func (r *Reserve) EvictBatchBin(
	ctx context.Context,
	batchID []byte,
	count int,
	bin uint8,
) (int, error) {
	r.multx.Lock(string(batchID))
	defer r.multx.Unlock(string(batchID))

	var (
		evictedItems       []*BatchRadiusItem
		pinnedEvictedItems []*BatchRadiusItem
	)

	if count <= 0 {
		return 0, nil
	}

	pinUuids, err := pinstore.GetCollectionUUIDs(r.st.IndexStore())
	if err != nil {
		return 0, err
	}

	err = r.st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &BatchRadiusItem{} },
		Prefix:  string(batchID),
	}, func(res storage.Result) (bool, error) {
		batchRadius := res.Entry.(*BatchRadiusItem)
		if batchRadius.Bin >= bin {
			return true, nil
		}

		// Check if the chunk is pinned in any collection
		pinned := false
		for _, uuid := range pinUuids {
			has, err := pinstore.IsChunkPinnedInCollection(r.st.IndexStore(), batchRadius.Address, uuid)
			if err != nil {
				return true, err
			}
			if has {
				pinned = true
				pinnedEvictedItems = append(pinnedEvictedItems, batchRadius)
				break
			}
		}

		if !pinned {
			evictedItems = append(evictedItems, batchRadius)
		}
		count--
		if count == 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return 0, err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(runtime.NumCPU())

	var evicted atomic.Int64

	for _, item := range evictedItems {
		func(item *BatchRadiusItem) {
			eg.Go(safe.RunFunc(r.logger, "reserve-eviction-remove-chunk", func() error {
				err := r.st.Run(ctx, func(s transaction.Store) error {
					return RemoveChunkWithItem(ctx, s, item)
				})
				if err != nil {
					return err
				}
				evicted.Add(1)
				return nil
			}))
		}(item)
	}

	for _, item := range pinnedEvictedItems {
		func(item *BatchRadiusItem) {
			eg.Go(safe.RunFunc(r.logger, "reserve-eviction-remove-metadata", func() error {
				err := r.st.Run(ctx, func(s transaction.Store) error {
					return RemoveChunkMetaData(ctx, s, item)
				})
				if err != nil {
					return err
				}
				evicted.Add(1)
				return nil
			}))
		}(item)
	}

	err = eg.Wait()

	r.size.Add(-evicted.Load())

	return int(evicted.Load()), err
}

func (r *Reserve) removeChunk(
	ctx context.Context,
	trx transaction.Store,
	chunkAddress swarm.Address,
	batchID []byte,
	stampHash []byte,
) error {
	item := &BatchRadiusItem{
		Bin:       swarm.Proximity(r.baseAddr.Bytes(), chunkAddress.Bytes()),
		BatchID:   batchID,
		Address:   chunkAddress,
		StampHash: stampHash,
	}
	err := trx.IndexStore().Get(item)
	if err != nil {
		return err
	}
	return RemoveChunkWithItem(ctx, trx, item)
}

// deleteChunkBinItem removes the ChunkBinItem at the (bin, binID) of item
// together with its companion ChunkSumItem, keeping the pullsync sum index
// consistent. It is a no-op if the ChunkBinItem does not exist or belongs to
// another entry.
func deleteChunkBinItem(store storage.IndexStore, item *BatchRadiusItem) error {
	cbi := &ChunkBinItem{Bin: item.Bin, BinID: item.BinID}
	err := store.Get(cbi)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		// The stored value predates the Sum field (pre-migration record, seen
		// during the Sum backfill migration and the sharky recovery that runs
		// before it). Such a record cannot have a companion ChunkSumItem, so
		// deleting by key alone is complete.
		if errors.Is(err, errUnmarshalInvalidSize) {
			return store.Delete(&ChunkBinItem{Bin: item.Bin, BinID: item.BinID})
		}
		return err
	}
	// In a corrupted reserve two entries can share a (bin, binID) key. The
	// stored item then belongs to the other entry and must be kept.
	if !cbi.Address.Equal(item.Address) || !bytes.Equal(cbi.StampHash, item.StampHash) {
		return nil
	}
	return errors.Join(
		store.Delete(cbi),
		store.Delete(&ChunkSumItem{Address: cbi.Address, Sum: [storage.ChunkSumSize]byte(cbi.Sum)}),
	)
}

func RemoveChunkWithItem(
	ctx context.Context,
	trx transaction.Store,
	item *BatchRadiusItem,
) error {
	return errors.Join(
		removeChunkMetadata(trx.IndexStore(), item),
		trx.ChunkStore().Delete(ctx, item.Address),
	)
}

// RemoveChunkMetaData removes chunk reserve metadata from reserve indexes but keeps the cunks in the chunkstore.
// used at pinned data eviction
func RemoveChunkMetaData(
	ctx context.Context,
	trx transaction.Store,
	item *BatchRadiusItem,
) error {
	return removeChunkMetadata(trx.IndexStore(), item)
}

func removeChunkMetadata(store storage.IndexStore, item *BatchRadiusItem) error {
	var errs error

	stamp, _ := chunkstamp.LoadWithStampHash(store, reserveScope, item.Address, item.StampHash)
	if stamp != nil {
		errs = errors.Join(
			stampindex.Delete(store, reserveScope, stamp),
			chunkstamp.DeleteWithStamp(store, reserveScope, item.Address, stamp),
		)
	}

	return errors.Join(errs,
		store.Delete(item),
		deleteChunkBinItem(store, item),
	)
}

// DeleteCorruptedChunkMetadata removes all reserve index entries for a chunk
// whose Sharky data was found to be corrupted during recovery. It is intended
// to be called from the recovery path, where only a storage.IndexStore (not a
// full transaction.Store) is available. If the chunk has no reserve metadata
// (e.g. it belongs to the upload store or cache), the function is a no-op.
func DeleteCorruptedChunkMetadata(store storage.IndexStore, baseAddr swarm.Address, addr swarm.Address) error {
	stamp, err := chunkstamp.Load(store, reserveScope, addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("load chunkstamp: %w", err)
	}

	stampHash, err := stamp.Hash()
	if err != nil {
		return fmt.Errorf("compute stamp hash: %w", err)
	}

	bin := swarm.Proximity(baseAddr.Bytes(), addr.Bytes())
	batchRadiusItem := &BatchRadiusItem{
		Bin:       bin,
		BatchID:   stamp.BatchID(),
		Address:   addr,
		StampHash: stampHash,
	}
	if err := store.Get(batchRadiusItem); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("get batch radius item: %w", err)
	}

	return errors.Join(
		stampindex.Delete(store, reserveScope, stamp),
		chunkstamp.DeleteWithStamp(store, reserveScope, addr, stamp),
		store.Delete(batchRadiusItem),
		deleteChunkBinItem(store, batchRadiusItem),
	)
}

func (r *Reserve) IterateBin(bin uint8, startBinID uint64, cb func(swarm.Address, uint64, []byte, []byte, []byte) (bool, error)) error {
	err := r.st.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(bin, startBinID),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)
		if item.Bin > bin {
			return true, nil
		}

		stop, err := cb(item.Address, item.BinID, item.BatchID, item.StampHash, item.Sum)
		if stop || err != nil {
			return true, err
		}

		return false, nil
	})

	return err
}

func (r *Reserve) IterateChunks(startBin uint8, cb func(swarm.Chunk) (bool, error)) error {
	err := r.st.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(startBin, 0),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)

		chunk, err := r.st.ChunkStore().Get(context.Background(), item.Address)
		if err != nil {
			return false, err
		}

		stamp, err := chunkstamp.LoadWithStampHash(r.st.IndexStore(), reserveScope, item.Address, item.StampHash)
		if err != nil {
			return false, err
		}

		stop, err := cb(chunk.WithStamp(stamp))
		if stop || err != nil {
			return true, err
		}
		return false, nil
	})

	return err
}

func (r *Reserve) IterateChunksItems(startBin uint8, cb func(*ChunkBinItem) (bool, error)) error {
	err := r.st.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(startBin, 0),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)
		stop, err := cb(item)
		if stop || err != nil {
			return true, err
		}
		return false, nil
	})

	return err
}

// Reset removes all the entries in the reserve. Must be done before any calls to the reserve.
func (r *Reserve) Reset(ctx context.Context) error {
	size := r.Size()

	// step 1: delete epoch timestamp
	err := r.st.Run(ctx, func(s transaction.Store) error { return s.IndexStore().Delete(&EpochItem{}) })
	if err != nil {
		return err
	}

	var eg errgroup.Group
	eg.SetLimit(runtime.NumCPU())

	// step 2: delete batchRadiusItem, chunkBinItem, and the chunk data
	bRitems := make([]*BatchRadiusItem, 0, size)
	err = r.st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &BatchRadiusItem{} },
	}, func(res storage.Result) (bool, error) {
		bRitems = append(bRitems, res.Entry.(*BatchRadiusItem))
		return false, nil
	})
	if err != nil {
		return err
	}
	for _, item := range bRitems {
		eg.Go(safe.RunFunc(r.logger, "reserve-cleanup-delete-chunk", func() error {
			return r.st.Run(ctx, func(s transaction.Store) error {
				return errors.Join(
					s.ChunkStore().Delete(ctx, item.Address),
					s.IndexStore().Delete(item),
					deleteChunkBinItem(s.IndexStore(), item),
				)
			})
		}))
	}

	err = eg.Wait()
	if err != nil {
		return err
	}
	bRitems = nil

	// step 3: delete stampindex and chunkstamp
	sitems := make([]*stampindex.Item, 0, size)
	err = r.st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &stampindex.Item{} },
	}, func(res storage.Result) (bool, error) {
		sitems = append(sitems, res.Entry.(*stampindex.Item))
		return false, nil
	})
	if err != nil {
		return err
	}
	for _, item := range sitems {
		eg.Go(safe.RunFunc(r.logger, "reserve-cleanup-delete-stamp", func() error {
			return r.st.Run(ctx, func(s transaction.Store) error {
				return errors.Join(
					s.IndexStore().Delete(item),
					chunkstamp.DeleteWithStamp(s.IndexStore(), reserveScope, item.ChunkAddress, postage.NewStamp(item.BatchID, item.StampIndex, item.StampTimestamp, nil)),
				)
			})
		}))
	}

	err = eg.Wait()
	if err != nil {
		return err
	}
	sitems = nil

	// step 4: delete binItems
	err = r.st.Run(context.Background(), func(s transaction.Store) error {
		for i := range swarm.MaxBins {
			err := s.IndexStore().Delete(&BinItem{Bin: i})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	r.size.Store(0)

	return nil
}

func (r *Reserve) Radius() uint8 {
	return uint8(r.radius.Load())
}

func (r *Reserve) Size() int {
	return int(r.size.Load())
}

func (r *Reserve) Capacity() int {
	return r.capacity
}

func (r *Reserve) IsWithinCapacity() bool {
	return int(r.size.Load()) <= r.capacity
}

func (r *Reserve) EvictionTarget() int {
	if r.IsWithinCapacity() {
		return 0
	}
	return int(r.size.Load()) - r.capacity
}

func (r *Reserve) SetRadius(rad uint8) error {
	r.radius.Store(uint32(rad))
	r.radiusSetter.SetStorageRadius(rad)
	return r.st.Run(context.Background(), func(s transaction.Store) error {
		return s.IndexStore().Put(&radiusItem{Radius: rad})
	})
}

func (r *Reserve) LastBinIDs() ([]uint64, uint64, error) {
	var epoch EpochItem
	err := r.st.IndexStore().Get(&epoch)
	if err != nil {
		return nil, 0, err
	}

	ids := make([]uint64, swarm.MaxBins)

	for bin := range swarm.MaxBins {
		binItem := &BinItem{Bin: bin}
		err := r.st.IndexStore().Get(binItem)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				ids[bin] = 0
			} else {
				return nil, 0, err
			}
		} else {
			ids[bin] = binItem.BinID
		}
	}

	return ids, epoch.Timestamp, nil
}

func (r *Reserve) IncBinID(store storage.IndexStore, bin uint8) (uint64, error) {
	item := &BinItem{Bin: bin}
	err := store.Get(item)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			item.BinID = 1
			return 1, store.Put(item)
		}

		return 0, err
	}

	item.BinID += 1

	return item.BinID, store.Put(item)
}

func wrappedAddrHex(ch swarm.Chunk) string {
	if ch == nil {
		return ""
	}
	if !soc.Valid(ch) {
		return ""
	}
	sch, err := soc.FromChunk(ch)
	if err != nil {
		return ""
	}
	return sch.WrappedChunk().Address().String()
}
