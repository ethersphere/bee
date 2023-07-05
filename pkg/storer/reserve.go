// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	reserveOverCapacity  = "reserveOverCapacity"
	reserveUnreserved    = "reserveUnreserved"
	reserveUpdateLockKey = "reserveUpdateLockKey"
	batchExpiry          = "batchExpiry"
	expiredBatchAccess   = "expiredBatchAccess"

	cleanupDur = time.Hour * 6
)

var errMaxRadius = errors.New("max radius reached")

type Syncer interface {
	// Number of active historical syncing jobs.
	SyncRate() float64
	Start(context.Context)
}

func threshold(capacity int) int { return capacity * 5 / 10 }

func (db *DB) startReserveWorkers(
	ctx context.Context,
	warmupDur, wakeUpDur time.Duration,
	radius func() (uint8, error),
) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-db.quit
		cancel()
	}()

	// start eviction worker first as there could be batch expirations because of
	// initial contract sync
	db.reserveWg.Add(1)
	go db.evictionWorker(ctx)

	select {
	case <-time.After(warmupDur):
	case <-db.quit:
		return
	}

	initialRadius := db.reserve.Radius()

	// possibly a fresh node, acquire initial radius externally
	if initialRadius == 0 {
		r, err := radius()
		if err != nil {
			db.logger.Error(err, "reserve worker initial radius")
		} else {
			initialRadius = r
		}
	}

	if err := db.reserve.SetRadius(db.repo.IndexStore(), initialRadius); err != nil {
		db.logger.Error(err, "reserve set radius")
	}

	// syncing can now begin now that the reserver worker is running
	db.syncer.Start(ctx)

	db.reserveWg.Add(1)
	go db.radiusWorker(ctx, wakeUpDur)
}

func (db *DB) radiusWorker(ctx context.Context, wakeUpDur time.Duration) {
	defer db.reserveWg.Done()

	radiusWakeUpTicker := time.NewTicker(wakeUpDur)
	defer radiusWakeUpTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-radiusWakeUpTicker.C:
			radius := db.reserve.Radius()
			if db.reserve.Size() < threshold(db.reserve.Capacity()) && db.syncer.SyncRate() == 0 && radius > 0 {
				radius--
				err := db.reserve.SetRadius(db.repo.IndexStore(), radius)
				if err != nil {
					db.logger.Error(err, "reserve set radius")
				}
				db.logger.Info("reserve radius decrease", "radius", radius)
			}
			db.metrics.StorageRadius.Set(float64(radius))
		}
	}
}

func (db *DB) evictionWorker(ctx context.Context) {
	defer db.reserveWg.Done()

	batchExpiryTrigger, batchExpiryUnsub := db.events.Subscribe(batchExpiry)
	defer batchExpiryUnsub()

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

	cleanupExpired := func() {
		db.lock.Lock(expiredBatchAccess)
		batchesToEvict := make([][]byte, len(db.expiredBatches))
		copy(batchesToEvict, db.expiredBatches)
		db.expiredBatches = nil
		db.lock.Unlock(expiredBatchAccess)

		defer db.events.Trigger(reserveUnreserved)

		if len(batchesToEvict) > 0 {
			for _, batchID := range batchesToEvict {
				err := db.evictBatch(ctx, batchID, swarm.MaxBins)
				if err != nil {
					db.logger.Error(err, "evict batch")
				}
				db.metrics.ExpiredBatchCount.Inc()
			}
		}
	}

	cleanUpTicker := time.NewTicker(cleanupDur)
	defer cleanUpTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-overCapTrigger:
			// check if there are expired batches first
			cleanupExpired()

			err := db.unreserve(ctx)
			if err != nil {
				db.logger.Error(err, "reserve unreserve")
			}
			db.metrics.OverCapTriggerCount.Inc()
		case <-batchExpiryTrigger:
			cleanupExpired()
		case <-cleanUpTicker.C:
			cleanupExpired()

			if err := db.reserveCleanup(ctx); err != nil {
				db.logger.Error(err, "cleanup")
			}
		}
	}
}

func (db *DB) ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte) (chunk swarm.Chunk, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveGet").Observe(dur())
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveGet", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveGet", "failure").Inc()
		}
	}()

	return db.reserve.Get(ctx, db.repo, addr, batchID)
}

func (db *DB) StorageRadius() uint8 {
	if db.reserve == nil {
		return 0
	}
	return db.reserve.Radius()
}

func (db *DB) ReserveSize() int {
	if db.reserve == nil {
		return 0
	}
	return db.reserve.Size()
}

func (db *DB) IsWithinStorageRadius(addr swarm.Address) bool {
	if db.reserve == nil {
		return false
	}
	return swarm.Proximity(addr.Bytes(), db.baseAddr.Bytes()) >= db.reserve.Radius()
}

func (db *DB) ReserveHas(addr swarm.Address, batchID []byte) (has bool, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveHas").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHas", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHas", "failure").Inc()
		}
	}()

	return db.reserve.Has(db.repo.IndexStore(), addr, batchID)
}

// ReservePutter returns a Putter for inserting chunks into the reserve.
func (db *DB) ReservePutter() storage.Putter {
	return putterWithMetrics{
		storage.PutterFunc(
			func(ctx context.Context, chunk swarm.Chunk) (err error) {

				var (
					newIndex bool
				)
				db.lock.Lock(reserveUpdateLockKey)
				err = db.Execute(ctx, func(tx internal.Storage) error {
					newIndex, err = db.reserve.Put(ctx, tx, chunk)
					if err != nil {
						return fmt.Errorf("reserve: putter.Put: %w", err)
					}
					return nil
				})
				db.lock.Unlock(reserveUpdateLockKey)
				if err != nil {
					return err
				}
				if newIndex {
					db.reserve.AddSize(1)
				}
				db.reserveBinEvents.Trigger(string(db.po(chunk.Address())))
				if !db.reserve.IsWithinCapacity() {
					db.events.Trigger(reserveOverCapacity)
				}
				db.metrics.ReserveSize.Set(float64(db.reserve.Size()))
				return nil
			},
		),
		db.metrics,
		"reserve",
	}
}

// EvictBatch evicts all chunks belonging to a batch from the reserve.
func (db *DB) EvictBatch(ctx context.Context, batchID []byte) (err error) {
	if db.reserve == nil {
		// if reserve is not configured, do nothing
		return nil
	}

	db.lock.Lock(expiredBatchAccess)
	db.expiredBatches = append(db.expiredBatches, batchID)
	db.lock.Unlock(expiredBatchAccess)

	db.events.Trigger(batchExpiry)

	return nil
}

func (db *DB) evictBatch(ctx context.Context, batchID []byte, upToBin uint8) (retErr error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "EvictBatch").Observe(dur())
		if retErr == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "failure").Inc()
		}
	}()

	for b := uint8(0); b < upToBin; b++ {

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var evicted int

		var itemsToEvict []swarm.Address

		err := db.reserve.IterateBatchBin(ctx, db.repo, b, batchID, func(address swarm.Address) (bool, error) {
			itemsToEvict = append(itemsToEvict, address)
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("reserve: iterate batch bin: %w", err)
		}

		func() {
			// this is a performance optimization where we dont need to lock the
			// reserve updates if we are expiring batches. This is because the
			// reserve updates will not be related to these entries as the batch
			// is no longer valid.
			if upToBin != swarm.MaxBins {
				db.lock.Lock(reserveUpdateLockKey)
				defer db.lock.Unlock(reserveUpdateLockKey)
			}

			for _, item := range itemsToEvict {
				err = db.removeChunk(ctx, item, batchID)
				if err != nil {
					retErr = errors.Join(retErr, fmt.Errorf("reserve: remove chunk %v: %w", item, err))
					continue
				}
				evicted++
			}
		}()

		// if there was an error, we still need to update the chunks that have already
		// been evicted from the reserve
		db.logger.Debug("reserve eviction", "bin", b, "evicted", evicted, "batchID", hex.EncodeToString(batchID), "size", db.reserve.Size())
		db.reserve.AddSize(-evicted)
		db.metrics.ReserveSize.Set(float64(db.reserve.Size()))

		if upToBin == swarm.MaxBins {
			db.metrics.ExpiredChunkCount.Add(float64(evicted))
		} else {
			db.metrics.EvictedChunkCount.Add(float64(evicted))
		}
		if retErr != nil {
			return retErr
		}
	}

	return nil
}

func (db *DB) removeChunk(ctx context.Context, address swarm.Address, batchID []byte) error {
	return db.Execute(ctx, func(tx internal.Storage) error {
		chunk, err := db.ChunkStore().Get(ctx, address)
		if err == nil {
			err := db.Cache().Put(ctx, chunk)
			if err != nil {
				db.logger.Warning("reserve eviction cache put", "err", err)
			}
		}

		err = db.reserve.DeleteChunk(ctx, tx, address, batchID)
		if err != nil {
			return fmt.Errorf("reserve: delete chunk: %w", err)
		}
		return nil
	})
}

func (db *DB) reserveCleanup(ctx context.Context) error {
	dur := captureDuration(time.Now())
	removed := 0
	defer func() {
		db.reserve.AddSize(-removed)
		db.logger.Info("cleanup finished", "removed", removed, "duration", dur())
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "cleanup").Observe(dur())
		db.metrics.ReserveCleanup.Add(float64(removed))
		db.metrics.ReserveSize.Set(float64(db.reserve.Size()))
	}()

	var itemsToEvict []reserve.ChunkItem

	err := db.reserve.IterateChunksItems(db.repo, 0, func(ci reserve.ChunkItem) (bool, error) {
		if exists, err := db.batchstore.Exists(ci.BatchID); err == nil && !exists {
			itemsToEvict = append(itemsToEvict, ci)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	expiredBatches := make(map[string]struct{})
	var retErr error

	for _, item := range itemsToEvict {
		err = db.removeChunk(ctx, item.ChunkAddress, item.BatchID)
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("reserve: remove chunk %v: %w", item, err))
			continue
		}
		removed++
		if _, ok := expiredBatches[string(item.BatchID)]; !ok {
			expiredBatches[string(item.BatchID)] = struct{}{}
			db.logger.Debug("cleanup expired batch", "batch_id", hex.EncodeToString(item.BatchID))
		}
	}

	return retErr
}

func (db *DB) unreserve(ctx context.Context) (err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "unreserve").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "unreserve", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "unreserve", "failure").Inc()
		}
	}()

	radius := db.reserve.Radius()
	defer db.events.Trigger(reserveUnreserved)

	if db.reserve.IsWithinCapacity() {
		return nil
	}

	for radius < swarm.MaxBins {

		err := db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {

			if db.reserve.IsWithinCapacity() {
				return true, nil
			}

			return false, db.evictBatch(ctx, b.ID, radius)
		})
		if err != nil {
			return err
		}
		if db.reserve.IsWithinCapacity() {
			return nil
		}

		radius++
		db.logger.Info("reserve radius increase", "radius", radius)
		_ = db.reserve.SetRadius(db.repo.IndexStore(), radius)
	}

	return errMaxRadius
}

// ReserveLastBinIDs returns all of the highest binIDs from all the bins in the reserve.
func (db *DB) ReserveLastBinIDs() ([]uint64, error) {
	return db.reserve.LastBinIDs(db.repo.IndexStore())
}

func (db *DB) ReserveIterateChunks(cb func(swarm.Chunk) (bool, error)) error {
	return db.reserve.IterateChunks(db.repo, 0, cb)
}

// BinC is the result returned from the SubscribeBin channel that contains the chunk address and the binID
type BinC struct {
	Address swarm.Address
	BinID   uint64
	BatchID []byte
}

// SubscribeBin returns a channel that feeds all the chunks in the reserve from a certain bin between a start and end binIDs.
func (db *DB) SubscribeBin(ctx context.Context, bin uint8, start uint64) (<-chan *BinC, func(), <-chan error) {
	out := make(chan *BinC)
	done := make(chan struct{})
	errC := make(chan error, 1)

	db.reserveWg.Add(1)
	go func() {
		defer db.reserveWg.Done()

		trigger, unsub := db.reserveBinEvents.Subscribe(string(bin))
		defer unsub()
		defer close(out)

		for {

			err := db.reserve.IterateBin(db.repo.IndexStore(), bin, start, func(a swarm.Address, binID uint64, batchID []byte) (bool, error) {

				select {
				case out <- &BinC{Address: a, BinID: binID, BatchID: batchID}:
					start = binID + 1
				case <-done:
					return true, nil
				case <-db.quit:
					return false, ErrDBQuit
				case <-ctx.Done():
					return false, ctx.Err()
				}

				return false, nil
			})
			if err != nil {
				errC <- err
				return
			}

			select {
			case <-trigger:
			case <-done:
				return
			case <-db.quit:
				errC <- ErrDBQuit
				return
			case <-ctx.Done():
				errC <- err
				return
			}
		}
	}()

	var doneOnce sync.Once
	return out, func() {
		doneOnce.Do(func() { close(done) })
	}, errC
}

func (db *DB) po(addr swarm.Address) uint8 {
	return swarm.Proximity(db.baseAddr.Bytes(), addr.Bytes())
}
