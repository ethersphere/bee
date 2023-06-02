// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
	storage "github.com/ethersphere/bee/pkg/storage"
	chunk "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

const (
	reserveOverCapacity  = "reserveOverCapacity"
	reserveUnreserved    = "reserveUnreserved"
	sampleSize           = 16
	reserveUpdateLockKey = "reserveUpdateLockKey"
)

var errMaxRadius = errors.New("max radius reached")

type Syncer interface {
	// Number of active historical syncing jobs.
	SyncRate() float64
	Start()
}

func threshold(capacity int) int { return capacity * 5 / 10 }

func (db *DB) reserveWorker(capacity int, warmupDur, wakeUpDur time.Duration, radius func() (uint8, error)) {
	defer db.reserveWg.Done()

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

	select {
	case <-time.After(warmupDur):
	case <-db.quit:
		return
	}

	// possibly a fresh node, acquire initial radius externally
	if db.StorageRadius() == 0 {
		r, err := radius()
		if err != nil {
			db.logger.Error(err, "reserve worker initial radius")
		} else {
			if err := db.reserve.SetRadius(db.repo.IndexStore(), r); err != nil {
				db.logger.Error(err, "reserve set radius")
			}
		}
	}

	// syncing can now begin now that the reserver worker is running
	db.syncer.Start()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-db.quit
		cancel()
	}()

	wakeUpTicker := time.NewTicker(wakeUpDur)

	for {
		select {
		case <-overCapTrigger:
			err := db.unreserve(ctx)
			if err != nil {
				db.logger.Error(err, "reserve unreserve")
			}
			db.metrics.OverCapTriggerCount.Inc()
		case <-wakeUpTicker.C:
			radius := db.reserve.Radius()
			if db.reserve.Size() < threshold(capacity) && db.syncer.SyncRate() == 0 && radius > 0 {
				radius--
				err := db.reserve.SetRadius(db.repo.IndexStore(), radius)
				if err != nil {
					db.logger.Error(err, "reserve set radius")
				}
				db.logger.Info("reserve radius decrease", "radius", radius)
			}
			db.metrics.StorageRadius.Set(float64(radius))
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
				dur := captureDuration(time.Now())
				defer func() {
					db.metrics.MethodCallsDuration.WithLabelValues("reserve", "ReservePut").Observe(dur())
					if err == nil {
						db.metrics.MethodCalls.WithLabelValues("reserve", "ReservePut", "success").Inc()
					} else {
						db.metrics.MethodCalls.WithLabelValues("reserve", "ReservePut", "failure").Inc()
					}
				}()

				db.lock.Lock(reserveUpdateLockKey)
				defer db.lock.Unlock(reserveUpdateLockKey)

				trx, commit, rollback := db.repo.NewTx(ctx)
				newIndex, err := db.reserve.Put(ctx, trx, chunk)
				if err != nil {
					return errors.Join(err, rollback())
				}
				if err := commit(); err != nil {
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

		err := func() error {
			db.lock.Lock(reserveUpdateLockKey)
			defer db.lock.Unlock(reserveUpdateLockKey)

			txnRepo, commit, rollback := db.repo.NewTx(ctx)

			// cache evicted chunks
			cache := func(c swarm.Chunk) {
				if err := db.Cache().Put(ctx, c); err != nil {
					db.logger.Error(err, "reserve cache")
				}
			}

			evicted, err := db.reserve.EvictBatchBin(ctx, txnRepo, b, batchID, cache)
			if err != nil {
				return errors.Join(err, rollback())
			}

			err = commit()
			if err != nil {
				return err
			}

			db.logger.Info("reserve eviction", "bin", b, "evicted", evicted, "batchID", hex.EncodeToString(batchID), "size", db.reserve.Size())

			db.reserve.AddSize(-evicted)
			db.metrics.ReserveSize.Set(float64(db.reserve.Size()))
			if upToBin == swarm.MaxBins {
				db.metrics.ExpiredChunkCount.Add(float64(evicted))
			} else {
				db.metrics.EvictedChunkCount.Add(float64(evicted))
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) batchesBelowValue(until *big.Int) (map[string]struct{}, error) {
	res := make(map[string]struct{})

	if until == nil {
		return res, nil
	}

	err := db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {
		if b.Value.Cmp(until) < 0 {
			res[string(b.ID)] = struct{}{}
		}
		return false, nil
	})

	return res, err
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

type SampleItem struct {
	TransformedAddress swarm.Address
	ChunkAddress       swarm.Address
	ChunkData          []byte
	Stamp              *postage.Stamp
}

func newStamp(s swarm.Stamp) *postage.Stamp {
	return postage.NewStamp(s.BatchID(), s.Index(), s.Timestamp(), s.Sig())
}

type Sample struct {
	Items []SampleItem
}

func RandSample(t *testing.T, anchor []byte) Sample {
	t.Helper()

	hasher := bmt.NewTrHasher(anchor)

	items := make([]SampleItem, sampleSize)
	for i := 0; i < sampleSize; i++ {
		ch := chunk.GenerateTestRandomChunk()

		tr, err := transformedAddress(hasher, ch, swarm.ChunkTypeContentAddressed)
		if err != nil {
			t.Fatal(err)
		}

		items[i] = SampleItem{
			TransformedAddress: tr,
			ChunkAddress:       ch.Address(),
			ChunkData:          ch.Data(),
			Stamp:              newStamp(ch.Stamp()),
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].TransformedAddress.Compare(items[j].TransformedAddress) == -1
	})

	return Sample{Items: items}
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
	minBatchBalance *big.Int,
) (Sample, error) {

	g, ctx := errgroup.WithContext(ctx)

	chunkC := make(chan reserve.ChunkItem)
	var stat sampleStat

	excludedBatchIDs, err := db.batchesBelowValue(minBatchBalance)
	if err != nil {
		db.logger.Error(err, "get batches below value")
	}

	t := time.Now()

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		defer close(chunkC)
		iterationStart := time.Now()

		err := db.reserve.IterateChunksItems(db.repo, storageRadius, func(chi reserve.ChunkItem) (bool, error) {
			select {
			case chunkC <- chi:
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
	sampleItemChan := make(chan SampleItem)
	const workers = 6

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			hasher := bmt.NewTrHasher(anchor)

			for chItem := range chunkC {
				getStart := time.Now()

				stat.GetDuration.Add(time.Since(getStart).Nanoseconds())

				// exclude chunks who's batches balance are below minimum
				if _, found := excludedBatchIDs[string(chItem.Chunk.Stamp().BatchID())]; found {
					stat.BelowBalanceIgnored.Inc()
					continue
				}

				// check if the timestamp on the postage stamp is not later than
				// the consensus time.
				if binary.BigEndian.Uint64(chItem.Chunk.Stamp().Timestamp()) > consensusTime {
					stat.NewIgnored.Inc()
					continue
				}

				hmacrStart := time.Now()

				taddr, err := transformedAddress(hasher, chItem.Chunk, chItem.Type)
				if err != nil {
					return err
				}

				stat.HmacrDuration.Add(time.Since(hmacrStart).Nanoseconds())

				select {
				case sampleItemChan <- SampleItem{
					TransformedAddress: taddr,
					ChunkAddress:       chItem.Chunk.Address(),
					ChunkData:          chItem.Chunk.Data(),
					Stamp:              newStamp(chItem.Chunk.Stamp()),
				}:
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

	sampleItems := make([]SampleItem, 0, sampleSize)
	// insert function will insert the new item in its correct place. If the sample
	// size goes beyond what we need we omit the last item.
	insert := func(item SampleItem) {
		added := false
		for i, sItem := range sampleItems {
			if le(item.TransformedAddress, sItem.TransformedAddress) {
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
		currentMaxAddr := swarm.EmptyAddress
		if len(sampleItems) > 0 {
			currentMaxAddr = sampleItems[len(sampleItems)-1].TransformedAddress
		}

		if le(item.TransformedAddress, currentMaxAddr) || len(sampleItems) < sampleSize {
			insert(item)

			// TODO: STAMP VALIDATION
		}
	}

	if err := g.Wait(); err != nil {
		return Sample{}, fmt.Errorf("sampler: failed creating sample: %w", err)
	}

	sample := Sample{
		Items: sampleItems,
	}

	db.logger.Info("reserve sampler done", "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", stat, "sample", sample)

	return sample, nil
}

func (db *DB) po(addr swarm.Address) uint8 {
	return swarm.Proximity(db.baseAddr.Bytes(), addr.Bytes())
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b swarm.Address) bool {
	return bytes.Compare(a.Bytes(), b.Bytes()) == -1
}

func transformedAddress(hasher *bmt.Hasher, chunk swarm.Chunk, chType swarm.ChunkType) (swarm.Address, error) {
	switch chType {
	case swarm.ChunkTypeContentAddressed:
		return transformedAddressCAC(hasher, chunk)
	case swarm.ChunkTypeSingleOwner:
		return transformedAddressSOC(hasher, chunk)
	default:
		return swarm.ZeroAddress, fmt.Errorf("chunk type [%v] is is not valid", chType)
	}
}

func transformedAddressCAC(hasher *bmt.Hasher, chunk swarm.Chunk) (swarm.Address, error) {
	hasher.Reset()
	hasher.SetHeader(chunk.Data()[:bmt.SpanSize])

	_, err := hasher.Write(chunk.Data()[bmt.SpanSize:])
	if err != nil {
		return swarm.ZeroAddress, err
	}

	taddr, err := hasher.Hash(nil)
	if err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(taddr), nil
}

func transformedAddressSOC(hasher *bmt.Hasher, chunk swarm.Chunk) (swarm.Address, error) {
	// Calculate transformed address from wrapped chunk
	sChunk, err := soc.FromChunk(chunk)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	taddrCac, err := transformedAddressCAC(hasher, sChunk.WrappedChunk())
	if err != nil {
		return swarm.ZeroAddress, err
	}

	// Hash address and transformed address to make transformed address for this SOC
	sHasher := swarm.NewHasher()
	if _, err := sHasher.Write(chunk.Address().Bytes()); err != nil {
		return swarm.ZeroAddress, err
	}
	if _, err := sHasher.Write(taddrCac.Bytes()); err != nil {
		return swarm.ZeroAddress, err
	}

	return swarm.NewAddress(sHasher.Sum(nil)), nil
}

type sampleStat struct {
	TotalIterated       atomic.Int64
	NotFound            atomic.Int64
	NewIgnored          atomic.Int64
	BelowBalanceIgnored atomic.Int64
	IterationDuration   atomic.Int64
	GetDuration         atomic.Int64
	HmacrDuration       atomic.Int64
	ValidStampDuration  atomic.Int64
}

func (s sampleStat) String() string {

	seconds := int64(time.Second)

	return fmt.Sprintf(
		"Chunks: %d NotFound: %d New Ignored: %d BelowBalanceIgnored: %d Iteration Duration: %d secs GetDuration: %d secs"+
			" HmacrDuration: %d secs ValidStampDuration: %d secs",
		s.TotalIterated.Load(),
		s.NotFound.Load(),
		s.NewIgnored.Load(),
		s.BelowBalanceIgnored.Load(),
		s.IterationDuration.Load()/seconds,
		s.GetDuration.Load()/seconds,
		s.HmacrDuration.Load()/seconds,
		s.ValidStampDuration.Load()/seconds,
	)
}
