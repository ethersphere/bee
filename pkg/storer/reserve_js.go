//go:build js
// +build js

package storer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func (db *DB) evictBatch(
	ctx context.Context,
	batchID []byte,
	evictCount int,
	upToBin uint8,
) (evicted int, err error) {
	defer func() {
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

func (db *DB) ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte, stampHash []byte) (ch swarm.Chunk, err error) {

	defer func() {
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			db.logger.Debug("reserve get error", "error", err)
		}
	}()

	return db.reserve.Get(ctx, addr, batchID, stampHash)
}

func (db *DB) ReserveHas(addr swarm.Address, batchID []byte, stampHash []byte) (has bool, err error) {

	defer func() {
		if err != nil {
			db.logger.Debug("reserve has error", "error", err)
		}
	}()

	return db.reserve.Has(addr, batchID, stampHash)
}

// ReservePutter returns a Putter for inserting chunks into the reserve.
func (db *DB) ReservePutter() storage.Putter {
	return storage.PutterFunc(
		func(ctx context.Context, chunk swarm.Chunk) error {
			err := db.reserve.Put(ctx, chunk)
			if err != nil {
				db.logger.Debug("reserve put error", "error", err)
				return fmt.Errorf("reserve putter.Put: %w", err)
			}
			db.reserveBinEvents.Trigger(string(db.po(chunk.Address())))
			if !db.reserve.IsWithinCapacity() {
				db.events.Trigger(reserveOverCapacity)
			}
			return nil
		},
	)
}

func (db *DB) unreserve(ctx context.Context) (err error) {
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
				db.logger.Debug("stopping unreserve, received batch expiration signal")
				return nil
			default:
			}

			evict := target - totalEvicted
			if evict < int(db.reserveOptions.minEvictCount) { // evict at least a min count
				evict = int(db.reserveOptions.minEvictCount)
			}

			binEvicted, err := db.evictBatch(ctx, b, evict, radius)
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

func (db *DB) startReserveWorkers(
	ctx context.Context,
	radius func() (uint8, error),
) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-db.quit
		cancel()
	}()

	db.inFlight.Add(1)
	go db.reserveWorker(ctx)

	sub, unsubscribe := db.reserveOptions.startupStabilizer.Subscribe()
	defer unsubscribe()

	select {
	case <-sub:
		db.logger.Debug("node warmup check completed")
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
	}

	// syncing can now begin now that the reserver worker is running
	db.syncer.Start(ctx)
}

func (db *DB) countWithinRadius(ctx context.Context) (int, error) {
	count := 0
	missing := 0
	radius := db.StorageRadius()

	evictBatches := make(map[string]bool)
	err := db.reserve.IterateChunksItems(0, func(ci *reserve.ChunkBinItem) (bool, error) {
		if ci.Bin >= radius {
			count++
		}

		if exists, err := db.batchstore.Exists(ci.BatchID); err == nil && !exists {
			missing++
			evictBatches[string(ci.BatchID)] = true
		}
		return false, nil
	})
	if err != nil {
		return 0, err
	}

	for batch := range evictBatches {
		db.logger.Debug("reserve: invalid batch", "batch_id", hex.EncodeToString([]byte(batch)))
		err = errors.Join(err, db.EvictBatch(ctx, []byte(batch)))
	}

	reserveSizeWithinRadius.Store(uint64(count))

	return count, err
}

func (db *DB) reserveWorker(ctx context.Context) {
	defer db.inFlight.Done()

	batchExpiryTrigger, batchExpiryUnsub := db.events.Subscribe(batchExpiry)
	defer batchExpiryUnsub()

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

	thresholdTicker := time.NewTicker(db.reserveOptions.wakeupDuration)
	defer thresholdTicker.Stop()

	_, _ = db.countWithinRadius(ctx)

	if !db.reserve.IsWithinCapacity() {
		db.events.Trigger(reserveOverCapacity)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-batchExpiryTrigger:

			err := db.evictExpiredBatches(ctx)
			if err != nil {
				db.logger.Warning("reserve worker evict expired batches", "error", err)
			}

			db.events.Trigger(batchExpiryDone)

			if !db.reserve.IsWithinCapacity() {
				db.events.Trigger(reserveOverCapacity)
			}

		case <-overCapTrigger:

			if err := db.unreserve(ctx); err != nil {
				db.logger.Warning("reserve worker unreserve", "error", err)
			}

		case <-thresholdTicker.C:

			radius := db.reserve.Radius()
			count, err := db.countWithinRadius(ctx)
			if err != nil {
				db.logger.Warning("reserve worker count within radius", "error", err)
				continue
			}

			if count < threshold(db.reserve.Capacity()) && db.syncer.SyncRate() == 0 && radius > db.reserveOptions.minimumRadius {
				radius--
				if err := db.reserve.SetRadius(radius); err != nil {
					db.logger.Error(err, "reserve set radius")
				}

				db.logger.Info("reserve radius decrease", "radius", radius)
			}
		}
	}
}
