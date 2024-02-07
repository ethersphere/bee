// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"

	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

type UploadStat struct {
	TotalUploaded uint64
	TotalSynced   uint64
}

type PinningStat struct {
	TotalCollections int
	TotalChunks      int
}

type CacheStat struct {
	Size     int
	Capacity int
}

type ReserveStat struct {
	SizeWithinRadius int
	TotalSize        int
	Capacity         int
	LastBinIDs       []uint64
	Epoch            uint64
}

type ChunkStoreStat struct {
	TotalChunks int
	SharedSlots int
}

type Info struct {
	Upload     UploadStat
	Pinning    PinningStat
	Cache      CacheStat
	Reserve    ReserveStat
	ChunkStore ChunkStoreStat
}

func (db *DB) DebugInfo(ctx context.Context) (Info, error) {
	eg, ctx := errgroup.WithContext(ctx)

	var (
		totalChunks int
		sharedSlots int
	)
	eg.Go(func() error {
		return chunkstore.IterateChunkEntries(
			db.storage.ReadOnly().IndexStore(),
			func(_ swarm.Address, isShared bool) (bool, error) {
				select {
				case <-ctx.Done():
					return true, ctx.Err()
				case <-db.quit:
					return true, ErrDBQuit
				default:
				}

				totalChunks++
				if isShared {
					sharedSlots++
				}
				return false, nil
			},
		)
	})

	var (
		uploaded uint64
		synced   uint64
	)
	eg.Go(func() error {
		return upload.IterateAllTagItems(db.storage.ReadOnly().IndexStore(), func(ti *upload.TagItem) (bool, error) {
			select {
			case <-ctx.Done():
				return true, ctx.Err()
			case <-db.quit:
				return true, ErrDBQuit
			default:
			}
			uploaded += ti.Split
			synced += ti.Synced
			return false, nil
		})
	})

	var (
		collections int
		chunkCount  int
	)
	eg.Go(func() error {
		return pinstore.IterateCollectionStats(
			db.storage.ReadOnly().IndexStore(),
			func(stat pinstore.CollectionStat) (bool, error) {
				select {
				case <-ctx.Done():
					return true, ctx.Err()
				case <-db.quit:
					return true, ErrDBQuit
				default:
				}

				collections++
				chunkCount += int(stat.Total - stat.DupInCollection)
				return false, nil
			},
		)
	})

	var (
		reserveCapacity         int
		reserveSize             int
		reserveSizeWithinRadius int

		lastBinIDs []uint64
		epoch      uint64
	)
	if db.reserve != nil {
		reserveCapacity = db.reserve.Capacity()
		reserveSize = db.reserve.Size()
		eg.Go(func() error {
			return db.reserve.IterateChunksItems(db.reserve.Radius(), func(ci *reserve.ChunkBinItem) (bool, error) {
				reserveSizeWithinRadius++
				return false, nil
			})
		})

		var err error
		lastBinIDs, epoch, err = db.ReserveLastBinIDs()
		if err != nil {
			return Info{}, err
		}
	}

	if err := eg.Wait(); err != nil {
		return Info{}, err
	}

	cacheSize := db.cacheObj.Size()
	cacheCapacity := db.cacheObj.Capacity()

	return Info{
		Upload: UploadStat{
			TotalUploaded: uploaded,
			TotalSynced:   synced,
		},
		Pinning: PinningStat{
			TotalCollections: collections,
			TotalChunks:      chunkCount,
		},
		Cache: CacheStat{
			Size:     int(cacheSize),
			Capacity: int(cacheCapacity),
		},
		Reserve: ReserveStat{
			SizeWithinRadius: reserveSizeWithinRadius,
			TotalSize:        reserveSize,
			Capacity:         reserveCapacity,
			LastBinIDs:       lastBinIDs,
			Epoch:            epoch,
		},
		ChunkStore: ChunkStoreStat{
			TotalChunks: totalChunks,
			SharedSlots: sharedSlots,
		},
	}, nil
}
