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
	"time"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pullsync"
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

func (db *DB) reserveWorker(capacity int, syncer pullsync.SyncReporter, warmupDur, wakeUpDur time.Duration) {

	defer db.reserveWg.Done()

	threshold := capacity * 4 / 10

	overCapTrigger, overCapUnsub := db.events.Subscribe(reserveOverCapacity)
	defer overCapUnsub()

	select {
	case <-time.After(warmupDur):
	case <-db.quit:
		return
	}

	for {
		select {
		case <-overCapTrigger:
			_ = db.unreserve(context.Background())
		case <-time.After(wakeUpDur):
			radius := db.reserve.Radius()
			if db.reserve.Size() < threshold && syncer.Rate() == 0 && radius > 0 {
				_ = db.reserve.SetRadius(db.repo.IndexStore(), radius-1)
			}
		case <-db.quit:
			return
		}
	}
}

func (db *DB) po(addr swarm.Address) uint8 {
	return swarm.Proximity(db.baseAddr.Bytes(), addr.Bytes())
}

func (db *DB) ReservePutter(ctx context.Context) PutterSession {

	trx, commit, rollback := db.repo.NewTx(ctx)
	reservePutter := db.reserve.Putter(trx)

	pos := make(map[uint8]bool)
	count := 0

	// lock to avoid Puting a chunk that expires during the session.
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

func (db *DB) EvictBatch(ctx context.Context, batchID []byte) error {
	return db.evictBatch(ctx, batchID, swarm.MaxBins)
}

func (db *DB) evictBatch(ctx context.Context, batchID []byte, bin uint8) error {

	db.lock.Lock(reserveLock)
	defer db.lock.Unlock(reserveLock)

	for b := uint8(0); b < bin; b++ {

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

		err := db.bs.Iterate(func(b *postage.Batch) (bool, error) {

			select {
			case <-db.quit:
				return false, errDBQuit
			default:
			}

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

func (db *DB) ReserveLastBinIDs() ([]uint64, error) {
	return db.reserve.LastBinIDs(db.repo.IndexStore())
}

func (db *DB) SubscribeBin(ctx context.Context, bin uint8, start, end uint64) (<-chan *BinC, <-chan error) {

	out := make(chan *BinC)
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
			case <-db.quit:
				errC <- errDBQuit
				return
			case <-ctx.Done():
				errC <- err
				return
			}
		}
	}()

	return out, errC
}

type Sample struct {
	Items []swarm.Address
	Hash  swarm.Address
}

func (db *DB) ReserveSample(
	ctx context.Context,
	anchor []byte,
	storageRadius uint8,
	consensusTime uint64,
) (Sample, error) {

	g, ctx := errgroup.WithContext(ctx)

	addrChan := make(chan swarm.Address)
	var stat sampleStat

	indexStore := db.repo.IndexStore()
	chunkStore := db.repo.ChunkStore()

	t := time.Now()

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		defer close(addrChan)
		iterationStart := time.Now()

		err := db.reserve.Iterate(indexStore, storageRadius, func(a swarm.Address, u uint64) (bool, error) {
			select {
			case addrChan <- a:
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

			for addr := range addrChan {
				getStart := time.Now()

				ch, err := chunkStore.Get(ctx, addr)
				if err != nil {
					stat.NotFound.Inc()
					continue
				}
				stat.GetDuration.Add(time.Since(getStart).Nanoseconds())

				// check if the timestamp on the postage stamp is not later than
				// the consensus time.
				if binary.BigEndian.Uint64(ch.Stamp().Timestamp()) > consensusTime {
					stat.NewIgnored.Inc()
					continue
				}

				hmacrStart := time.Now()
				_, err = hmacr.Write(ch.Data())
				if err != nil {
					return err
				}
				taddr := hmacr.Sum(nil)
				hmacr.Reset()
				stat.HmacrDuration.Add(time.Since(hmacrStart).Nanoseconds())

				select {
				case sampleItemChan <- sampleEntry{transformedAddress: swarm.NewAddress(taddr), chunk: ch}:
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
