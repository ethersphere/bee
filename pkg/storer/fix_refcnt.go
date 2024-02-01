// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
//	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ethersphere/bee/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
)

// FixRefCnt attempts to correct the RefCnt in all RetrievalIndexItems by scanning the actual chunk referers.
func FixRefCnt(ctx context.Context, basePath string, opts *Options, repair bool) error {
	logger := opts.Logger

	store, err := initStore(basePath, opts)
	if err != nil {
		return fmt.Errorf("failed creating levelDB index store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error(err, "failed closing store")
		}
	}()
	
	statusInterval := 15 * time.Second
	ticked := false
	ticker := time.NewTicker(statusInterval)
	go func() {
		for {
			select {
			case _ = <-ticker.C:
				ticked = true
			}
		}
	}()

	startChunkCount := time.Now()
	total := 0
	_ = chunkstore.Iterate(store, func(item *chunkstore.RetrievalIndexItem) error {
		total++
		if ticked {
			ticked = false
			logger.Info("..still counting chunks", "total", total, "elapsed", time.Since(startChunkCount).Round(time.Second))
		}
		return nil
	})
	ticker.Stop()
	logger.Info("chunk count finished", "total", total, "elapsed", time.Since(startChunkCount).Round(time.Second))
	
	pinTotal := -1
	if false {
		ticked = false
		ticker.Reset(statusInterval)
		startPinCount := time.Now()
		_ = pinstore.IteratePinnedChunks(store, func(addr swarm.Address) (bool, error) {
				pinTotal++
				if ticked {
					ticked = false
					logger.Info("..still counting pins", "total", pinTotal, "elapsed", time.Since(startPinCount).Round(time.Second))
				}
				return false, nil
			})
		logger.Info("pin count finished", "total", pinTotal, "elapsed", time.Since(startPinCount).Round(time.Second))
	}

//type RetrievalIndexItem struct {
//	Address   swarm.Address
//	Timestamp uint64
//	Location  sharky.Location
//	RefCnt    uint32
//}
	type ItemInfo struct {
		item *chunkstore.RetrievalIndexItem
		pinCnt uint32
		reserveCnt uint32
		cacheCnt uint32
		uploadCnt uint32
		batches [][]byte
	}
	
	start := time.Now()
	repaired := 0
	processed := 0
	discovered := 0
	shouldBeZero := 0
	chunksPerPass := 50_000_000
	for {
		n := time.Now()
		skip := processed
		ticked = false
		ticker.Reset(statusInterval)
		logger.Info("searching chunks", "skip", skip)

		items := make(map[string]*ItemInfo)
		// we deliberately choose to iterate the whole store again for each shard
		// so that we do not store all the items in memory (for operators with huge localstores)
		_ = chunkstore.Iterate(store, func(item *chunkstore.RetrievalIndexItem) error {
			if ticked {
				ticked = false
				logger.Info("..still searching chunks", "skip", skip, "found", len(items), "total", total, "elapsed", time.Since(n).Round(time.Second))
			}
			if skip > 0 {
				skip--
				return nil
			}
			items[item.Address.String()] = &ItemInfo{item:item}
			if len(items) >= chunksPerPass {
				return errors.New("found enough chunks")
			}
			return nil
		})
		ticker.Stop()

		logger.Info("found chunks", "found", len(items), "processed", processed, "total", total, "elapsed", time.Since(n).Round(time.Second))

		logger.Info("Scanning Cache")
		cacheCnt := 0
		cacheSeen := 0
		startCache := time.Now()
		ticked = false
		ticker.Reset(statusInterval)
		_ = cache.IterateCachedChunks(store, func(address swarm.Address) error {
			cacheSeen++
			_, exists := items[address.String()]
			if exists {
				items[address.String()].cacheCnt++
				cacheCnt++
			}
			if ticked {
				ticked = false
				logger.Info("..still scanning cache", "scanned", cacheSeen, "cached", cacheCnt, "elapsed", time.Since(startCache).Round(time.Second))
			}
			return nil
		})
		ticker.Stop()
		logger.Debug("scanned cache", "cached", cacheCnt, "total", cacheSeen, "elapsed", time.Since(startCache).Round(time.Second))
		
//type BatchRadiusItem struct {
//	Bin     uint8
//	BatchID []byte
//	Address swarm.Address
//	BinID   uint64
//}

		logger.Info("Scanning Reserve")
		reserveCnt := 0
		var reserveMax uint32
		reserveSeen := 0
		startReserve := time.Now()
		ticked = false
		ticker.Reset(statusInterval)
		_ = reserve.IterateReserve(store, func(bri *reserve.BatchRadiusItem) (bool, error) {
			reserveSeen++
			_, exists := items[bri.Address.String()]
			if exists {
				items[bri.Address.String()].reserveCnt++
				if items[bri.Address.String()].reserveCnt > reserveMax {
					reserveMax = items[bri.Address.String()].reserveCnt
				}
				items[bri.Address.String()].batches = append(items[bri.Address.String()].batches, bri.BatchID)
				reserveCnt++
			}
			if ticked {
				ticked = false
				logger.Info("..still scanning reserve", "scanned", reserveSeen, "reserved", reserveCnt, "maxDupes", reserveMax, "elapsed", time.Since(startReserve).Round(time.Second))
			}
			return false, nil
		})
		ticker.Stop()
		logger.Info("scanned reserve", "reserved", reserveCnt, "total", reserveSeen, "elapsed", time.Since(startReserve).Round(time.Second))

		logger.Info("Scanning Uploads")
		uploadCnt := 0
		uploadSeen := 0
		startUpload := time.Now()
		ticked = false
		ticker.Reset(statusInterval)
		_ = upload.IterateAllPushItems(store, func(address swarm.Address) (bool, error) {
			uploadSeen++
			_, exists := items[address.String()]
			if exists {
				items[address.String()].uploadCnt++
				uploadCnt++
			}
			if ticked {
				ticked = false
				logger.Info("..still scanning uploads", "scanned", uploadSeen, "uploads", uploadCnt, "elapsed", time.Since(startUpload).Round(time.Second))
			}
			return false, nil
		})
		ticker.Stop()
		logger.Debug("scanned uploads", "uploads", uploadCnt, "total", uploadSeen, "elapsed", time.Since(startUpload).Round(time.Second))

		logger.Info("Scanning Pins")
		pinScanCount := 0
		pinCnt := 0
		var pinMax uint32
		startPins := time.Now()
		ticked = false
		ticker.Reset(statusInterval)
		_ = pinstore.IteratePinnedChunks(store, func(addr swarm.Address) (bool, error) {
				pinScanCount++
				_, exists := items[addr.String()]
				if exists {
					items[addr.String()].pinCnt++
					if items[addr.String()].pinCnt > pinMax {
						pinMax = items[addr.String()].pinCnt
					}
					pinCnt++
				} else if len(items) == total {	// Don't complain about missing pins if we can't handle all chunks at once
					logger.Debug("missing pinned chunk", "address", addr)
				}
				if ticked {
					ticked = false
					logger.Info("..still scanning pins", "scanned", pinScanCount, "pinned", pinCnt, "total", pinTotal, "maxDupes", pinMax, "elapsed", time.Since(startPins).Round(time.Second))
				}
				return false, nil
			})
		ticker.Stop()
		if pinTotal == -1 {
			pinTotal = pinScanCount
		}
		logger.Info("scanned pins", "pinned", pinCnt, "total", pinTotal, "elapsed", time.Since(startPins).Round(time.Second))

		batch, err := store.Batch(ctx)
		if err != nil {
			logger.Error(err, "Failed to create batch")
		}
		batchHits := 0

		logger.Info("Summarizing refCnts")
		refs := make(map[uint32]uint32)
		for _, item := range items {
			refs[item.item.RefCnt]++
			newRefCnt := item.pinCnt*100 + item.reserveCnt + item.cacheCnt + item.uploadCnt
			if newRefCnt != item.item.RefCnt {
				discovered++
				if item.item.RefCnt == 1 && newRefCnt == 0 {
					shouldBeZero++
				} else {
					logger.Warning("Mismatched RefCnt", "original", item.item.RefCnt, "new", newRefCnt, "chunk", item.item.Address, "pins", item.pinCnt, "reserve", item.reserveCnt, "cache", item.cacheCnt, "upload", item.uploadCnt, "batchHits", batchHits)
					if repair {
						orgRefCnt := item.item.RefCnt
						item.item.RefCnt = max(1,newRefCnt)
						if err := batch.Put(item.item); err != nil {
							logger.Error(err,"Failed to update RefCnt", "chunk", item.item.Address, "from", orgRefCnt, "to", newRefCnt)
						}
						batchHits = batchHits + 1
						if batchHits >= 20000 {
							if err := batch.Commit(); err != nil {
								logger.Error(err, "batch.Commit failed")
							}
							batch, err = store.Batch(ctx)
							if err != nil {
								logger.Error(err, "Failed to create batch")
							}
							batchHits = 0
						}
						repaired++
					} else if newRefCnt == 0 {
						shouldBeZero++
					}
				}
//			} else if item.item.RefCnt > 9 {
//				_, exists := items[item.item.Address.String()]
//				logger.Debug("Matched RefCnt", "original", item.item.RefCnt, "new", newRefCnt, "chunk", item.item.Address, "pins", item.pinCnt, "reserve", item.reserveCnt, "cache", item.cacheCnt, "upload", item.uploadCnt, "exists", exists)
			}
//			if item.reserveCnt > 1 {
//				logger.Debug("multi-reserve", "chunk", item.item.Address, "count", item.reserveCnt)
//				for _, batch := range item.batches {
//					logger.Debug("multi-reserve", "chunk", item.item.Address, "batch", hex.EncodeToString(batch))
//				}
//			}
		}
		if err := batch.Commit(); err != nil {
			logger.Error(err, "batch.Commit failed")
		}

		keys := make([]int, 0, len(refs))
		for k := range refs {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)

		for _, k := range keys {
			logger.Debug("refCnt distribution", "refCnt", k, "count", refs[uint32(k)])
		}

		processed += len(items)
		logger.Info("scanned chunks", "discovered", discovered, "repaired", repaired, "shouldBeZero", shouldBeZero, "processed", processed, "total", total, "elapsed", time.Since(start).Round(time.Second))

		if len(items) < chunksPerPass {	// Incomplete means that we hit the end!
			break
		}
	}

	return nil
}
