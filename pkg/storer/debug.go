// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"

	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

type UploadStat struct {
	TotalUploaded int
	TotalSynced   int
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
	Size     int
	Capacity int
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

	totalChunks, sharedSlots := 0, 0
	eg.Go(func() error {
		return chunkstore.IterateChunkEntries(
			db.repo.IndexStore(),
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

	uploaded, synced := 0, 0
	eg.Go(func() error {
		return upload.IterateAll(
			db.repo.IndexStore(),
			func(_ swarm.Address, isSynced bool) (bool, error) {
				select {
				case <-ctx.Done():
					return true, ctx.Err()
				case <-db.quit:
					return true, ErrDBQuit
				default:
				}

				uploaded++
				if isSynced {
					synced++
				}
				return false, nil
			},
		)
	})

	collections, chunkCount := 0, 0
	eg.Go(func() error {
		return pinstore.IterateCollectionStats(
			db.repo.IndexStore(),
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

	cacheSize, cacheCapacity := db.cacheObj.Size(), db.cacheObj.Capacity()

	reserveSize, reserveCapacity := 0, 0
	if db.reserve != nil {
		reserveSize, reserveCapacity = db.reserve.Size(), db.reserve.Capacity()
	}

	if err := eg.Wait(); err != nil {
		return Info{}, err
	}

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
			Size:     reserveSize,
			Capacity: reserveCapacity,
		},
		ChunkStore: ChunkStoreStat{
			TotalChunks: totalChunks,
			SharedSlots: sharedSlots,
		},
	}, nil
}
