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

	cleanupDur = time.Hour * 6
)

var errMaxRadius = errors.New("max radius reached")

type Syncer interface {
	// Number of active historical syncing jobs.
	SyncRate() float64
	Start(context.Context)
}

func threshold(capacity int) int { return capacity * 5 / 10 }

func (db *DB) reserveWorker(ctx context.Context, warmupDur, wakeUpDur time.Duration, radius func() (uint8, error)) {
	defer db.reserveWg.Done()

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-db.quit
		cancel()
	}()

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

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

	radiusWakeUpTicker := time.NewTicker(wakeUpDur)
	defer radiusWakeUpTicker.Stop()

	cleanUpTicker := time.NewTicker(cleanupDur)
	defer cleanUpTicker.Stop()

	for {
		select {
		case <-overCapTrigger:
			err := db.unreserve(ctx)
			if err != nil {
				db.logger.Error(err, "reserve unreserve")
			}
			db.metrics.OverCapTriggerCount.Inc()
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
		case <-cleanUpTicker.C:
			if err := db.reserveCleanup(ctx); err != nil {
				db.logger.Error(err, "cleanup")
			}
		case <-db.quit:
			return
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
				db.lock.Lock(reserveUpdateLockKey)
				defer db.lock.Unlock(reserveUpdateLockKey)

				var (
					newIndex bool
				)
				err = db.Do(ctx, func(trx internal.Storage) error {
					newIndex, err = db.reserve.Put(ctx, trx, chunk)
					if err != nil {
						return fmt.Errorf("reserve: putter.Put: %w", err)
					}
					return nil
				})
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
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "EvictBatch").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "failure").Inc()
		}
	}()
	return db.evictBatch(ctx, batchID, swarm.MaxBins)
}

func (db *DB) evictBatch(ctx context.Context, batchID []byte, upToBin uint8) (err error) {

	for b := uint8(0); b < upToBin; b++ {

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var evicted int

		err = db.reserve.IterateBatchBin(ctx, db.repo, b, batchID, func(address swarm.Address) (bool, error) {
			err := db.removeChunk(ctx, address, batchID)
			if err != nil {
				return false, err
			}
			evicted++
			return false, nil
		})

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
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) removeChunk(ctx context.Context, address swarm.Address, batchID []byte) error {

	return db.Do(ctx, func(txnRepo internal.Storage) error {
		chunk, err := db.ChunkStore().Get(ctx, address)
		if err == nil {
			err := db.Cache().Put(ctx, chunk)
			if err != nil {
				db.logger.Warning("reserve eviction cache put", "err", err)
			}
		}

		db.lock.Lock(reserveUpdateLockKey)
		defer db.lock.Unlock(reserveUpdateLockKey)

		err = db.reserve.DeleteChunk(ctx, txnRepo, address, batchID)
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

	ids := map[string]struct{}{}

	err := db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {
		ids[string(b.ID)] = struct{}{}
		return false, nil
	})
	if err != nil {
		return err
	}

	return db.reserve.IterateChunksItems(db.repo, 0, func(ci reserve.ChunkItem) (bool, error) {
		if _, ok := ids[string(ci.BatchID)]; !ok {
			removed++
			db.logger.Debug("cleanup expired batch", "batch_id", hex.EncodeToString(ci.BatchID))
			return false, db.removeChunk(ctx, ci.ChunkAddress, ci.BatchID)
		}
		return false, nil
	})
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
