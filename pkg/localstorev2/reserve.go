// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/postage"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

const (
	reserveOverCapacity = "reserveOverCapacity"
	reserveUnreserved   = "reserveUnreserved"
	reserveLock         = "reserveLock"
)

type SyncReporter interface {
	// Number of active historical syncing jobs.
	Rate() float64
}

func (db *DB) reserveWorker(capacity int, syncer SyncReporter, warmupDur, wakeUpDur time.Duration) {

	defer db.reserveWg.Done()

	threshold := capacity * 4 / 10

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

	for {
		select {
		case <-overCapTrigger:
			err := db.unreserve(ctx)
			db.logger.Error(err, "reserve unreserve process")
		case <-time.After(wakeUpDur):
			radius := db.reserve.Radius()
			if db.reserve.Size() < threshold && syncer.Rate() == 0 && radius > 0 {
				err := db.reserve.SetRadius(db.repo.IndexStore(), radius-1)
				db.logger.Error(err, "reserve set radius")
			}
		case <-db.quit:
			return
		}
	}
}

func (db *DB) ReserveGet(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	return db.reserve.Get(ctx, db.repo, addr)
}

func (db *DB) ReserveHas(addr swarm.Address) (bool, error) {
	return db.reserve.Has(db.repo.IndexStore(), addr)
}

// ReservePutter returns a PutterSession for inserting chunks into the reserve.
func (db *DB) ReservePutter(ctx context.Context) PutterSession {

	trx, commit, rollback := db.repo.NewTx(ctx)
	reservePutter := db.reserve.Putter(trx)

	pos := make(map[uint8]bool)
	count := 0

	// lock to avoid Putting a chunk that expires during the session.
	db.lock.Lock(reserveLock)

	return &putterSession{
		Putter: storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
			err := reservePutter.Put(ctx, chunk)
			if err != nil {
				return err
			}
			pos[db.po(chunk.Address())] = true
			count++
			return nil
		}),
		done: func(swarm.Address) error {
			defer db.lock.Unlock(reserveLock)
			err := commit()
			if err != nil {
				return err
			}
			db.reserve.AddSize(count)
			for po := range pos {
				db.reserveBinEvents.Trigger(string(po))
			}
			if !db.reserve.IsWithinCapacity() {
				db.events.Trigger(reserveOverCapacity)
			}
			return nil
		},
		cleanup: func() error {
			defer db.lock.Unlock(reserveLock)
			return rollback()
		},
	}
}

// EvictBatch evicts all chunks belonging to a batch from the reserve.
func (db *DB) EvictBatch(ctx context.Context, batchID []byte) error {
	return db.evictBatch(ctx, batchID, swarm.MaxBins)
}

func (db *DB) evictBatch(ctx context.Context, batchID []byte, bin uint8) error {

	db.lock.Lock(reserveLock)
	defer db.lock.Unlock(reserveLock)

	for b := uint8(0); b < bin; b++ {

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		txnRepo, commit, rollback := db.repo.NewTx(ctx)

		evicted, err := db.reserve.EvictBatchBin(txnRepo, batchID, b)
		if err != nil {
			_ = rollback()
			return err
		}

		err = commit()
		if err != nil {
			return err
		}

		db.reserve.AddSize(-evicted)
	}

	return nil
}

func (db *DB) unreserve(ctx context.Context) error {

	withinCap := false
	radius := db.reserve.Radius()
	defer db.events.Trigger(reserveUnreserved)

	for {

		err := db.batchstore.Iterate(func(b *postage.Batch) (bool, error) {

			err := db.evictBatch(ctx, b.ID, radius)
			if err != nil {
				return false, err
			}

			if db.reserve.IsWithinCapacity() {
				withinCap = true
				return true, nil
			}

			return false, nil
		})
		if err != nil {
			return err
		}
		if withinCap {
			return nil
		}

		radius++
		_ = db.reserve.SetRadius(db.repo.IndexStore(), radius)
	}
}

// ReserveLastBinIDs returns all of the highest binIDs from all the bins in the reserve.
func (db *DB) ReserveLastBinIDs() ([]uint64, error) {
	return db.reserve.LastBinIDs(db.repo.IndexStore())
}

// BinC is the result returned from the SubscribeBin channel that contains the chunk address and the binID
type BinC struct {
	Address swarm.Address
	BinID   uint64
}

// SubscribeBin returns a channel that feeds all the chunks in the reserve from a certain bin between a start and end binIDs.
func (db *DB) SubscribeBin(ctx context.Context, bin uint8, start, end uint64) (<-chan *BinC, func(), <-chan error) {

	out := make(chan *BinC)
	done := make(chan struct{})
	errC := make(chan error, 1)

	db.reserveWg.Add(1)
	go func() {

		trigger, unsub := db.reserveBinEvents.Subscribe(string(bin))
		defer unsub()
		defer close(out)
		defer db.reserveWg.Done()

		var (
			stop      = false
			lastBinID uint64
			startID   = start
		)

		for {

			err := db.reserve.IterateBin(db.repo.IndexStore(), bin, startID, func(a swarm.Address, binID uint64) (bool, error) {

				if binID <= end {
					lastBinID = binID
					select {
					case out <- &BinC{Address: a, BinID: binID}:
					case <-done:
						return false, nil
					case <-db.quit:
						return false, errDBQuit
					case <-ctx.Done():
						return false, ctx.Err()
					}

				}

				if binID >= end {
					stop = true
					return true, nil
				}

				return false, nil

			})
			if err != nil {
				errC <- err
				return
			}

			if stop {
				return
			}

			startID = lastBinID + 1

			select {
			case <-trigger:
			case <-done:
				return
			case <-db.quit:
				errC <- errDBQuit
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

		err := db.reserve.IterateChunks(db.repo, storageRadius, func(a swarm.Chunk, u uint64) (bool, error) {
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

	db.logger.Info("sampler done", "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", stat, "sample", sample)

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
