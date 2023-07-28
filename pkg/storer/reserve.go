// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/semaphore"
)

const (
	reserveOverCapacity  = "reserveOverCapacity"
	reserveUnreserved    = "reserveUnreserved"
	reserveUpdateLockKey = "reserveUpdateLockKey"
	batchExpiry          = "batchExpiry"

	cleanupDur = time.Hour * 6
)

func reserveUpdateBatchLockKey(batchID []byte, bin uint8) string {
	return fmt.Sprintf("%s%s%d", reserveUpdateLockKey, string(batchID), bin)
}

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
	db.inFlight.Add(1)
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

	db.inFlight.Add(1)
	go db.radiusWorker(ctx, wakeUpDur)
}

func (db *DB) radiusWorker(ctx context.Context, wakeUpDur time.Duration) {
	defer db.inFlight.Done()

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

func (db *DB) getExpiredBatches() ([][]byte, error) {
	var batchesToEvict [][]byte
	err := db.repo.IndexStore().Iterate(storage.Query{
		Factory:      func() storage.Item { return new(expiredBatchItem) },
		ItemProperty: storage.QueryItemID,
	}, func(result storage.Result) (bool, error) {
		batchesToEvict = append(batchesToEvict, []byte(result.ID))
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return batchesToEvict, nil
}

func (db *DB) removeExpiredBatch(ctx context.Context, batchID []byte) error {
	evicted, err := db.evictBatch(ctx, batchID, swarm.MaxBins)
	if err != nil {
		return err
	}
	if evicted > 0 {
		db.logger.Debug("evicted expired batch", "batch_id", hex.EncodeToString(batchID), "total_evicted", evicted)
	}
	return db.Execute(ctx, func(tx internal.Storage) error {
		return tx.IndexStore().Delete(&expiredBatchItem{BatchID: batchID})
	})
}

func (db *DB) evictionWorker(ctx context.Context) {
	defer db.inFlight.Done()

	batchExpiryTrigger, batchExpiryUnsub := db.events.Subscribe(batchExpiry)
	defer batchExpiryUnsub()

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

	var (
		unreserveSem    = semaphore.NewWeighted(1)
		unreserveCtx    context.Context
		cancelUnreserve context.CancelFunc
		expirySem       = semaphore.NewWeighted(1)
		expiryWorkers   = semaphore.NewWeighted(4)
	)

	stopped := make(chan struct{})
	stopWorkers := func() {
		defer close(stopped)

		// wait for all workers to finish
		_ = expirySem.Acquire(context.Background(), 1)
		_ = unreserveSem.Acquire(context.Background(), 1)
		_ = expiryWorkers.Acquire(context.Background(), 4)
	}

	cleanupExpired := func() {
		db.metrics.ExpiryTriggersCount.Inc()
		if !expirySem.TryAcquire(1) {
			// if there is already a goroutine taking care of expirations dont wait
			// for it to finish. Cleanup is called from all cases, so we dont
			// need to trigger everytime.
			return
		}
		defer expirySem.Release(1)

		batchesToEvict, err := db.getExpiredBatches()
		if err != nil {
			db.logger.Error(err, "get expired batches")
			return
		}

		if len(batchesToEvict) == 0 {
			return
		}

		// we ensure unreserve is not running and if it is we cancel it and wait
		// for it to finish, this is to prevent unreserve and expirations running
		// at the same time. The expiration will free up space so the unreserve
		// target might change. This is to prevent unreserve from running with
		// the old target.
		if !unreserveSem.TryAcquire(1) {
			cancelUnreserve()
			err := unreserveSem.Acquire(ctx, 1)
			if err != nil {
				db.logger.Error(err, "acquire unreserve semaphore")
				return
			}
			// trigger it again at the end if required
			defer db.events.Trigger(reserveOverCapacity)
		}
		defer unreserveSem.Release(1)

		defer db.events.Trigger(reserveUnreserved)

		db.metrics.ExpiryRunsCount.Inc()

		for _, batchID := range batchesToEvict {
			b := batchID
			if err := expiryWorkers.Acquire(ctx, 1); err != nil {
				db.logger.Error(err, "acquire expiry worker semaphore")
				return
			}
			go func() {
				defer expiryWorkers.Release(1)

				err := db.removeExpiredBatch(ctx, b)
				if err != nil {
					db.logger.Error(err, "remove expired batch", "batch_id", hex.EncodeToString(b))
				} else {
					db.metrics.ExpiredBatchCount.Inc()
				}
			}()
		}

		if err := expiryWorkers.Acquire(ctx, 4); err != nil {
			db.logger.Error(err, "wait for expiry workers")
			return
		}
		expiryWorkers.Release(4)
	}

	// Initial cleanup.
	db.events.Trigger(batchExpiry)

	cleanUpTicker := time.NewTicker(cleanupDur)
	defer cleanUpTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			stopWorkers()
			<-stopped
			return
		case <-overCapTrigger:
			// check if there are expired batches first
			db.metrics.OverCapTriggerCount.Inc()

			go cleanupExpired()

			go func() {
				if !unreserveSem.TryAcquire(1) {
					// if there is already a goroutine taking care of unreserving
					// dont wait for it to finish
					return
				}
				defer unreserveSem.Release(1)

				unreserveCtx, cancelUnreserve = context.WithCancel(ctx)
				err := db.unreserve(unreserveCtx)
				if err != nil {
					db.logger.Error(err, "reserve unreserve")
				}
			}()

		case <-batchExpiryTrigger:
			go cleanupExpired()

		case <-cleanUpTicker.C:
			go cleanupExpired()

			go func() {
				// wait till we get a slot to run the cleanup. this is to ensure we
				// dont run cleanup when expirations are running
				err := expirySem.Acquire(ctx, 1)
				if err != nil {
					return
				}
				defer expirySem.Release(1)

				if err := db.reserveCleanup(ctx); err != nil {
					db.logger.Error(err, "reserve cleanup")
				}
			}()
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
				lockKey := reserveUpdateBatchLockKey(
					chunk.Stamp().BatchID(),
					db.po(chunk.Address()),
				)
				db.lock.Lock(lockKey)
				err = db.Execute(ctx, func(tx internal.Storage) error {
					newIndex, err = db.reserve.Put(ctx, tx, chunk)
					if err != nil {
						return fmt.Errorf("reserve: putter.Put: %w", err)
					}
					return nil
				})
				db.lock.Unlock(lockKey)
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

// expiredBatchItem is a storage.Item implementation for expired batches.
type expiredBatchItem struct {
	BatchID []byte
}

// ID implements storage.Item.
func (e *expiredBatchItem) ID() string {
	return string(e.BatchID)
}

// Namespace implements storage.Item.
func (e *expiredBatchItem) Namespace() string {
	return "expiredBatchItem"
}

// Marshal implements storage.Item.
// It is a no-op as expiredBatchItem is not serialized.
func (e *expiredBatchItem) Marshal() ([]byte, error) {
	return nil, nil
}

// Unmarshal implements storage.Item.
// It is a no-op as expiredBatchItem is not serialized.
func (e *expiredBatchItem) Unmarshal(_ []byte) error {
	return nil
}

// Clone implements storage.Item.
func (e *expiredBatchItem) Clone() storage.Item {
	if e == nil {
		return nil
	}
	return &expiredBatchItem{
		BatchID: slices.Clone(e.BatchID),
	}
}

// String implements storage.Item.
func (e *expiredBatchItem) String() string {
	return storageutil.JoinFields(e.Namespace(), e.ID())
}

// EvictBatch evicts all chunks belonging to a batch from the reserve.
func (db *DB) EvictBatch(ctx context.Context, batchID []byte) error {
	if db.reserve == nil {
		// if reserve is not configured, do nothing
		return nil
	}

	err := db.Execute(ctx, func(tx internal.Storage) error {
		return tx.IndexStore().Put(&expiredBatchItem{BatchID: batchID})
	})
	if err != nil {
		return fmt.Errorf("save expired batch: %w", err)
	}

	db.events.Trigger(batchExpiry)
	return nil
}

func (db *DB) evictBatch(
	ctx context.Context,
	batchID []byte,
	upToBin uint8,
) (evicted int, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.reserve.AddSize(-evicted)
		db.metrics.ReserveSize.Set(float64(db.reserve.Size()))
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "EvictBatch").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "failure").Inc()
		}
		if upToBin == swarm.MaxBins {
			db.metrics.ExpiredChunkCount.Add(float64(evicted))
		} else {
			db.metrics.EvictedChunkCount.Add(float64(evicted))
		}
	}()

	for b := uint8(0); b < upToBin; b++ {

		select {
		case <-ctx.Done():
			return evicted, ctx.Err()
		default:
		}

		binEvicted, err := func() (int, error) {
			lockKey := reserveUpdateBatchLockKey(batchID, b)
			db.lock.Lock(lockKey)
			defer db.lock.Unlock(lockKey)

			// cache evicted chunks
			cache := func(c swarm.Chunk) {
				if err := db.Cache().Put(ctx, c); err != nil {
					db.logger.Error(err, "reserve cache")
				}
			}

			return db.reserve.EvictBatchBin(ctx, db, b, batchID, cache)
		}()
		evicted += binEvicted

		// if there was an error, we still need to update the chunks that have already
		// been evicted from the reserve
		db.logger.Debug(
			"reserve eviction",
			"bin", b,
			"evicted", binEvicted,
			"batchID", hex.EncodeToString(batchID),
			"new_size", db.reserve.Size(),
		)

		if err != nil {
			return evicted, err
		}
	}

	return evicted, nil
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

	// these are batches that are known to be expired but have not been evicted yet,
	// so we should let the expiration process handle them
	knownExpiredBatches, _ := db.getExpiredBatches()

	err := db.reserve.IterateChunksItems(db.repo, 0, func(ci reserve.ChunkItem) (bool, error) {
		if exists, err := db.batchstore.Exists(ci.BatchID); err == nil && !exists {
			if !slices.ContainsFunc(knownExpiredBatches, func(b []byte) bool {
				return bytes.Equal(b, ci.BatchID)
			}) {
				itemsToEvict = append(itemsToEvict, ci)
			}
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	expiredBatches := make(map[string]struct{})
	var retErr error

	batchCnt := 1000

	for i := 0; i < len(itemsToEvict); i += batchCnt {
		end := i + batchCnt
		if end > len(itemsToEvict) {
			end = len(itemsToEvict)
		}

		retErr = db.Execute(ctx, func(tx internal.Storage) error {
			batch, err := tx.IndexStore().Batch(ctx)
			if err != nil {
				return err
			}
			var tErr error
			for _, item := range itemsToEvict[i:end] {
				chunk, err := db.ChunkStore().Get(ctx, item.ChunkAddress)
				if err == nil {
					err := db.Cache().Put(ctx, chunk)
					if err != nil {
						db.logger.Warning("reserve eviction cache put", "err", err)
					}
				}

				// safe to assume that this need not be locked as the batch is already expired
				// and cleaned up because expiry process failed to handle it
				err = db.reserve.DeleteChunk(ctx, tx, batch, item.ChunkAddress, item.BatchID)
				if err != nil {
					if errors.Is(err, storage.ErrNotFound) {
						db.reserve.CleanupBinIndex(ctx, batch, item.ChunkAddress, item.BinID)
						continue
					}
					continue
				}
				removed++
				if _, ok := expiredBatches[string(item.BatchID)]; !ok {
					expiredBatches[string(item.BatchID)] = struct{}{}
					db.logger.Debug("cleanup expired batch", "batch_id", hex.EncodeToString(item.BatchID))
				}
			}
			return tErr
		})
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

	target := db.reserve.EvictionTarget()
	if target == 0 {
		return nil
	}

	totalEvicted := 0
	for radius < swarm.MaxBins {

		err := db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {

			if totalEvicted >= target {
				return true, nil
			}

			binEvicted, err := db.evictBatch(ctx, b.ID, radius)

			// eviction happens in batches, so we need to keep track of the total
			// number of chunks evicted even if there was an error
			totalEvicted += binEvicted

			// we can only get error here for critical cases, for eg. batch commit
			// error, which is not recoverable, so we return true to stop the iteration
			if err != nil {
				return true, err
			}

			return false, nil
		})
		if err != nil {
			return err
		}
		if totalEvicted >= target {
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

	db.inFlight.Add(1)
	go func() {
		defer db.inFlight.Done()

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
