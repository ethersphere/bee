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

// Reserve Put has to handle multiple possible scenarios.
//  1. Since the same chunk may belong to different postage stamp indices, the reserve will support one chunk to many postage
//     stamp indices relationship.
//  2. A new chunk that shares the same stamp index belonging to the same batch with an already stored chunk will overwrite
//     the existing chunk if the new chunk has a higher stamp timestamp (regardless of batch type).
//  3. A new chunk that has the same address belonging to the same stamp index with an already stored chunk will overwrite the existing chunk
//     if the new chunk has a higher stamp timestamp (regardless of batch type and chunk type, eg CAC & SOC).
//  4. Two different chunk addresses that share the same batch stamp index and timestamp are settled by a tie-break:
//     the lexicographically lower chunk address wins. The loser is rejected; the winner replaces the stored chunk
//     through the usual remove-and-store path (including a fresh bin ID for pullsync).
//  5. Two single owner chunks that share an address under different stamps (any batch or stamp
//     index) settle on one shared payload: a strictly higher stamp timestamp replaces it; equal
//     timestamps are settled by the lexicographically lower stamp hash. Same-stamp divergence remains handled by resolveDivergence above.
func (r *Reserve) Put(ctx context.Context, chunk swarm.Chunk) error {
	socReplaced, err := r.putChunk(ctx, chunk)
	if err != nil {
		return err
	}
	if socReplaced {
		// A single owner chunk's payload is stored once per address while index
		// entries exist per stamp: replacing the payload invalidates the
		// divergence checksums of co-resident entries under other stamps. The
		// refresh runs after the put transaction, with no locks held, because
		// it takes the sibling entries' batch locks (see refreshSiblingSums).
		if err := r.refreshSiblingSums(ctx, chunk.Address()); err != nil {
			return err
		}
		r.logAddressStampState(chunk.Address(), "after_payload_replace_refresh")
		return nil
	}
	return nil
}

// putChunk stores the chunk and reports whether the shared payload of an
// already stored single owner chunk was replaced, in which case the sums of
// co-resident entries must be refreshed by the caller.
func (r *Reserve) putChunk(ctx context.Context, chunk swarm.Chunk) (socReplaced bool, err error) {
	// batchID lock, Put vs Eviction
	r.multx.Lock(string(chunk.Stamp().BatchID()))
	defer r.multx.Unlock(string(chunk.Stamp().BatchID()))

	stampHash, err := chunk.Stamp().Hash()
	if err != nil {
		return false, err
	}

	chunkType := storage.ChunkType(chunk)

	sum, err := storage.ChunkSum(chunk)
	if err != nil {
		return false, err
	}

	bin := swarm.Proximity(r.baseAddr.Bytes(), chunk.Address().Bytes())

	// check if the chunk with the same batch, stamp timestamp and index is already stored
	has, err := r.Has(chunk.Address(), chunk.Stamp().BatchID(), stampHash)
	if err != nil {
		return false, err
	}
	stampTS := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
	batchHex := hex.EncodeToString(chunk.Stamp().BatchID())
	stampHashHex := hex.EncodeToString(stampHash)
	stampIndexHex := hex.EncodeToString(chunk.Stamp().Index())
	sumHex := hex.EncodeToString(sum)
	wrappedHex := wrappedAddrHex(chunk)

	if has {
		// Address, batch and stamp all match, but two single owner chunks can
		// share those and still wrap different content. The sum tells them
		// apart: if it matches we already hold this exact chunk, otherwise the
		// chunks diverge and a tie-break decides which one the neighborhood
		// keeps.
		hasSum, err := r.HasSum(chunk.Address(), sum)
		if err != nil {
			return false, err
		}
		if hasSum {
			r.logger.Debug("putChunk same stamp+sum noop",
				"address", chunk.Address(),
				"batch_id", batchHex,
				"stamp_hash", stampHashHex,
				"stamp_index", stampIndexHex,
				"stamp_timestamp", stampTS,
				"bin", bin,
				"sum", sumHex,
				"wrapped_chunk_address", wrappedHex,
				"chunk_type", chunkType,
			)
			return false, nil
		}
		r.logger.Debug("putChunk same stamp divergent sum, resolveDivergence",
			"address", chunk.Address(),
			"batch_id", batchHex,
			"stamp_hash", stampHashHex,
			"stamp_index", stampIndexHex,
			"stamp_timestamp", stampTS,
			"bin", bin,
			"sum", sumHex,
			"wrapped_chunk_address", wrappedHex,
			"chunk_type", chunkType,
		)
		if err := r.resolveDivergence(ctx, chunk, sum, stampHash, bin, chunkType); err != nil {
			return false, err
		}
		// the tie-break winner replaced the shared payload, so co-resident
		// entries under other stamps need their sums refreshed like on any
		// other single owner chunk replacement.
		return true, nil
	}

	// bin lock
	r.multx.Lock(strconv.Itoa(int(bin)))
	defer r.multx.Unlock(strconv.Itoa(int(bin)))

	var shouldIncReserveSize bool
	path := "putCAC"
	if chunkType == swarm.ChunkTypeSingleOwner {
		path = "putSOC"
		r.logger.Debug("putChunk new stamp, putSOC",
			"address", chunk.Address(),
			"batch_id", batchHex,
			"stamp_hash", stampHashHex,
			"stamp_index", stampIndexHex,
			"stamp_timestamp", stampTS,
			"bin", bin,
			"sum", sumHex,
			"wrapped_chunk_address", wrappedHex,
		)
		socReplaced, shouldIncReserveSize, err = r.putSOC(ctx, chunk, sum, stampHash, bin)
	} else {
		shouldIncReserveSize, err = r.putCAC(ctx, chunk, sum, stampHash, bin)
	}
	if err != nil {
		r.logger.Debug("putChunk failed",
			"error", err,
			"address", chunk.Address(),
			"batch_id", batchHex,
			"stamp_hash", stampHashHex,
			"stamp_index", stampIndexHex,
			"stamp_timestamp", stampTS,
			"bin", bin,
			"sum", sumHex,
			"wrapped_chunk_address", wrappedHex,
			"chunk_type", chunkType,
			"path", path,
		)
		return false, err
	}
	if shouldIncReserveSize {
		r.size.Add(1)
	}
	return socReplaced, nil
}

func (r *Reserve) putSOC(ctx context.Context, chunk swarm.Chunk, sum, stampHash []byte, bin uint8) (socReplaced, shouldInc bool, err error) {
	err = r.st.Run(ctx, func(s transaction.Store) error {
		oldStampIndex, loaded, err := stampindex.LoadOrStore(s.IndexStore(), reserveScope, chunk)
		if err != nil {
			return fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
		}

		if loaded {
			sameAddr, err := r.resolveStampIndexCollision(ctx, s, chunk, oldStampIndex, sum, stampHash, bin)
			if err != nil {
				r.logger.Debug("putSOC stamp index collision rejected",
					"error", err,
					"address", chunk.Address(),
					"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
					"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
				)
				return err
			}
			if sameAddr {
				r.logger.Debug("replacing soc in chunkstore",
					"address", chunk.Address(),
					"reason", "same_address_stamp_index_collision",
					"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
					"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
				)
				socReplaced = true
				return s.ChunkStore().Replace(ctx, chunk, false)
			}
		}

		if err := r.storeReserveEntries(s, chunk, sum, stampHash, bin); err != nil {
			return err
		}

		has, err := s.ChunkStore().Has(ctx, chunk.Address())
		if err != nil {
			return err
		}
		if has {
			r.logger.Debug("replacing soc in chunkstore",
				"address", chunk.Address(),
				"reason", "cross_stamp_payload_replace",
				"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
				"stamp_hash", hex.EncodeToString(stampHash),
				"stamp_index", hex.EncodeToString(chunk.Stamp().Index()),
				"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
				"bin", bin,
				"sum", hex.EncodeToString(sum),
				"wrapped_chunk_address", wrappedAddrHex(chunk),
			)
			socReplaced = true
			err = s.ChunkStore().Replace(ctx, chunk, true)
		} else {
			r.logger.Debug("storing new soc in chunkstore",
				"address", chunk.Address(),
				"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
				"stamp_hash", hex.EncodeToString(stampHash),
				"stamp_index", hex.EncodeToString(chunk.Stamp().Index()),
				"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
				"bin", bin,
				"sum", hex.EncodeToString(sum),
				"wrapped_chunk_address", wrappedAddrHex(chunk),
			)
			err = s.ChunkStore().Put(ctx, chunk)
		}
		if err != nil {
			return err
		}

		shouldInc = !loaded
		return nil
	})
	if err == nil && (socReplaced || shouldInc) {
		r.logAddressStampState(chunk.Address(), "after_putSOC")
	}
	return
}

func (r *Reserve) putCAC(ctx context.Context, chunk swarm.Chunk, sum, stampHash []byte, bin uint8) (shouldInc bool, err error) {
	err = r.st.Run(ctx, func(s transaction.Store) error {
		oldStampIndex, loaded, err := stampindex.LoadOrStore(s.IndexStore(), reserveScope, chunk)
		if err != nil {
			return fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
		}

		if loaded {
			sameAddr, err := r.resolveStampIndexCollision(ctx, s, chunk, oldStampIndex, sum, stampHash, bin)
			if err != nil {
				return err
			}
			if sameAddr {
				return nil
			}
		}

		if err := r.storeReserveEntries(s, chunk, sum, stampHash, bin); err != nil {
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

// resolveStampIndexCollision settles a stamp-index slot collision found by
// LoadOrStore (same batchID and stamp index already occupied). On success it
// returns sameAddr to tell the caller what remains:
//
//  1. sameAddr=true: the stored entry has the same chunk address. Old reserve
//     index entries for that stamp are replaced in place. The caller only
//     performs the type-specific chunkstore action (SOC Replace, CAC no-op).
//  2. sameAddr=false: the stored entry points at a different address. That
//     chunk and its reserve metadata are removed and the stamp index is
//     rewritten. The caller must still call storeReserveEntries and write the
//     new chunk to the chunkstore.
func (r *Reserve) resolveStampIndexCollision(
	ctx context.Context, s transaction.Store,
	chunk swarm.Chunk, oldStampIndex *stampindex.Item,
	sum, stampHash []byte, bin uint8,
) (sameAddr bool, err error) {
	prev := binary.BigEndian.Uint64(oldStampIndex.StampTimestamp)
	curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
	if prev > curr {
		return false, fmt.Errorf("overwrite same chunk. prev %d cur %d batch %s: %w", prev, curr, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
	}

	// Same stamp index and timestamp, different chunk addresses: both
	// claims are otherwise valid, so settle on the lower address.
	if prev == curr && !oldStampIndex.ChunkAddress.Equal(chunk.Address()) {
		if bytes.Compare(chunk.Address().Bytes(), oldStampIndex.ChunkAddress.Bytes()) >= 0 {
			r.logger.Debug(
				"discarding stamp index collision",
				"old_chunk", oldStampIndex.ChunkAddress,
				"new_chunk", chunk.Address(),
				"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
				"stamp_index", hex.EncodeToString(chunk.Stamp().Index()),
				"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
				"incoming_stamp_hash", hex.EncodeToString(stampHash),
				"stored_stamp_hash", hex.EncodeToString(oldStampIndex.StampHash),
			)
			return false, fmt.Errorf(
				"stamp index collision chunk %s lost tie-break: %w",
				chunk.Address(),
				storage.ErrDivergentChunkRejected,
			)
		}
		r.logger.Debug(
			"replacing stamp index collision",
			"old_chunk", oldStampIndex.ChunkAddress,
			"new_chunk", chunk.Address(),
			"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
			"stamp_index", hex.EncodeToString(chunk.Stamp().Index()),
			"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
			"incoming_stamp_hash", hex.EncodeToString(stampHash),
			"stored_stamp_hash", hex.EncodeToString(oldStampIndex.StampHash),
		)
	} else {
		r.logger.Debug(
			"replacing chunk stamp index",
			"old_chunk", oldStampIndex.ChunkAddress,
			"new_chunk", chunk.Address(),
			"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
		)
	}

	if oldStampIndex.ChunkAddress.Equal(chunk.Address()) {
		// Same address, same timestamp, same batch id: settle on the lower stamp hash.
		// Paranoid check: normally such stamps are invalid (invalid signature) and rejected earlier.
		if prev == curr && bytes.Compare(oldStampIndex.StampHash, stampHash) <= 0 {
			return false, fmt.Errorf(
				"stamp index collision chunk %s lost stamp-hash tie-break: %w",
				chunk.Address(),
				storage.ErrOverwriteNewerChunk,
			)
		}

		oldStamp, err := chunkstamp.LoadWithStampHash(s.IndexStore(), reserveScope, oldStampIndex.ChunkAddress, oldStampIndex.StampHash)
		if err != nil {
			return false, err
		}

		oldBatchRadiusItem := &BatchRadiusItem{
			Bin:       bin,
			Address:   oldStampIndex.ChunkAddress,
			BatchID:   oldStampIndex.BatchID,
			StampHash: oldStampIndex.StampHash,
		}
		err = s.IndexStore().Get(oldBatchRadiusItem)
		if err != nil {
			return false, err
		}

		err = errors.Join(
			s.IndexStore().Delete(oldBatchRadiusItem),
			deleteChunkBinItem(s.IndexStore(), oldBatchRadiusItem.Bin, oldBatchRadiusItem.BinID),
			stampindex.Delete(s.IndexStore(), reserveScope, oldStamp),
			chunkstamp.DeleteWithStamp(s.IndexStore(), reserveScope, oldBatchRadiusItem.Address, oldStamp),
		)
		if err != nil {
			return false, err
		}

		err = errors.Join(
			stampindex.Store(s.IndexStore(), reserveScope, chunk),
			r.storeReserveEntries(s, chunk, sum, stampHash, bin),
		)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	// An older and different chunk with the same batchID and stamp index has been previously
	// saved to the reserve. We must do the below before saving the new chunk:
	// 1. Delete the old chunk from the chunkstore.
	// 2. Delete the old chunk's stamp data.
	// 3. Delete ALL old chunk related items from the reserve.
	// 4. Update the stamp index.

	err = r.removeChunk(ctx, s, oldStampIndex.ChunkAddress, oldStampIndex.BatchID, oldStampIndex.StampHash)
	if err != nil {
		return false, fmt.Errorf("failed removing older chunk %s: %w", oldStampIndex.ChunkAddress, err)
	}

	err = stampindex.Store(s.IndexStore(), reserveScope, chunk)
	if err != nil {
		return false, fmt.Errorf("failed updating stamp index: %w", err)
	}

	return false, nil
}

// storeReserveEntries writes the common set of reserve index entries for a
// chunk: chunkstamp, BatchRadiusItem, ChunkBinItem and ChunkSumItem and allocates a fresh bin ID via IncBinID.
// The stamp index is NOT written here because its lifecycle differs across call sites (LoadOrStore vs explicit Store after collision cleanup).
func (r *Reserve) storeReserveEntries(s transaction.Store, chunk swarm.Chunk, sum, stampHash []byte, bin uint8) error {
	chunkType := storage.ChunkType(chunk)
	binID, err := r.IncBinID(s.IndexStore(), bin)
	if err != nil {
		return err
	}

	r.logger.Debug("storeReserveEntries allocated bin_id",
		"address", chunk.Address(),
		"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
		"stamp_hash", hex.EncodeToString(stampHash),
		"stamp_index", hex.EncodeToString(chunk.Stamp().Index()),
		"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
		"bin", bin,
		"bin_id", binID,
		"sum", hex.EncodeToString(sum),
		"wrapped_chunk_address", wrappedAddrHex(chunk),
		"chunk_type", chunkType,
	)

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
		s.IndexStore().Put(&ChunkSumItem{Address: chunk.Address(), Sum: sum}),
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

				sum, err := storage.ChunkSumFromParts(stamp.BatchID(), stampHash, chunk)
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
						"address", addr,
						"batch_id", hex.EncodeToString(stamp.BatchID()),
						"stamp_hash", hex.EncodeToString(stampHash),
						"stamp_index", hex.EncodeToString(stamp.Index()),
						"stamp_timestamp", binary.BigEndian.Uint64(stamp.Timestamp()),
						"bin", bin,
						"bin_id", item.BinID,
						"sum", hex.EncodeToString(sum),
						"wrapped_chunk_address", wrappedAddrHex(chunk),
					)
					return nil
				}

				oldSum := cbi.Sum
				cbi.Sum = sum
				r.logger.Debug("refreshSiblingSums updating sum",
					"address", addr,
					"batch_id", hex.EncodeToString(stamp.BatchID()),
					"stamp_hash", hex.EncodeToString(stampHash),
					"stamp_index", hex.EncodeToString(stamp.Index()),
					"stamp_timestamp", binary.BigEndian.Uint64(stamp.Timestamp()),
					"bin", bin,
					"bin_id", item.BinID,
					"old_sum", hex.EncodeToString(oldSum),
					"new_sum", hex.EncodeToString(sum),
					"wrapped_chunk_address", wrappedAddrHex(chunk),
				)
				return errors.Join(
					s.IndexStore().Delete(&ChunkSumItem{Address: addr, Sum: oldSum}),
					s.IndexStore().Put(cbi),
					s.IndexStore().Put(&ChunkSumItem{Address: addr, Sum: sum}),
				)
			})
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// resolveDivergence settles two single owner chunks that share an address,
// batch and stamp but wrap different content. Both are individually valid, so
// the protocol cannot pick between them; the choice is made here, in the
// storage layer, by a tie-break that depends only on the two payloads. Every
// node in the neighborhood therefore converges on the same chunk no matter
// which one it received first.
//
// If the incoming chunk wins it replaces the stored one in place, reusing the
// existing stamp index and stamp entries, which are identical for both. The
// bin ID is bumped so that peers which already synced past the old bin ID are
// offered the replacement, propagating the resolution outwards.
//
// The reserve size is unchanged either way: one chunk goes in, one comes out.
func (r *Reserve) resolveDivergence(
	ctx context.Context,
	chunk swarm.Chunk,
	sum []byte,
	stampHash []byte,
	bin uint8,
	chunkType swarm.ChunkType,
) error {
	// bin lock
	r.multx.Lock(strconv.Itoa(int(bin)))
	defer r.multx.Unlock(strconv.Itoa(int(bin)))

	return r.st.Run(ctx, func(s transaction.Store) error {
		stored, err := s.ChunkStore().Get(ctx, chunk.Address())
		if err != nil {
			return fmt.Errorf("failed loading diverging chunk %s: %w", chunk.Address(), err)
		}
		// ChunkStore returns payload only; stamp is in the chunkstamp index.
		// stampHash is the same key Has() already confirmed for this put.
		stamp, err := chunkstamp.LoadWithStampHash(s.IndexStore(), reserveScope, chunk.Address(), stampHash)
		if err != nil {
			return fmt.Errorf("failed loading stamp for diverging chunk %s: %w", chunk.Address(), err)
		}
		stored = stored.WithStamp(stamp)

		// Verify timestamp precedence: an incoming chunk with an older timestamp
		// can never displace a stored chunk.
		prevTimestamp := binary.BigEndian.Uint64(stored.Stamp().Timestamp())
		currTimestamp := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
		if prevTimestamp > currTimestamp {
			return fmt.Errorf("overwrite same chunk. prev %d cur %d batch %s: %w", prevTimestamp, currTimestamp, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
		}

		// At equal timestamp, if the stamps differ, the lower stamp hash wins.
		if prevTimestamp == currTimestamp {
			storedStampHash, err := stored.Stamp().Hash()
			if err != nil {
				return err
			}
			if !bytes.Equal(storedStampHash, stampHash) && bytes.Compare(storedStampHash, stampHash) < 0 {
				r.logger.Debug(
					"discarding diverging chunk (weaker stamp hash at equal timestamp)",
					"address", chunk.Address(),
					"stored_stamp_hash", hex.EncodeToString(storedStampHash),
					"incoming_stamp_hash", hex.EncodeToString(stampHash),
				)
				return fmt.Errorf("diverging chunk %s lost stamp-hash tie-break: %w", chunk.Address(), storage.ErrDivergentChunkRejected)
			}
		}

		wins, err := storage.DivergentChunkWins(stored, chunk)
		if err != nil {
			return fmt.Errorf("divergence tie-break for chunk %s: %w", chunk.Address(), err)
		}

		storedSum, _ := storage.ChunkSum(stored)
		storedWrapped := wrappedAddrHex(stored)
		incomingWrapped := wrappedAddrHex(chunk)

		if !wins {
			r.logger.Debug(
				"discarding diverging chunk",
				"address", chunk.Address(),
				"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
				"stamp_hash", hex.EncodeToString(stampHash),
				"stamp_index", hex.EncodeToString(chunk.Stamp().Index()),
				"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
				"bin", bin,
				"stored_sum", hex.EncodeToString(storedSum),
				"incoming_sum", hex.EncodeToString(sum),
				"stored_wrapped_chunk_address", storedWrapped,
				"incoming_wrapped_chunk_address", incomingWrapped,
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
		if err := deleteChunkBinItem(s.IndexStore(), item.Bin, item.BinID); err != nil {
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
			"stamp_hash", hex.EncodeToString(stampHash),
			"stamp_index", hex.EncodeToString(chunk.Stamp().Index()),
			"stamp_timestamp", binary.BigEndian.Uint64(chunk.Stamp().Timestamp()),
			"bin", bin,
			"old_bin_id", item.BinID,
			"new_bin_id", binID,
			"stored_sum", hex.EncodeToString(storedSum),
			"incoming_sum", hex.EncodeToString(sum),
			"stored_wrapped_chunk_address", storedWrapped,
			"incoming_wrapped_chunk_address", incomingWrapped,
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
				ChunkType: chunkType,
				StampHash: stampHash,
				Sum:       sum,
			}),
			s.IndexStore().Put(&ChunkSumItem{Address: chunk.Address(), Sum: sum}),
		)
		if err != nil {
			return err
		}

		// swap the payload without touching the reference count: the chunk
		// store entry is reused, only its content changes.
		return s.ChunkStore().Replace(ctx, chunk, false)
	})
}

func (r *Reserve) Has(addr swarm.Address, batchID []byte, stampHash []byte) (bool, error) {
	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr, StampHash: stampHash}
	return r.st.IndexStore().Has(item)
}

// HasSum reports whether the reserve holds a chunk at addr whose pullsync
// divergence checksum equals sum. Used by the pullsync want-decision.
func (r *Reserve) HasSum(addr swarm.Address, sum []byte) (bool, error) {
	return r.st.IndexStore().Has(&ChunkSumItem{Address: addr, Sum: sum})
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

// deleteChunkBinItem removes the ChunkBinItem identified by (bin, binID)
// together with its companion ChunkSumItem, keeping the pullsync sum index
// consistent. It is a no-op if the ChunkBinItem does not exist.
func deleteChunkBinItem(store storage.IndexStore, bin uint8, binID uint64) error {
	cbi := &ChunkBinItem{Bin: bin, BinID: binID}
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
			return store.Delete(&ChunkBinItem{Bin: bin, BinID: binID})
		}
		return err
	}
	return errors.Join(
		store.Delete(cbi),
		store.Delete(&ChunkSumItem{Address: cbi.Address, Sum: cbi.Sum}),
	)
}

// RemoveMalformedChunkBinItems deletes chunkBin entries whose stored value
// does not match the current serialization, without unmarshaling them. Such
// entries are orphaned pre-Sum records (a ChunkBinItem without a matching
// BatchRadiusItem is never rewritten by the Sum backfill migration) and would
// otherwise fail every full iteration of the namespace: the reserve sampler,
// pullsync bin subscriptions and the reserve repairer. They have no companion
// ChunkSumItem to remove. Returns the number of entries deleted.
func RemoveMalformedChunkBinItems(ctx context.Context, st transaction.Storage) (int, error) {
	var malformed []*ChunkBinItem
	err := st.IndexStore().Iterate(storage.Query{
		Factory:      func() storage.Item { return &ChunkBinItem{} },
		ItemProperty: storage.QueryItemSize,
	}, func(res storage.Result) (bool, error) {
		if res.Size == chunkBinItemSize {
			return false, nil
		}
		bin, binID, err := ParseChunkBinID(res.ID)
		if err != nil {
			return false, err
		}
		malformed = append(malformed, &ChunkBinItem{Bin: bin, BinID: binID})
		return false, nil
	})
	if err != nil {
		return 0, err
	}

	const batchSize = 1000
	for i := 0; i < len(malformed); i += batchSize {
		end := min(i+batchSize, len(malformed))
		err := st.Run(ctx, func(s transaction.Store) error {
			for _, item := range malformed[i:end] {
				if err := s.IndexStore().Delete(item); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return len(malformed), nil
}

func RemoveChunkWithItem(
	ctx context.Context,
	trx transaction.Store,
	item *BatchRadiusItem,
) error {
	var errs error

	stamp, _ := chunkstamp.LoadWithStampHash(trx.IndexStore(), reserveScope, item.Address, item.StampHash)
	if stamp != nil {
		errs = errors.Join(
			stampindex.Delete(trx.IndexStore(), reserveScope, stamp),
			chunkstamp.DeleteWithStamp(trx.IndexStore(), reserveScope, item.Address, stamp),
		)
	}

	return errors.Join(errs,
		trx.IndexStore().Delete(item),
		deleteChunkBinItem(trx.IndexStore(), item.Bin, item.BinID),
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
	var errs error

	stamp, _ := chunkstamp.LoadWithStampHash(trx.IndexStore(), reserveScope, item.Address, item.StampHash)
	if stamp != nil {
		errs = errors.Join(
			stampindex.Delete(trx.IndexStore(), reserveScope, stamp),
			chunkstamp.DeleteWithStamp(trx.IndexStore(), reserveScope, item.Address, stamp),
		)
	}

	return errors.Join(errs,
		trx.IndexStore().Delete(item),
		deleteChunkBinItem(trx.IndexStore(), item.Bin, item.BinID),
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
		deleteChunkBinItem(store, bin, batchRadiusItem.BinID),
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
					deleteChunkBinItem(s.IndexStore(), item.Bin, item.BinID),
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

// logAddressStampState dumps every stamp index entry at addr for diagnostic runs.
func (r *Reserve) logAddressStampState(addr swarm.Address, event string) {
	bin := swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes())
	payloadWrapped := ""
	if ch, err := r.st.ChunkStore().Get(context.Background(), addr); err == nil {
		payloadWrapped = wrappedAddrHex(ch)
	}

	err := chunkstamp.IterateAll(r.st.IndexStore(), reserveScope, addr, func(stamp swarm.Stamp) (bool, error) {
		stampHash, err := stamp.Hash()
		if err != nil {
			// nolint:nilerr
			return false, nil
		}
		item := &BatchRadiusItem{Bin: bin, BatchID: stamp.BatchID(), Address: addr, StampHash: stampHash}
		if err := r.st.IndexStore().Get(item); err != nil {
			r.logger.Debug("stamp state entry missing batch radius",
				"event", event,
				"address", addr,
				"batch_id", hex.EncodeToString(stamp.BatchID()),
				"stamp_hash", hex.EncodeToString(stampHash),
				"stamp_index", hex.EncodeToString(stamp.Index()),
				"stamp_timestamp", binary.BigEndian.Uint64(stamp.Timestamp()),
				"error", err,
			)
			return false, nil
		}
		cbi := &ChunkBinItem{Bin: bin, BinID: item.BinID}
		sumHex := ""
		if err := r.st.IndexStore().Get(cbi); err == nil {
			sumHex = hex.EncodeToString(cbi.Sum)
		}
		r.logger.Debug("stamp state entry",
			"event", event,
			"address", addr,
			"payload_wrapped_chunk_address", payloadWrapped,
			"batch_id", hex.EncodeToString(stamp.BatchID()),
			"stamp_hash", hex.EncodeToString(stampHash),
			"stamp_index", hex.EncodeToString(stamp.Index()),
			"stamp_timestamp", binary.BigEndian.Uint64(stamp.Timestamp()),
			"bin", bin,
			"bin_id", item.BinID,
			"sum", sumHex,
		)
		return false, nil
	})
	if err != nil {
		r.logger.Debug("stamp state iterate failed", "event", event, "address", addr, "error", err)
	}
}
