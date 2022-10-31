// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

const sampleSize = 8

var errDbClosed = errors.New("database closed")
var errSamplerStopped = errors.New("sampler stopped due to ongoing evictions")
var errSamplerStoppedI = errors.New("sampler stopped due to batchStore iteration error")

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
	chunkItem          shed.Item
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
	consensusTime uint64, // nanoseconds
	minimumBalance *big.Int,
) (storage.Sample, error) {

	g, ctx := errgroup.WithContext(ctx)
	addrChan := make(chan swarm.Address)
	var stat sampleStat
	logger := db.logger.WithName("sampler").V(1).Register()

	t := time.Now()
	excludedBatchIDs, err := db.batchStore.GetBatchIDsExpiringUntil(minimumBalance)
	if err != nil {
		logger.Error(err, "error getting minimum balance based excluded batchIDs")
		return storage.Sample{}, fmt.Errorf("sampler: failed creating sample: %w", errSamplerStoppedI)
	}

	// signal start of sampling to see if we get any evictions during the sampler
	// run
	db.startSampling()
	defer db.resetSamplingState()

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		defer close(addrChan)
		iterationStart := time.Now()

		err := db.pullIndex.Iterate(func(item shed.Item) (bool, error) {
			select {
			case addrChan <- swarm.NewAddress(item.Address):
				stat.TotalIterated.Inc()
				return false, nil
			case <-ctx.Done():
				return true, ctx.Err()
			case <-db.close:
				return true, errDbClosed
			}
		}, &shed.IterateOptions{
			StartFrom: &shed.Item{
				Address: db.addressInBin(storageRadius).Bytes(),
			},
		})
		if err != nil {
			logger.Error(err, "sampler: failed iteration")
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
				chItem, err := db.get(ctx, storage.ModeGetSync, addr)
				stat.GetDuration.Add(time.Since(getStart).Nanoseconds())
				if err != nil {
					stat.NotFound.Inc()
					continue
				}

				// check if the timestamp on the postage stamp is not later than
				// the consensus time.
				if binary.BigEndian.Uint64(chItem.Timestamp) > consensusTime {
					stat.NewIgnored.Inc()
					continue
				}

				hmacrStart := time.Now()
				_, err = hmacr.Write(chItem.Data)
				if err != nil {
					return err
				}
				taddr := hmacr.Sum(nil)
				hmacr.Reset()
				stat.HmacrDuration.Add(time.Since(hmacrStart).Nanoseconds())

				select {
				case sampleItemChan <- sampleEntry{transformedAddress: swarm.NewAddress(taddr), chunkItem: chItem}:
					// continue
				case <-ctx.Done():
					return ctx.Err()
				case <-db.close:
					return errDbClosed
				case <-db.samplerSignal:
					return errSamplerStopped
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

			validStart := time.Now()

			chunk := swarm.NewChunk(swarm.NewAddress(item.chunkItem.Address), item.chunkItem.Data)

			if _, found := excludedBatchIDs[string(item.chunkItem.BatchID)]; found {
				logger.Debug("excluded chunk with batch balance below minimum", "chunk_address", chunk.Address())
				continue
			}

			stamp := postage.NewStamp(
				item.chunkItem.BatchID,
				item.chunkItem.Index,
				item.chunkItem.Timestamp,
				item.chunkItem.Sig,
			)

			stampData, err := stamp.MarshalBinary()
			if err != nil {
				logger.Debug("error marshaling stamp for chunk", "chunk_address", chunk.Address(), "error", err)
				continue
			}
			_, err = db.validStamp(chunk, stampData)
			if err == nil {
				if !validChunkFn(chunk) {
					logger.Debug("data invalid for chunk address", "chunk_address", chunk.Address())
				} else {
					insert(item.transformedAddress)
				}
			} else {
				logger.Debug("invalid stamp for chunk", "chunk_address", chunk.Address(), "error", err)
			}

			stat.ValidStampDuration.Add(time.Since(validStart).Nanoseconds())
		}
	}

	if err := g.Wait(); err != nil {
		db.metrics.SamplerFailedRuns.Inc()
		if errors.Is(err, errSamplerStopped) {
			db.metrics.SamplerStopped.Inc()
		}
		return storage.Sample{}, fmt.Errorf("sampler: failed creating sample: %w", err)
	}

	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	for _, s := range sampleItems {
		_, err := hasher.Write(s.Bytes())
		if err != nil {
			db.metrics.SamplerFailedRuns.Inc()
			return storage.Sample{}, fmt.Errorf("sampler: failed creating root hash of sample: %w", err)
		}
	}
	hash := hasher.Sum(nil)

	sample := storage.Sample{
		Items: sampleItems,
		Hash:  swarm.NewAddress(hash),
	}

	db.metrics.SamplerSuccessfulRuns.Inc()
	logger.Info("sampler done", "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", stat, "sample", sample)

	return sample, nil
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b []byte) bool {
	return bytes.Compare(a, b) == -1
}

func (db *DB) startSampling() {
	db.lock.Lock(lockKeySampling)
	defer db.lock.Unlock(lockKeySampling)

	db.samplerStop = new(sync.Once)
	db.samplerSignal = make(chan struct{})
}

func (db *DB) stopSamplingIfRunning() {
	db.lock.Lock(lockKeySampling)
	defer db.lock.Unlock(lockKeySampling)

	if db.samplerStop != nil {
		db.samplerStop.Do(func() { close(db.samplerSignal) })
	}
}

func (db *DB) resetSamplingState() {
	db.lock.Lock(lockKeySampling)
	defer db.lock.Unlock(lockKeySampling)

	db.samplerStop = nil
	db.samplerSignal = nil
}

var validChunkFn func(swarm.Chunk) bool

func validChunk(ch swarm.Chunk) bool {
	if !cac.Valid(ch) && !soc.Valid(ch) {
		return false
	}
	return true
}
