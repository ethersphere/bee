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
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/storer/internal/stampindex"
	"github.com/ethersphere/bee/pkg/storer/internal/transaction"
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
		logger:       logger.WithName(reserveNamespace).Register(),
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

// Put stores a new chunk in the reserve and returns if the reserve size should increase.
func (r *Reserve) Put(ctx context.Context, chunk swarm.Chunk) error {

	bin := swarm.Proximity(r.baseAddr.Bytes(), chunk.Address().Bytes())

	// batchID lock, Put vs Eviction
	r.multx.Lock(string(chunk.Stamp().BatchID()))
	defer r.multx.Unlock(string(chunk.Stamp().BatchID()))

	// bin lock
	r.multx.Lock(strconv.Itoa(int(bin)))
	defer r.multx.Unlock(strconv.Itoa(int(bin)))

	trx, done := r.st.NewTransaction(ctx)
	defer done()

	indexStore := trx.IndexStore()
	chunkStore := trx.ChunkStore()

	has, err := indexStore.Has(&BatchRadiusItem{
		Bin:     bin,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	})
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	item, loadedStamp, err := stampindex.LoadOrStore(indexStore, reserveNamespace, chunk)
	if err != nil {
		return fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
	}
	if loadedStamp {
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

		err := r.removeChunk(ctx, trx, item.ChunkAddress, chunk.Stamp().BatchID())
		if err != nil {
			return fmt.Errorf("failed removing older chunk: %w", err)
		}

		r.logger.Debug(
			"replacing chunk stamp index",
			"old_chunk", item.ChunkAddress,
			"new_chunk", chunk.Address(),
			"batch_id", hex.EncodeToString(chunk.Stamp().BatchID()),
		)

		err = stampindex.Store(indexStore, reserveNamespace, chunk)
		if err != nil {
			return fmt.Errorf("failed updating stamp index: %w", err)
		}
	}

	err = chunkstamp.Store(indexStore, reserveNamespace, chunk)
	if err != nil {
		return err
	}

	binID, err := r.IncBinID(indexStore, bin)
	if err != nil {
		return err
	}

	err = indexStore.Put(&BatchRadiusItem{
		Bin:     bin,
		BinID:   binID,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	})
	if err != nil {
		return err
	}

	err = indexStore.Put(&ChunkBinItem{
		Bin:     bin,
		BinID:   binID,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
		Type:    ChunkType(chunk),
	})
	if err != nil {
		return err
	}

	err = chunkStore.Put(ctx, chunk)
	if err != nil {
		return err
	}

	err = trx.Commit()
	if err != nil {
		return err
	}

	if !loadedStamp {
		r.size.Add(1)
	}

	return nil
}

func (r *Reserve) Has(addr swarm.Address, batchID []byte) (bool, error) {
	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr}
	return r.st.IndexStore().Has(item)
}

func (r *Reserve) Get(ctx context.Context, addr swarm.Address, batchID []byte) (swarm.Chunk, error) {
	r.multx.Lock(string(batchID))
	defer r.multx.Unlock(string(batchID))

	item := &BatchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr}
	err := r.st.IndexStore().Get(item)
	if err != nil {
		return nil, err
	}

	stamp, err := chunkstamp.LoadWithBatchID(r.st.IndexStore(), reserveNamespace, addr, item.BatchID)
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

	var evicted []*BatchRadiusItem

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
		evicted = append(evicted, batchRadius)
		count--
		if count == 0 {
			return true, nil
		}
		return false, nil
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

		err := r.st.Run(ctx, func(s transaction.Store) error {
			for _, item := range evicted[i:end] {
				err = r.removeChunkWithItem(ctx, s, item)
				if err != nil {
					return err
				}
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
	trx transaction.Store,
	chunkAddress swarm.Address,
	batchID []byte,
) error {
	item := &BatchRadiusItem{
		Bin:     swarm.Proximity(r.baseAddr.Bytes(), chunkAddress.Bytes()),
		BatchID: batchID,
		Address: chunkAddress,
	}
	err := trx.IndexStore().Get(item)
	if err != nil {
		return err
	}
	return r.removeChunkWithItem(ctx, trx, item)
}

func (r *Reserve) removeChunkWithItem(
	ctx context.Context,
	trx transaction.Store,
	item *BatchRadiusItem,
) error {

	indexStore := trx.IndexStore()

	var errs error

	stamp, _ := chunkstamp.LoadWithBatchID(indexStore, reserveNamespace, item.Address, item.BatchID)
	if stamp != nil {
		errs = errors.Join(
			stampindex.Delete(
				trx.IndexStore(),
				reserveNamespace,
				swarm.NewChunk(item.Address, nil).WithStamp(stamp),
			),
			chunkstamp.DeleteWithStamp(trx.IndexStore(), reserveNamespace, item.Address, stamp),
		)
	}

	return errors.Join(errs,
		trx.IndexStore().Delete(item),
		trx.IndexStore().Delete(&ChunkBinItem{Bin: item.Bin, BinID: item.BinID}),
		trx.ChunkStore().Delete(ctx, item.Address),
	)
}

func (r *Reserve) IterateBin(bin uint8, startBinID uint64, cb func(swarm.Address, uint64, []byte) (bool, error)) error {
	err := r.st.IndexStore().Iterate(storage.Query{
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

		stamp, err := chunkstamp.LoadWithBatchID(r.st.IndexStore(), reserveNamespace, item.Address, item.BatchID)
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
