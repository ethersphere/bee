// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	"github.com/ethersphere/bee/pkg/log"
	storagev2 "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "reserve"

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

func New(baseAddr swarm.Address, store storagev2.Store, capacity int, reserveRadius uint8, radiusSetter topology.SetStorageRadiuser, logger log.Logger) (*Reserve, error) {

	rs := &Reserve{
		baseAddr:     baseAddr,
		capacity:     capacity,
		radiusSetter: radiusSetter,
		logger:       logger.WithName(loggerName).Register(),
	}

	rItem := &radiusItem{}
	err := store.Get(rItem)
	if err != nil {
		if errors.Is(err, storagev2.ErrNotFound) { // fresh node
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

func (r *Reserve) Putter(store internal.Storage) storagev2.Putter {

	return storagev2.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {

		r.mtx.Lock()
		defer r.mtx.Unlock()

		indexStore := store.IndexStore()
		chunkStore := store.ChunkStore()

		po := swarm.Proximity(r.baseAddr.Bytes(), chunk.Address().Bytes())

		has, err := indexStore.Has(&batchRadiusItem{
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

		binID, err := incBinID(indexStore, po)
		if err != nil {
			return err
		}

		// fmt.Println("inserting", "bin", po, "addr", chunk.Address(), "BinID", binID, "batchID", hex.EncodeToString(chunk.Stamp().BatchID()))

		err = indexStore.Put(&batchRadiusItem{
			Bin:     po,
			Address: chunk.Address(),
			BatchID: chunk.Stamp().BatchID(),
			BinID:   binID,
		})
		if err != nil {
			return err
		}

		err = indexStore.Put(&chunkBinItem{
			Bin:     po,
			BinID:   binID,
			Address: chunk.Address(),
		})
		if err != nil {
			return err
		}

		return chunkStore.Put(ctx, chunk)
	})
}

func (r *Reserve) IterateBin(store storagev2.Store, bin uint8, startBinID uint64, cb func(swarm.Address, uint64) (bool, error)) error {
	err := store.Iterate(storagev2.Query{
		Factory:       func() storagev2.Item { return &chunkBinItem{} },
		Prefix:        binIDToString(bin, startBinID),
		PrefixAtStart: true,
	}, func(res storagev2.Result) (bool, error) {
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

func (r *Reserve) Iterate(store storagev2.Store, bin uint8, cb func(swarm.Address, uint64) (bool, error)) error {
	err := store.Iterate(storagev2.Query{
		Factory:       func() storagev2.Item { return &chunkBinItem{} },
		Prefix:        binIDToString(bin, 0),
		PrefixAtStart: true,
	}, func(res storagev2.Result) (bool, error) {
		item := res.Entry.(*chunkBinItem)
		stop, err := cb(item.Address, item.BinID)
		if stop || err != nil {
			return true, err
		}
		return false, nil
	})

	return err
}

func (r *Reserve) EvictBatchBin(store internal.Storage, batchID []byte, bin uint8) (int, error) {

	r.mtx.Lock()
	defer r.mtx.Unlock()

	indexStore := store.IndexStore()
	chunkStore := store.ChunkStore()

	evicted := 0

	for i := uint8(0); i < bin; i++ {
		err := indexStore.Iterate(storagev2.Query{
			Factory: func() storagev2.Item {
				return &batchRadiusItem{}
			},
			Prefix: batchBinToString(i, batchID),
		}, func(res storagev2.Result) (bool, error) {

			batchRadius := res.Entry.(*batchRadiusItem)

			err := indexStore.Delete(batchRadius)
			if err != nil {
				return false, err
			}

			err = indexStore.Delete(&chunkBinItem{
				Bin:   batchRadius.Bin,
				BinID: batchRadius.BinID,
			})
			if err != nil {
				return false, err
			}

			err = chunkStore.Delete(context.TODO(), batchRadius.Address)
			if err != nil {
				return false, err
			}

			evicted++

			return false, nil
		})
		if err != nil {
			return 0, err
		}
	}

	return evicted, nil
}

func (r *Reserve) LastBinIDs(store storagev2.Store) ([]uint64, error) {

	r.mtx.Lock()
	defer r.mtx.Unlock()

	ids := make([]uint64, swarm.MaxBins)

	for bin := uint8(0); bin < swarm.MaxBins; bin++ {
		binItem := &binItem{Bin: bin}
		err := store.Get(binItem)
		if err != nil {
			if errors.Is(err, storagev2.ErrNotFound) {
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

// Must be called underlock.
func (r *Reserve) SetRadius(store storagev2.Store, rad uint8) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.radius = rad
	r.radiusSetter.SetStorageRadius(r.radius)
	return store.Put(&radiusItem{Radius: rad})
}

// Must be called under lock.
func incBinID(store storagev2.Store, po uint8) (uint64, error) {

	bin := &binItem{Bin: po}
	err := store.Get(bin)
	if err != nil {
		if errors.Is(err, storagev2.ErrNotFound) {
			return 0, store.Put(bin)
		}

		return 0, err
	}

	bin.BinID += 1

	return bin.BinID, store.Put(bin)
}
