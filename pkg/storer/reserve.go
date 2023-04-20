// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/postage"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

const (
	reserveOverCapacity = "reserveOverCapacity"
	reserveUnreserved   = "reserveUnreserved"
	reserveLock         = "reserveLock"
)

var errMaxRadius = errors.New("max radius reached")

type SyncReporter interface {
	// Number of active historical syncing jobs.
	SyncRate() float64
}

func threshold(capacity int) int { return capacity * 4 / 10 }

func (db *DB) reserveWorker(capacity int, warmupDur, wakeUpDur time.Duration) {
	defer db.reserveWg.Done()

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

	select {
	case <-time.After(warmupDur):
	case <-db.quit:
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-db.quit
		cancel()
	}()

	wakeUpTimer := time.NewTicker(wakeUpDur)

	for {
		select {
		case <-overCapTrigger:
			err := db.unreserve(ctx)
			if err != nil {
				db.logger.Error(err, "reserve unreserve")
			}
		case <-wakeUpTimer.C:
			radius := db.reserve.Radius()
			if db.reserve.Size() < threshold(capacity) && db.syncer.SyncRate() == 0 && radius > 0 {
				radius--
				err := db.reserve.SetRadius(db.repo.IndexStore(), radius)
				if err != nil {
					db.logger.Error(err, "reserve set radius")
				}
				db.logger.Info("reserve radius decrease", "radius", radius)
			}
			wakeUpTimer.Reset(wakeUpDur)
		case <-db.quit:
			return
		}
	}
}

func (db *DB) ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte) (chunk swarm.Chunk, err error) {
	defer func() {
		dur := captureDuration(time.Now())
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
	return db.reserve.Size()
}

func (db *DB) IsWithinStorageRadius(addr swarm.Address) bool {
	if db.reserve == nil {
		return false
	}
	return swarm.Proximity(addr.Bytes(), db.baseAddr.Bytes()) >= db.reserve.Radius()
}

func (db *DB) ReserveHas(addr swarm.Address, batchID []byte) (has bool, err error) {
	defer func() {
		dur := captureDuration(time.Now())
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReserveHas").Observe(dur())
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHas", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReserveHas", "failure").Inc()
		}
	}()

	return db.reserve.Has(db.repo.IndexStore(), addr, batchID)
}

func (db *DB) ReservePut(ctx context.Context, chunk swarm.Chunk) (err error) {
	defer func() {
		dur := captureDuration(time.Now())
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReservePut").Observe(dur())
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReservePut", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "ReservePut", "failure").Inc()
		}
	}()

	putter := db.ReservePutter(ctx)
	if err := putter.Put(ctx, chunk); err != nil {
		return errors.Join(err, putter.Cleanup())
	}
	return putter.Done(swarm.ZeroAddress)
}

// ReservePutter returns a PutterSession for inserting chunks into the reserve.
func (db *DB) ReservePutter(ctx context.Context) PutterSession {

	trx, commit, rollback := db.repo.NewTx(ctx)

	triggerBins := make(map[uint8]bool)
	count := 0

	db.reserveMtx.RLock()

	return &putterSession{
		Putter: putterWithMetrics{
			storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
				newIndex, err := db.reserve.Put(ctx, trx, chunk)
				if err != nil {
					return err
				}
				triggerBins[db.po(chunk.Address())] = true
				if newIndex {
					count++
				}
				return nil
			}),
			db.metrics,
			"reserve",
		},
		done: func(swarm.Address) error {
			defer db.reserveMtx.RUnlock()
			err := commit()
			if err != nil {
				return err
			}
			db.reserve.AddSize(count)
			for bin := range triggerBins {
				db.reserveBinEvents.Trigger(string(bin))
			}
			if !db.reserve.IsWithinCapacity() {
				db.events.Trigger(reserveOverCapacity)
			}
			return nil
		},
		cleanup: func() error {
			defer db.reserveMtx.RUnlock()
			return rollback()
		},
	}
}

// EvictBatch evicts all chunks belonging to a batch from the reserve.
func (db *DB) EvictBatch(ctx context.Context, batchID []byte) (err error) {
	defer func() {
		dur := captureDuration(time.Now())
		db.metrics.MethodCallsDuration.WithLabelValues("reserve", "EvictBatch").Observe(dur())
		if err == nil || errors.Is(err, storage.ErrNotFound) {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("reserve", "EvictBatch", "failure").Inc()
		}
	}()

	return db.evictBatch(ctx, batchID, swarm.MaxBins)
}

func (db *DB) evictBatch(ctx context.Context, batchID []byte, upToBin uint8) error {

	db.reserveMtx.Lock()
	defer db.reserveMtx.Unlock()

	for b := uint8(0); b < upToBin; b++ {

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		txnRepo, commit, rollback := db.repo.NewTx(ctx)

		// cache evicted chunks
		cache := func(c swarm.Chunk) {
			if err := db.Cache().Put(ctx, c); err != nil {
				db.logger.Error(err, "reserve cache")
			}
		}

		evicted, err := db.reserve.EvictBatchBin(ctx, txnRepo, b, batchID, cache)
		if err != nil {
			_ = rollback()
			return err
		}

		err = commit()
		if err != nil {
			return err
		}

		db.logger.Info("reserve eviction", "bin", b, "evicted", evicted, "batchID", hex.EncodeToString(batchID), "size", db.reserve.Size())

		db.reserve.AddSize(-evicted)
	}

	return nil
}

func (db *DB) unreserve(ctx context.Context) error {

	radius := db.reserve.Radius()
	defer db.events.Trigger(reserveUnreserved)

	for radius < swarm.MaxPO {

		err := db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {

			if db.reserve.IsWithinCapacity() {
				return true, nil
			}

			err := db.evictBatch(ctx, b.ID, radius)
			if err != nil {
				return false, err
			}

			return false, nil
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

type Sample struct {
	Items []swarm.Address
	Hash  swarm.Address
}

// ReserveSample generates the sample of reserve storage of a node required for the
// storage incentives agent to participate in the lottery round. In order to generate
// this sample we need to iterate through all the chunks in the node's reserve and
// calculate the transformed hashes of all the chunks using the anchor as the salt.
// In order to generate the transformed hashes, we will use the std hmac keyed-hash
// implementation by using the anchor as the key. Nodes need to calculate the sample
// in the most optimal way and there are time restrictions. The lottery round is a
// time based round, so nodes participating in the round need to perform this
// calculation within the round limits.
// In order to optimize this we use a simple pipeline pattern:
// Iterate chunk addresses -> Get the chunk data and calculate transformed hash -> Assemble the sample
func (db *DB) ReserveSample(
	ctx context.Context,
	anchor []byte,
	storageRadius uint8,
	consensusTime uint64,
) (Sample, error) {

	g, ctx := errgroup.WithContext(ctx)

	chunkC := make(chan swarm.Chunk)
	var stat sampleStat

	t := time.Now()

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		defer close(chunkC)
		iterationStart := time.Now()

		err := db.reserve.IterateChunks(db.repo, storageRadius, func(a swarm.Chunk) (bool, error) {
			select {
			case chunkC <- a:
				stat.TotalIterated.Inc()
				return false, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		})
		if err != nil {
			return err
		}

		stat.IterationDuration.Add(time.Since(iterationStart).Nanoseconds())
		return nil
	})

	// Phase 2: Get the chunk data and calculate transformed hash
	sampleItemChan := make(chan sampleEntry)
	const workers = 6
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			hmacr := hmac.New(swarm.NewHasher, anchor)

			for chunk := range chunkC {
				getStart := time.Now()

				stat.GetDuration.Add(time.Since(getStart).Nanoseconds())

				// check if the timestamp on the postage stamp is not later than
				// the consensus time.
				if binary.BigEndian.Uint64(chunk.Stamp().Timestamp()) > consensusTime {
					stat.NewIgnored.Inc()
					continue
				}

				hmacrStart := time.Now()
				_, err := hmacr.Write(chunk.Data())
				if err != nil {
					return err
				}
				taddr := hmacr.Sum(nil)
				hmacr.Reset()
				stat.HmacrDuration.Add(time.Since(hmacrStart).Nanoseconds())

				select {
				case sampleItemChan <- sampleEntry{transformedAddress: swarm.NewAddress(taddr), chunk: chunk}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			return nil
		})
	}

	go func() {
		_ = g.Wait()
		close(sampleItemChan)
	}()

	sampleItems := make([]swarm.Address, 0, sampleSize)
	// insert function will insert the new item in its correct place. If the sample
	// size goes beyond what we need we omit the last item.
	insert := func(item swarm.Address) {
		added := false
		for i, sItem := range sampleItems {
			if le(item.Bytes(), sItem.Bytes()) {
				sampleItems = append(sampleItems[:i+1], sampleItems[i:]...)
				sampleItems[i] = item
				added = true
				break
			}
		}
		if len(sampleItems) > sampleSize {
			sampleItems = sampleItems[:sampleSize]
		}
		if len(sampleItems) < sampleSize && !added {
			sampleItems = append(sampleItems, item)
		}
	}

	// Phase 3: Assemble the sample. Here we need to assemble only the first sampleSize
	// no of items from the results of the 2nd phase.
	for item := range sampleItemChan {
		var currentMaxAddr swarm.Address
		if len(sampleItems) > 0 {
			currentMaxAddr = sampleItems[len(sampleItems)-1]
		} else {
			currentMaxAddr = swarm.NewAddress(make([]byte, 32))
		}
		if le(item.transformedAddress.Bytes(), currentMaxAddr.Bytes()) || len(sampleItems) < sampleSize {
			insert(item.transformedAddress)

			// TODO: STAMP VALIDATION
		}
	}

	if err := g.Wait(); err != nil {
		return Sample{}, fmt.Errorf("sampler: failed creating sample: %w", err)
	}

	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	for _, s := range sampleItems {
		_, err := hasher.Write(s.Bytes())
		if err != nil {
			return Sample{}, fmt.Errorf("sampler: failed creating root hash of sample: %w", err)
		}
	}
	hash := hasher.Sum(nil)

	sample := Sample{
		Items: sampleItems,
		Hash:  swarm.NewAddress(hash),
	}

	db.logger.Info("reserve sampler done", "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", stat, "sample", sample)

	return sample, nil
}

func (db *DB) po(addr swarm.Address) uint8 {
	return swarm.Proximity(db.baseAddr.Bytes(), addr.Bytes())
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b []byte) bool {
	return bytes.Compare(a, b) == -1
}

const sampleSize = 8

type sampleStat struct {
	TotalIterated      atomic.Int64
	NotFound           atomic.Int64
	NewIgnored         atomic.Int64
	IterationDuration  atomic.Int64
	GetDuration        atomic.Int64
	HmacrDuration      atomic.Int64
	ValidStampDuration atomic.Int64
}

type sampleEntry struct {
	transformedAddress swarm.Address
	chunk              swarm.Chunk
}

func (s sampleStat) String() string {

	seconds := int64(time.Second)

	return fmt.Sprintf(
		"Chunks: %d NotFound: %d New Ignored: %d Iteration Duration: %d secs GetDuration: %d secs"+
			" HmacrDuration: %d secs ValidStampDuration: %d secs",
		s.TotalIterated.Load(),
		s.NotFound.Load(),
		s.NewIgnored.Load(),
		s.IterationDuration.Load()/seconds,
		s.GetDuration.Load()/seconds,
		s.HmacrDuration.Load()/seconds,
		s.ValidStampDuration.Load()/seconds,
	)
}
