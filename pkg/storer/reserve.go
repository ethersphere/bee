// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	reserveOverCapacity = "reserveOverCapacity"
	reserveUnreserved   = "reserveUnreserved"
	batchExpiry         = "batchExpiry"
	batchExpiryDone     = "batchExpiryDone"
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
	case <-time.After(db.opts.warmupDuration):
	case <-db.quit:
		return
	}

	r, err := radius()
	if err != nil {
		db.logger.Error(err, "reserve worker initial radius")
		return // node shutdown
	}

	err = db.reserve.SetRadius(r)
	if err != nil {
		db.logger.Error(err, "reserve set radius")
	} else {
		db.metrics.StorageRadius.Set(float64(r))
	}

	// syncing can now begin now that the reserver worker is running
	db.syncer.Start(ctx)

	db.inFlight.Add(2)
	go db.reserveSizeWithinRadiusWorker(ctx)
}

func (db *DB) reserveSizeWithinRadiusWorker(ctx context.Context) {
	defer db.inFlight.Done()

	var activeEviction atomic.Bool
	go func() {
		defer db.inFlight.Done()

		expiryTrigger, expiryUnsub := db.events.Subscribe(batchExpiry)
		defer expiryUnsub()

		expiryDoneTrigger, expiryDoneUnsub := db.events.Subscribe(batchExpiryDone)
		defer expiryDoneUnsub()

		for {
			select {
			case <-ctx.Done():
				return
			case <-expiryTrigger:
				activeEviction.Store(true)
			case <-expiryDoneTrigger:
				activeEviction.Store(false)
			}
		}
	}()

	countFn := func() int {
		skipInvalidCheck := activeEviction.Load()

		count := 0
		missing := 0
		radius := db.StorageRadius()

		evictBatches := make(map[string]bool)

		err := db.storage.Run(func(t transaction.Store) error {
			return db.reserve.IterateChunksItems(0, func(ci reserve.ChunkItem) (bool, error) {
				if ci.Bin >= radius {
					count++
				}

				if skipInvalidCheck {
					return false, nil
				}

				if exists, err := db.batchstore.Exists(ci.BatchID); err == nil && !exists {
					missing++
					evictBatches[string(ci.BatchID)] = true
				}
				return false, nil
			})
		})
		if err != nil {
			db.logger.Error(err, "reserve count within radius")
		}

		for batch := range evictBatches {
			db.logger.Debug("reserve size worker, invalid batch id", "batch_id", hex.EncodeToString([]byte(batch)))
			if err := db.EvictBatch(ctx, []byte(batch)); err != nil {
				db.logger.Warning("reserve size worker, batch eviction", "batch_id", hex.EncodeToString([]byte(batch)), "error", err)
			}
		}

		db.metrics.ReserveSizeWithinRadius.Set(float64(count))
		if !skipInvalidCheck {
			db.metrics.ReserveMissingBatch.Set(float64(missing))
		}

		return count
	}

	// initial run for the metrics
	_ = countFn()

	ticker := time.NewTicker(db.opts.wakeupDuration)
	defer ticker.Stop()

	for {

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		count := countFn()
		radius := db.StorageRadius()

		if count < threshold(db.reserve.Capacity()) && db.syncer.SyncRate() == 0 && radius > 0 {
			radius--
			err := db.reserve.SetRadius(radius)
			if err != nil {
				db.logger.Error(err, "reserve set radius")
			}

			db.logger.Info("reserve radius decrease", "radius", radius)
		}
		db.metrics.StorageRadius.Set(float64(radius))
	}
}

func (db *DB) evictionWorker(ctx context.Context) {
	defer db.inFlight.Done()

	batchExpiryTrigger, batchExpiryUnsub := db.events.Subscribe(batchExpiry)
	defer batchExpiryUnsub()

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

	for {
		select {
		case <-batchExpiryTrigger:

			err := db.evictExpiredBatches(ctx)
			if err != nil {
				db.logger.Warning("eviction worker expired batches", "error", err)
			}

			db.events.Trigger(batchExpiryDone)

			if !db.reserve.IsWithinCapacity() {
				db.events.Trigger(reserveOverCapacity)
			}

		case <-overCapTrigger:
			db.metrics.OverCapTriggerCount.Inc()
			err := db.unreserve(ctx)
			if err != nil {
				db.logger.Error(err, "eviction worker unreserve")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (db *DB) evictExpiredBatches(ctx context.Context) error {

	batches, err := db.getExpiredBatches()
	if err != nil {
		return err
	}

	for _, batchID := range batches {
		evicted, err := db.evictBatch(ctx, batchID, math.MaxInt, swarm.MaxBins)
		if err != nil {
			return err
		}
		if evicted > 0 {
			db.logger.Debug("evicted expired batch", "batch_id", hex.EncodeToString(batchID), "total_evicted", evicted)
		}
		err = db.storage.Run(func(st transaction.Store) error {
			return st.IndexStore().Delete(&expiredBatchItem{BatchID: batchID})
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) getExpiredBatches() ([][]byte, error) {
	var batchesToEvict [][]byte
	err := db.storage.ReadOnly().IndexStore().Iterate(storage.Query{
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

func (db *DB) evictBatch(
	ctx context.Context,
	batchID []byte,
	evictCount int,
	upToBin uint8,
) (evicted int, err error) {
	dur := captureDuration(time.Now())
	defer func() {
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
		db.logger.Debug(
			"reserve eviction",
			"uptoBin", upToBin,
			"evicted", evicted,
			"batchID", hex.EncodeToString(batchID),
			"new_size", db.reserve.Size(),
		)
	}()

	return db.reserve.EvictBatchBin(ctx, batchID, evictCount, upToBin)
}

// EvictBatch evicts all chunks belonging to a batch from the reserve.
func (db *DB) EvictBatch(ctx context.Context, batchID []byte) error {
	if db.reserve == nil {
		// if reserve is not configured, do nothing
		return nil
	}

	err := db.storage.Run(func(tx transaction.Store) error {
		return tx.IndexStore().Put(&expiredBatchItem{BatchID: batchID})
	})
	if err != nil {
		return fmt.Errorf("save expired batch: %w", err)
	}

	db.events.Trigger(batchExpiry)
	return nil
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

	return db.reserve.Get(ctx, addr, batchID)
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

	return db.reserve.Has(addr, batchID)
}

// ReservePutter returns a Putter for inserting chunks into the reserve.
func (db *DB) ReservePutter() storage.Putter {
	return putterWithMetrics{
		storage.PutterFunc(
			func(ctx context.Context, chunk swarm.Chunk) error {
				err := db.reserve.Put(ctx, chunk)
				if err != nil {
					db.logger.Debug("reserve put error", "error", err)
					return fmt.Errorf("reserve: putter.Put: %w", err)
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
	if target <= 0 {
		return nil
	}
	db.logger.Info("unreserve start", "target", target, "radius", radius)

	batchExpiry, unsub := db.events.Subscribe(batchExpiry)
	defer unsub()

	totalEvicted := 0

	var batches [][]byte
	err = db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {
		batches = append(batches, b.ID)
		return false, nil
	})
	if err != nil {
		return err
	}

	for radius < swarm.MaxBins {

		for _, b := range batches {

			select {
			case <-batchExpiry:
				return nil
			default:
			}

			binEvicted, err := db.evictBatch(ctx, b, target-totalEvicted, radius)
			// eviction happens in batches, so we need to keep track of the total
			// number of chunks evicted even if there was an error
			totalEvicted += binEvicted

			// we can only get error here for critical cases, for eg. batch commit
			// error, which is not recoverable
			if err != nil {
				return err
			}

			if totalEvicted >= target {
				db.logger.Info("unreserve finished", "evicted", totalEvicted, "radius", radius)
				return nil
			}
		}

		radius++
		db.logger.Info("reserve radius increase", "radius", radius)
		_ = db.reserve.SetRadius(radius)
	}

	return errMaxRadius
}

// ReserveLastBinIDs returns all of the highest binIDs from all the bins in the reserve and the epoch time of the reserve.
func (db *DB) ReserveLastBinIDs() ([]uint64, uint64, error) {
	if db.reserve == nil {
		return nil, 0, nil
	}

	return db.reserve.LastBinIDs()
}

func (db *DB) ReserveIterateChunks(cb func(swarm.Chunk) (bool, error)) error {
	return db.reserve.IterateChunks(0, cb)
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

			err := db.reserve.IterateBin(bin, start, func(a swarm.Address, binID uint64, batchID []byte) (bool, error) {
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

func (db *DB) po(addr swarm.Address) uint8 {
	return swarm.Proximity(db.baseAddr.Bytes(), addr.Bytes())
}
