// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/reserve"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pullsync"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	reserveOverCapacity = "reserveOverCapacity"
	reserveUnreserved   = "reserveUnreserved"
)

func (db *DB) reserveWorker(capacity int, syncer pullsync.SyncReporter, warmupDur, wakeUpDur time.Duration) {

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
			fmt.Println("overCapTrigger")
			_ = db.unreserve(context.Background())
		case <-time.After(wakeUpDur):
			if db.reserve.Size() < threshold && syncer.Rate() == 0 && db.reserve.Radius() > 0 {
				_ = db.reserve.SetRadius(db.repo.IndexStore(), db.reserve.Radius()-1)
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
			err := commit()
			if err != nil {
				return err
			}
			db.reserve.AddSize(count)
			for po := range pos {
				db.reserveBinEvents.Trigger(string(po))
			}
			if !db.reserve.IsWithinCapacity() {
				fmt.Println("over capacity")
				db.events.Trigger(reserveOverCapacity)
			}
			return nil
		},
		cleanup: func() error {
			return rollback()
		},
	}
}

func (db *DB) EvictBatch(ctx context.Context, batchID []byte) error {
	return db.evictBatch(ctx, batchID, swarm.MaxBins)
}

func (db *DB) evictBatch(ctx context.Context, batchID []byte, bin uint8) error {

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
			fmt.Println(hex.EncodeToString(b.ID))
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

func (db *DB) SubscribeBin(ctx context.Context, bin uint8, start, end uint64) (<-chan *BinC, <-chan error) {

	out := make(chan *BinC)
	errC := make(chan error, 1)

	go func() {
		trigger, unsub := db.reserveBinEvents.Subscribe(string(bin))
		defer unsub()
		defer close(out)

		var (
			stop      = false
			lastBinID uint64
			startID   = start
		)

		for {

			err := db.reserve.IterateBin(db.repo.IndexStore(), bin, startID, func(a swarm.Address, binID uint64) (bool, error) {

				fmt.Println("-", bin, binID, a, startID)

				if binID <= end {
					lastBinID = binID
					select {
					case out <- &BinC{Address: a, BinID: binID}:
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
			case <-ctx.Done():
				errC <- err
				return
			}
		}
	}()

	return out, errC
}

func (db *DB) ReserveSample(
	ctx context.Context,
	anchor []byte,
	storageRadius uint8,
	consensusTime uint64,
) (reserve.Sample, error) {

	sample, err := db.reserve.ReserveSample(ctx, db.repo, anchor, storageRadius, consensusTime)
	if err != nil {
		return reserve.Sample{}, err
	}

	return sample, nil
}
