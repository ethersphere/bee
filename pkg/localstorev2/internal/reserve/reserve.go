// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstamp"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/stampindex"
	"github.com/ethersphere/bee/pkg/log"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "reserve"

const reserveNamespace = "reserve"

var (
	// errOverwriteOfImmutableBatch is returned when stamp index already
	// exists and the batch is immutable.
	errOverwriteOfImmutableBatch = errors.New("reserve: overwrite of existing immutable batch")

	// errOverwriteOfNewerBatch is returned if a stamp index already exists
	// and the existing chunk with the same stamp index has a newer timestamp.
	errOverwriteOfNewerBatch = errors.New("reserve: overwrite of existing batch with newer timestamp")
)

type Reserve struct {
	mtx sync.Mutex

	baseAddr     swarm.Address
	radiusSetter topology.SetStorageRadiuser
	logger       log.Logger

	capacity int
	size     int
	radius   uint8
}

/*
	pull by 	bin - binID
	evict by 	bin - batchID
	sample by 	bin
*/

func New(
	baseAddr swarm.Address,
	store storage.Store,
	capacity int,
	reserveRadius uint8,
	radiusSetter topology.SetStorageRadiuser,
	logger log.Logger) (*Reserve, error) {

	rs := &Reserve{
		baseAddr:     baseAddr,
		capacity:     capacity,
		radiusSetter: radiusSetter,
		logger:       logger.WithName(loggerName).Register(),
	}

	rItem := &radiusItem{}
	err := store.Get(rItem)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) { // fresh node
			rItem.Radius = reserveRadius
		} else {
			return nil, err
		}
	}
	err = rs.SetRadius(store, rItem.Radius)
	if err != nil {
		return nil, err
	}

	size, err := store.Count(&batchRadiusItem{})
	if err != nil {
		return nil, err
	}

	rs.size = size

	return rs, nil
}

// Put stores a new chunk in the reserve and returns if the reserve size should increase.
func (r *Reserve) Put(ctx context.Context, store internal.Storage, chunk swarm.Chunk) (bool, error) {

	indexStore := store.IndexStore()
	chunkStore := store.ChunkStore()

	po := swarm.Proximity(r.baseAddr.Bytes(), chunk.Address().Bytes())

	has, err := indexStore.Has(&batchRadiusItem{
		Bin:     po,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	})
	if err != nil {
		return false, err
	}
	if has {
		return false, nil
	}

	newStampIndex := true

	switch item, loaded, err := stampindex.LoadOrStore(indexStore, reserveNamespace, chunk); {
	case err != nil:
		return false, fmt.Errorf("load or store stamp index for chunk %v has fail: %w", chunk, err)
	case loaded && item.ChunkIsImmutable:
		return false, errOverwriteOfImmutableBatch
	case loaded && !item.ChunkIsImmutable:
		prev := binary.BigEndian.Uint64(item.StampTimestamp)
		curr := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
		if prev >= curr {
			return false, errOverwriteOfNewerBatch
		}
		// An older and different chunk with the same batchID and stamp index has been previously
		// saved to the reserve. We must do the below before saving the new chunk:
		// 1. Delete the old chunk from the chunkstore
		// 2. Delete the old chunk's stamp data
		// 3. Update the stamp index
		newStampIndex = false
		err := chunkStore.Delete(ctx, item.ChunkAddress)
		if err != nil {
			return false, fmt.Errorf("failed deleting chunk with older timestamp from chunkstore: %w", err)
		}
		err = chunkstamp.Delete(indexStore, reserveNamespace, item.ChunkAddress, chunk.Stamp().BatchID())
		if err != nil {
			return false, fmt.Errorf("failed deleting the stamp of the older chunk: %w", err)
		}
		err = stampindex.Store(indexStore, reserveNamespace, chunk)
		if err != nil {
			return false, fmt.Errorf("failed updating stamp index: %w", err)
		}
	}

	err = chunkstamp.Store(indexStore, reserveNamespace, chunk)
	if err != nil {
		return false, err
	}

	binID, err := r.incBinID(indexStore, po)
	if err != nil {
		return false, err
	}

	err = indexStore.Put(&batchRadiusItem{
		Bin:     po,
		BinID:   binID,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	})
	if err != nil {
		return false, err
	}

	err = indexStore.Put(&chunkBinItem{
		Bin:     po,
		BinID:   binID,
		Address: chunk.Address(),
		BatchID: chunk.Stamp().BatchID(),
	})
	if err != nil {
		return false, err
	}

	err = chunkStore.Put(ctx, chunk)
	if err != nil {
		return false, err
	}

	return newStampIndex, nil
}

func (r *Reserve) Has(store storage.Store, addr swarm.Address, batchID []byte) (bool, error) {
	item := &batchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr}
	err := store.Get(item)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (r *Reserve) Get(ctx context.Context, storage internal.Storage, addr swarm.Address, batchID []byte) (swarm.Chunk, error) {

	item := &batchRadiusItem{Bin: swarm.Proximity(r.baseAddr.Bytes(), addr.Bytes()), BatchID: batchID, Address: addr}
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

func (r *Reserve) IterateBin(store storage.Store, bin uint8, startBinID uint64, cb func(swarm.Address, uint64) (bool, error)) error {
	err := store.Iterate(storage.Query{
		Factory:       func() storage.Item { return &chunkBinItem{} },
		Prefix:        binIDToString(bin, startBinID),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*chunkBinItem)
		if item.Bin > bin {
			return true, nil
		}

		stop, err := cb(item.Address, item.BinID)
		if stop || err != nil {
			return true, err
		}

		return false, nil
	})

	return err
}

func (r *Reserve) IterateChunks(store internal.Storage, startBin uint8, cb func(swarm.Chunk, uint64) (bool, error)) error {
	err := store.IndexStore().Iterate(storage.Query{
		Factory:       func() storage.Item { return &chunkBinItem{} },
		Prefix:        binIDToString(startBin, 0),
		PrefixAtStart: true,
	}, func(res storage.Result) (bool, error) {
		item := res.Entry.(*chunkBinItem)

		chunk, err := store.ChunkStore().Get(context.Background(), item.Address)
		if err != nil {
			return false, err
		}

		stamp, err := chunkstamp.LoadWithBatchID(store.IndexStore(), reserveNamespace, item.Address, item.BatchID)
		if err != nil {
			return false, err
		}

		stop, err := cb(chunk.WithStamp(stamp), item.BinID)
		if stop || err != nil {
			return true, err
		}
		return false, nil
	})

	return err
}

func (r *Reserve) EvictBatchBin(store internal.Storage, batchID []byte, bin uint8) (int, error) {

	indexStore := store.IndexStore()
	chunkStore := store.ChunkStore()

	count := 0

	for i := uint8(0); i < bin; i++ {

		var evicted []*batchRadiusItem

		err := indexStore.Iterate(storage.Query{
			Factory: func() storage.Item {
				return &batchRadiusItem{}
			},
			Prefix: batchBinToString(i, batchID),
		}, func(res storage.Result) (bool, error) {
			batchRadius := res.Entry.(*batchRadiusItem)
			evicted = append(evicted, batchRadius)
			return false, nil
		})
		if err != nil {
			return 0, err
		}

		count += len(evicted)

		for _, item := range evicted {

			err := indexStore.Delete(&chunkBinItem{
				Bin:   item.Bin,
				BinID: item.BinID,
			})
			if err != nil {
				return 0, err
			}

			stamp, err := chunkstamp.LoadWithBatchID(indexStore, reserveNamespace, item.Address, item.BatchID)
			if err != nil {
				return 0, err
			}

			err = stampindex.Delete(indexStore, reserveNamespace, swarm.NewChunk(item.Address, nil).WithStamp(stamp))
			if err != nil {
				return 0, err
			}

			err = chunkstamp.Delete(indexStore, reserveNamespace, item.Address, item.BatchID)
			if err != nil {
				return 0, err
			}

			err = chunkStore.Delete(context.Background(), item.Address)
			if err != nil {
				return 0, err
			}

			err = indexStore.Delete(item)
			if err != nil {
				return 0, err
			}
		}
	}

	return count, nil
}

func (r *Reserve) LastBinIDs(store storage.Store) ([]uint64, error) {

	ids := make([]uint64, swarm.MaxBins)

	for bin := uint8(0); bin < swarm.MaxBins; bin++ {
		binItem := &binItem{Bin: bin}
		err := store.Get(binItem)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				ids[bin] = 0
			} else {
				return nil, err
			}
		} else {
			ids[bin] = binItem.BinID
		}
	}

	return ids, nil
}

func (r *Reserve) Radius() uint8 {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.radius
}

func (r *Reserve) Size() int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.size
}

func (r *Reserve) AddSize(diff int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.size += diff
}

func (r *Reserve) IsWithinCapacity() bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.size <= r.capacity
}

func (r *Reserve) SetRadius(store storage.Store, rad uint8) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.radius = rad
	r.radiusSetter.SetStorageRadius(rad)
	return store.Put(&radiusItem{Radius: rad})
}

func (r *Reserve) incBinID(store storage.Store, po uint8) (uint64, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	bin := &binItem{Bin: po}
	err := store.Get(bin)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, store.Put(bin)
		}

		return 0, err
	}

	bin.BinID += 1

	return bin.BinID, store.Put(bin)
}
