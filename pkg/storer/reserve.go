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
	"math/bits"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	reserveOverCapacity = "reserveOverCapacity"
	reserveUnreserved   = "reserveUnreserved"
	batchExpiry         = "batchExpiry"
	batchExpiryDone     = "batchExpiryDone"
)

var errMaxRadius = errors.New("max radius reached")
var reserveSizeWithinRadius atomic.Uint64

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

	db.inFlight.Add(1)
	go db.reserveWorker(ctx)

	select {
	case <-time.After(db.reserveOptions.warmupDuration):
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

	db.metrics.ReserveSizeWithinRadius.Set(float64(count))
	db.metrics.ReserveMissingBatch.Set(float64(missing))
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

			db.metrics.OverCapTriggerCount.Inc()
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
				db.metrics.StorageRadius.Set(float64(radius))
				db.logger.Info("reserve radius decrease", "radius", radius)
			}
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
		err = db.storage.Run(ctx, func(st transaction.Store) error {
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
	err := db.storage.IndexStore().Iterate(storage.Query{
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

	err := db.storage.Run(ctx, func(tx transaction.Store) error {
		return tx.IndexStore().Put(&expiredBatchItem{BatchID: batchID})
	})
	if err != nil {
		return fmt.Errorf("save expired batch: %w", err)
	}

	db.events.Trigger(batchExpiry)
	return nil
}

func (db *DB) ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte, stampHash []byte) (ch swarm.Chunk, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveGet").Observe(dur())
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveGet", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveGet", "failure").Inc()
			db.logger.Debug("reserve get error", "error", err)
		}
	}()

	return db.reserve.Get(ctx, addr, batchID, stampHash)
}

func (db *DB) ReserveHas(addr swarm.Address, batchID []byte, stampHash []byte) (has bool, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveHas").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHas", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHas", "failure").Inc()
			db.logger.Debug("reserve has error", "error", err)
		}
	}()

	return db.reserve.Has(addr, batchID, stampHash)
}

// ReservePutter returns a Putter for inserting chunks into the reserve.
func (db *DB) ReservePutter() storage.Putter {
	return putterWithMetrics{
		storage.PutterFunc(
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
		db.metrics.StorageRadius.Set(float64(radius))
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

func (db *DB) CommittedDepth() uint8 {
	if db.reserve == nil {
		return 0
	}

	return uint8(db.reserveOptions.capacityDoubling) + db.reserve.Radius()
}

func (db *DB) ReserveSize() int {
	if db.reserve == nil {
		return 0
	}
	return db.reserve.Size()
}

func (db *DB) ReserveSizeWithinRadius() uint64 {
	return reserveSizeWithinRadius.Load()
}

func (db *DB) IsWithinStorageRadius(addr swarm.Address) bool {
	if db.reserve == nil {
		return false
	}
	return swarm.Proximity(addr.Bytes(), db.baseAddr.Bytes()) >= db.reserve.Radius()
}

// BinC is the result returned from the SubscribeBin channel that contains the chunk address and the binID
type BinC struct {
	Address   swarm.Address
	BinID     uint64
	BatchID   []byte
	StampHash []byte
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

			err := db.reserve.IterateBin(bin, start, func(a swarm.Address, binID uint64, batchID, stampHash []byte) (bool, error) {
				select {
				case out <- &BinC{Address: a, BinID: binID, BatchID: batchID, StampHash: stampHash}:
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

type NeighborhoodStat struct {
	Neighborhood            swarm.Neighborhood
	ReserveSizeWithinRadius int
	Proximity               uint8
}

func (db *DB) NeighborhoodsStat(ctx context.Context) ([]*NeighborhoodStat, error) {

	radius := db.StorageRadius()
	committedDepth := db.CommittedDepth()

	prefixes := neighborhoodPrefixes(db.baseAddr, int(radius), db.reserveOptions.capacityDoubling)
	neighs := make([]*NeighborhoodStat, len(prefixes))
	for i, n := range prefixes {
		neighs[i] = &NeighborhoodStat{
			Neighborhood:            swarm.NewNeighborhood(n, committedDepth),
			ReserveSizeWithinRadius: 0,
			Proximity:               min(committedDepth, swarm.Proximity(n.Bytes(), db.baseAddr.Bytes()))}
	}

	err := db.reserve.IterateChunksItems(0, func(ch *reserve.ChunkBinItem) (bool, error) {
		for _, n := range neighs {
			if swarm.Proximity(ch.Address.Bytes(), n.Neighborhood.Bytes()) >= committedDepth {
				n.ReserveSizeWithinRadius++
				break
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return neighs, err
}

func neighborhoodPrefixes(base swarm.Address, radius int, suffixLength int) []swarm.Address {
	bitCombinationsCount := int(math.Pow(2, float64(suffixLength)))
	bitSuffixes := make([]uint8, bitCombinationsCount)

	for i := 0; i < bitCombinationsCount; i++ {
		bitSuffixes[i] = uint8(i)
	}

	binPrefixes := make([]swarm.Address, bitCombinationsCount)

	// copy base address
	for i := range binPrefixes {
		binPrefixes[i] = base.Clone()
	}

	for j := range binPrefixes {
		pseudoAddrBytes := binPrefixes[j].Bytes()

		// set pseudo suffix
		bitSuffixPos := suffixLength - 1
		for l := radius + 0; l < radius+suffixLength+1; l++ {
			index, pos := l/8, l%8

			if hasBit(bitSuffixes[j], uint8(bitSuffixPos)) {
				pseudoAddrBytes[index] = bits.Reverse8(setBit(bits.Reverse8(pseudoAddrBytes[index]), uint8(pos)))
			} else {
				pseudoAddrBytes[index] = bits.Reverse8(clearBit(bits.Reverse8(pseudoAddrBytes[index]), uint8(pos)))
			}

			bitSuffixPos--
		}

		// clear rest of the bits
		for l := radius + suffixLength + 1; l < len(pseudoAddrBytes)*8; l++ {
			index, pos := l/8, l%8
			pseudoAddrBytes[index] = bits.Reverse8(clearBit(bits.Reverse8(pseudoAddrBytes[index]), uint8(pos)))
		}
	}

	return binPrefixes
}

// Clears the bit at pos in n.
func clearBit(n, pos uint8) uint8 {
	mask := ^(uint8(1) << pos)
	return n & mask
}

// Sets the bit at pos in the integer n.
func setBit(n, pos uint8) uint8 {
	return n | 1<<pos
}

func hasBit(n, pos uint8) bool {
	return n&(1<<pos) > 0
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
