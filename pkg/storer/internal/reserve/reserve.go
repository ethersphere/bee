// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
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
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstamp"
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
func (r *Reserve) Put(ctx context.Context, chunk swarm.Chunk) error {

	// batchID lock, Put vs Eviction
	r.multx.Lock(string(chunk.Stamp().BatchID()))
	defer r.multx.Unlock(string(chunk.Stamp().BatchID()))

	stampHash, err := chunk.Stamp().Hash()
	if err != nil {
		return err
	}

	// check if the chunk with the same batch, stamp timestamp and index is already stored
	has, err := r.Has(chunk.Address(), chunk.Stamp().BatchID(), stampHash)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	chunkType := storage.ChunkType(chunk)

	bin := swarm.Proximity(r.baseAddr.Bytes(), chunk.Address().Bytes())

	// bin lock
	r.multx.Lock(strconv.Itoa(int(bin)))
	defer r.multx.Unlock(strconv.Itoa(int(bin)))

	var shouldIncReserveSize bool

	err = r.st.Run(ctx, func(s transaction.Store) error {

		oldStampIndex, loadedStampIndex, err := stampindex.LoadOrStore(s.IndexStore(), reserveScope, chunk)
		if err != nil {
			return fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
		}

		// index collision
		if loadedStampIndex {

			prev := binary.BigEndian.Uint64(oldStampIndex.StampTimestamp)
			curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
			if prev >= curr {
				return fmt.Errorf("overwrite same chunk. prev %d cur %d batch %s: %w", prev, curr, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
			}

			r.logger.Debug(
				"replacing chunk stamp index",
				"old_chunk", oldStampIndex.ChunkAddress,
				"new_chunk", chunk.Address(),
				"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
			)

			// same chunk address
			if oldStampIndex.ChunkAddress.Equal(chunk.Address()) {

				oldStamp, err := chunkstamp.LoadWithStampHash(s.IndexStore(), reserveScope, oldStampIndex.ChunkAddress, oldStampIndex.StampHash)
				if err != nil {
					return err
				}

				oldBatchRadiusItem := &BatchRadiusItem{
					Bin:       bin,
					Address:   oldStampIndex.ChunkAddress,
					BatchID:   oldStampIndex.BatchID,
					StampHash: oldStampIndex.StampHash,
				}
				// load item to get the binID
				err = s.IndexStore().Get(oldBatchRadiusItem)
				if err != nil {
					return err
				}

				// delete old chunk index items
				err = errors.Join(
					s.IndexStore().Delete(oldBatchRadiusItem),
					s.IndexStore().Delete(&ChunkBinItem{Bin: oldBatchRadiusItem.Bin, BinID: oldBatchRadiusItem.BinID}),
					stampindex.Delete(s.IndexStore(), reserveScope, oldStamp),
					chunkstamp.DeleteWithStamp(s.IndexStore(), reserveScope, oldBatchRadiusItem.Address, oldStamp),
				)
				if err != nil {
					return err
				}

				binID, err := r.IncBinID(s.IndexStore(), bin)
				if err != nil {
					return err
				}

				err = errors.Join(
					stampindex.Store(s.IndexStore(), reserveScope, chunk),
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
					}),
				)
				if err != nil {
					return err
				}

				if chunkType == swarm.ChunkTypeSingleOwner {
					r.logger.Debug("replacing soc in chunkstore", "address", chunk.Address())
					return s.ChunkStore().Replace(ctx, chunk, false)
				}

				return nil
			}

			// An older and different chunk with the same batchID and stamp index has been previously
			// saved to the reserve. We must do the below before saving the new chunk:
			// 1. Delete the old chunk from the chunkstore.
			// 2. Delete the old chunk's stamp data.
			// 3. Delete ALL old chunk related items from the reserve.
			// 4. Update the stamp index.

			err = r.removeChunk(ctx, s, oldStampIndex.ChunkAddress, oldStampIndex.BatchID, oldStampIndex.StampHash)
			if err != nil {
				return fmt.Errorf("failed removing older chunk %s: %w", oldStampIndex.ChunkAddress, err)
			}

			// replace old stamp index.
			err = stampindex.Store(s.IndexStore(), reserveScope, chunk)
			if err != nil {
				return fmt.Errorf("failed updating stamp index: %w", err)
			}
		}

		binID, err := r.IncBinID(s.IndexStore(), bin)
		if err != nil {
			return err
		}

		err = errors.Join(
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
			}),
		)
		if err != nil {
			return err
		}

		var has bool
		if chunkType == swarm.ChunkTypeSingleOwner {
			has, err = s.ChunkStore().Has(ctx, chunk.Address())
			if err != nil {
				return err
			}
			if has {
				r.logger.Debug("replacing soc in chunkstore", "address", chunk.Address())
				err = s.ChunkStore().Replace(ctx, chunk, true)
			} else {
				err = s.ChunkStore().Put(ctx, chunk)
			}
		} else {
			err = s.ChunkStore().Put(ctx, chunk)
		}

		if err != nil {
			return err
		}

		if !loadedStampIndex {
			shouldIncReserveSize = true
		}

		return nil
	})
	if err != nil {
		return err
	}
	if shouldIncReserveSize {
		r.size.Add(1)
	}
	return nil
}

func (r *Reserve) Has(addr swarm.Address, batchID []byte, stampHash []byte) (bool, error) {
	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr, StampHash: stampHash}
	return r.st.IndexStore().Has(item)
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
func (r *Reserve) EvictBatchBin(
	ctx context.Context,
	batchID []byte,
	count int,
	bin uint8,
) (int, error) {
	r.multx.Lock(string(batchID))
	defer r.multx.Unlock(string(batchID))

	var evicteditems []*BatchRadiusItem

	if count <= 0 {
		return 0, nil
	}

	err := r.st.IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return &BatchRadiusItem{} },
		Prefix:  string(batchID),
	}, func(res storage.Result) (bool, error) {
		batchRadius := res.Entry.(*BatchRadiusItem)
		if batchRadius.Bin >= bin {
			return true, nil
		}
		evicteditems = append(evicteditems, batchRadius)
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

	for _, item := range evicteditems {
		func(item *BatchRadiusItem) {
			eg.Go(func() error {
				err := r.st.Run(ctx, func(s transaction.Store) error {
					return RemoveChunkWithItem(ctx, s, item)
				})
				if err != nil {
					return err
				}
				evicted.Add(1)
				return nil
			})
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
		trx.IndexStore().Delete(&ChunkBinItem{Bin: item.Bin, BinID: item.BinID}),
		trx.ChunkStore().Delete(ctx, item.Address),
	)
}

func (r *Reserve) IterateBin(bin uint8, startBinID uint64, cb func(swarm.Address, uint64, []byte, []byte) (bool, error)) error {
	err := r.st.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(bin, startBinID),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)
		if item.Bin > bin {
			return true, nil
		}

		stop, err := cb(item.Address, item.BinID, item.BatchID, item.StampHash)
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

// Reset removes all the entires in the reserve. Must be done before any calls to the reserve.
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
		eg.Go(func() error {
			return r.st.Run(ctx, func(s transaction.Store) error {
				return errors.Join(
					s.ChunkStore().Delete(ctx, item.Address),
					s.IndexStore().Delete(item),
					s.IndexStore().Delete(&ChunkBinItem{Bin: item.Bin, BinID: item.BinID}),
				)
			})
		})
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
		eg.Go(func() error {
			return r.st.Run(ctx, func(s transaction.Store) error {
				return errors.Join(
					s.IndexStore().Delete(item),
					chunkstamp.DeleteWithStamp(s.IndexStore(), reserveScope, item.ChunkAddress, postage.NewStamp(item.BatchID, item.StampIndex, item.StampTimestamp, nil)),
				)
			})
		})
	}

	err = eg.Wait()
	if err != nil {
		return err
	}
	sitems = nil

	// step 4: delete binItems
	err = r.st.Run(context.Background(), func(s transaction.Store) error {
		for i := uint8(0); i < swarm.MaxBins; i++ {
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

	for bin := uint8(0); bin < swarm.MaxBins; bin++ {
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
