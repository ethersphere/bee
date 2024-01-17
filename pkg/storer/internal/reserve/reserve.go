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
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"resenje.org/multex"
)

const reserveNamespace = "reserve"

type Reserve struct {
	baseAddr     swarm.Address
	radiusSetter topology.SetStorageRadiuser
	logger       log.Logger

	capacity int
	size     atomic.Int64
	radius   atomic.Uint32

	binMtx sync.Mutex
	mutx   *multex.Multex
}

func New(
	baseAddr swarm.Address,
	store storage.Store,
	capacity int,
	radiusSetter topology.SetStorageRadiuser,
	logger log.Logger,
) (*Reserve, error) {

	rs := &Reserve{
		baseAddr:     baseAddr,
		capacity:     capacity,
		radiusSetter: radiusSetter,
		logger:       logger.WithName(reserveNamespace).Register(),
		mutx:         multex.New(),
	}

	rItem := &radiusItem{}
	err := store.Get(rItem)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	rs.radius.Store(uint32(rItem.Radius))

	epochItem := &EpochItem{}
	err = store.Get(epochItem)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			err := store.Put(&EpochItem{Timestamp: uint64(time.Now().Unix())})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	size, err := store.Count(&BatchRadiusItem{})
	if err != nil {
		return nil, err
	}
	rs.size.Store(int64(size))

	return rs, nil
}

// Put stores a new chunk in the reserve and returns if the reserve size should increase.
func (r *Reserve) Put(ctx context.Context, store internal.Storage, chunk swarm.Chunk) error {
	indexStore := store.IndexStore()
	chunkStore := store.ChunkStore()

	unlock := r.lock(chunk.Address(), chunk.Stamp().BatchID())
	defer unlock()

	po := swarm.Proximity(r.baseAddr.Bytes(), chunk.Address().Bytes())

	has, err := indexStore.Has(&BatchRadiusItem{
		Bin:     po,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	})
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	storeBatch, err := indexStore.Batch(ctx)
	if err != nil {
		return err
	}

	newStampIndex := true

	switch item, loaded, err := stampindex.LoadOrStore(
		indexStore,
		storeBatch,
		reserveNamespace,
		chunk,
	); {
	case err != nil:
		return fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
	case loaded && item.ChunkIsImmutable:
		return fmt.Errorf("batch %s index %s: %w", hex.EncodeToString(chunk.Stamp().BatchID()), hex.EncodeToString(chunk.Stamp().Index()), storage.ErrOverwriteOfImmutableBatch)
	case loaded && !item.ChunkIsImmutable:
		prev := binary.BigEndian.Uint64(item.StampTimestamp)
		curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
		if prev >= curr {
			return fmt.Errorf("overwrite prev %d cur %d batch %s :%w", prev, curr, hex.EncodeToString(chunk.Stamp().BatchID()), storage.ErrOverwriteNewerChunk)
		}
		// An older and different chunk with the same batchID and stamp index has been previously
		// saved to the reserve. We must do the below before saving the new chunk:
		// 1. Delete the old chunk from the chunkstore.
		// 2. Delete the old chunk's stamp data.
		// 3. Delete ALL old chunk related items from the reserve.
		// 4. Update the stamp index.
		newStampIndex = false

		err := r.removeChunk(ctx, store, storeBatch, item.ChunkAddress, chunk.Stamp().BatchID())
		if err != nil {
			return fmt.Errorf("failed removing older chunk: %w", err)
		}

		r.logger.Debug(
			"replacing chunk stamp index",
			"old_chunk", item.ChunkAddress,
			"new_chunk", chunk.Address(),
			"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
		)

		err = stampindex.Store(storeBatch, reserveNamespace, chunk)
		if err != nil {
			return fmt.Errorf("failed updating stamp index: %w", err)
		}
	}

	err = chunkstamp.Store(storeBatch, reserveNamespace, chunk)
	if err != nil {
		return err
	}

	binID, err := r.IncBinID(indexStore, po)
	if err != nil {
		return err
	}

	err = storeBatch.Put(&BatchRadiusItem{
		Bin:     po,
		BinID:   binID,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	})
	if err != nil {
		return err
	}

	err = storeBatch.Put(&ChunkBinItem{
		Bin:       po,
		BinID:     binID,
		Address:   chunk.Address(),
		BatchID:   chunk.Stamp().BatchID(),
		ChunkType: ChunkType(chunk),
	})
	if err != nil {
		return err
	}

	err = chunkStore.Put(ctx, chunk)
	if err != nil {
		return err
	}

	err = storeBatch.Commit()
	if err != nil {
		return err
	}

	if newStampIndex {
		r.size.Add(1)
	}

	return nil
}

func (r *Reserve) Has(store storage.Store, addr swarm.Address, batchID []byte) (bool, error) {
	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr}
	return store.Has(item)
}

func (r *Reserve) Get(ctx context.Context, storage internal.Storage, addr swarm.Address, batchID []byte) (swarm.Chunk, error) {

	unlock := r.lock(addr, batchID)
	defer unlock()

	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr}
	err := storage.IndexStore().Get(item)
	if err != nil {
		return nil, err
	}

	stamp, err := chunkstamp.LoadWithBatchID(storage.IndexStore(), reserveNamespace, addr, item.BatchID)
	if err != nil {
		return nil, err
	}

	ch, err := storage.ChunkStore().Get(ctx, addr)
	if err != nil {
		return nil, err
	}

	return ch.WithStamp(stamp), nil
}

func (r *Reserve) IterateBin(store storage.Store, bin uint8, startBinID uint64, cb func(swarm.Address, uint64, []byte) (bool, error)) error {
	err := store.Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(bin, startBinID),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)
		if item.Bin > bin {
			return true, nil
		}

		stop, err := cb(item.Address, item.BinID, item.BatchID)
		if stop || err != nil {
			return true, err
		}

		return false, nil
	})

	return err
}

func (r *Reserve) IterateChunks(store internal.Storage, startBin uint8, cb func(swarm.Chunk) (bool, error)) error {
	err := store.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(startBin, 0),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)

		chunk, err := store.ChunkStore().Get(context.Background(), item.Address)
		if err != nil {
			return false, err
		}

		stamp, err := chunkstamp.LoadWithBatchID(store.IndexStore(), reserveNamespace, item.Address, item.BatchID)
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

type ChunkItem struct {
	ChunkAddress swarm.Address
	BatchID      []byte
	Type         swarm.ChunkType
	BinID        uint64
	Bin          uint8
}

func (r *Reserve) IterateChunksItems(store internal.Storage, startBin uint8, cb func(ChunkItem) (bool, error)) error {
	err := store.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &ChunkBinItem{} },
		Prefix:        binIDToString(startBin, 0),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*ChunkBinItem)

		chItem := ChunkItem{
			ChunkAddress: item.Address,
			BatchID:      item.BatchID,
			Type:         item.ChunkType,
			BinID:        item.BinID,
			Bin:          item.Bin,
		}

		stop, err := cb(chItem)
		if stop || err != nil {
			return true, err
		}
		return false, nil
	})

	return err
}

// EvictBatchBin evicts all chunks from bins upto the bin provided.
func (r *Reserve) EvictBatchBin(
	ctx context.Context,
	txExecutor internal.TxExecutor,
	batchID []byte,
	count int,
	bin uint8,
) (int, error) {

	unlock := r.lock(swarm.ZeroAddress, batchID)
	defer unlock()

	var evicted []*BatchRadiusItem

	if count <= 0 {
		return 0, nil
	}

	err := txExecutor.Execute(ctx, func(store internal.Storage) error {
		return store.IndexStore().Iterate(storage.Query{
			Factory: func() storage.Item { return &BatchRadiusItem{} },
			Prefix:  string(batchID),
		}, func(res storage.Result) (bool, error) {
			batchRadius := res.Entry.(*BatchRadiusItem)
			if batchRadius.Bin >= bin {
				return true, nil
			}
			evicted = append(evicted, batchRadius)
			if len(evicted) == count {
				return true, nil
			}
			return false, nil
		})
	})
	if err != nil {
		return 0, err
	}

	batchCnt := 1_000
	evictionCompleted := 0
	defer func() {
		r.size.Add(-int64(evictionCompleted))
	}()

	for i := 0; i < len(evicted); i += batchCnt {
		end := i + batchCnt
		if end > len(evicted) {
			end = len(evicted)
		}

		moveToCache := make([]swarm.Address, 0, end-i)

		err := txExecutor.Execute(ctx, func(store internal.Storage) error {
			batch, err := store.IndexStore().Batch(ctx)
			if err != nil {
				return err
			}

			for _, item := range evicted[i:end] {
				err = removeChunkWithItem(ctx, store, batch, item)
				if err != nil {
					return err
				}
				moveToCache = append(moveToCache, item.Address)
			}
			if err := batch.Commit(); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return evictionCompleted, err
		}
		evictionCompleted += end - i
	}

	return evictionCompleted, nil
}

func (r *Reserve) removeChunk(
	ctx context.Context,
	store internal.Storage,
	batch storage.Writer,
	chunkAddress swarm.Address,
	batchID []byte,
) error {
	item := &BatchRadiusItem{
		Bin:     swarm.Proximity(r.baseAddr.Bytes(), chunkAddress.Bytes()),
		BatchID: batchID,
		Address: chunkAddress,
	}
	err := store.IndexStore().Get(item)
	if err != nil {
		return err
	}
	return removeChunkWithItem(ctx, store, batch, item)
}

func removeChunkWithItem(
	ctx context.Context,
	store internal.Storage,
	batch storage.Writer,
	item *BatchRadiusItem,
) error {

	indexStore := store.IndexStore()

	var errs error

	stamp, _ := chunkstamp.LoadWithBatchID(indexStore, reserveNamespace, item.Address, item.BatchID)
	if stamp != nil {
		errs = errors.Join(
			stampindex.Delete(
				batch,
				reserveNamespace,
				swarm.NewChunk(item.Address, nil).WithStamp(stamp),
			),
			chunkstamp.DeleteWithStamp(batch, reserveNamespace, item.Address, stamp),
		)
	}

	return errors.Join(errs,
		batch.Delete(item),
		batch.Delete(&ChunkBinItem{Bin: item.Bin, BinID: item.BinID}),
		store.ChunkStore().Delete(ctx, item.Address),
	)
}

func (r *Reserve) lock(addr swarm.Address, batchID []byte) func() {
	r.mutx.Lock(addr.ByteString())
	r.mutx.Lock(string(batchID))
	return func() {
		r.mutx.Unlock(addr.ByteString())
		r.mutx.Unlock(string(batchID))
	}
}

func (r *Reserve) Radius() uint8 {
	return uint8(r.radius.Load())
}

func (r *Reserve) SetRadius(store storage.Store, rad uint8) error {
	r.radius.Store(uint32(rad))
	r.radiusSetter.SetStorageRadius(rad)
	return store.Put(&radiusItem{Radius: rad})
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

func (r *Reserve) LastBinIDs(store storage.Store) ([]uint64, uint64, error) {
	r.binMtx.Lock()
	defer r.binMtx.Unlock()

	var epoch EpochItem
	err := store.Get(&epoch)
	if err != nil {
		return nil, 0, err
	}

	ids := make([]uint64, swarm.MaxBins)

	for bin := uint8(0); bin < swarm.MaxBins; bin++ {
		binItem := &BinItem{Bin: bin}
		err := store.Get(binItem)
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

// should be called under lock
func (r *Reserve) IncBinID(store storage.Store, bin uint8) (uint64, error) {
	r.binMtx.Lock()
	defer r.binMtx.Unlock()

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
