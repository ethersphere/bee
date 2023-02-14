// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pullsync provides the pullsync protocol
// implementation.

package reserve

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

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

// TODO
func (r *Reserve) ReserveSample(
	ctx context.Context,
	store internal.Storage,
	anchor []byte,
	storageRadius uint8,
	consensusTime uint64,
) (Sample, error) {

	g, ctx := errgroup.WithContext(ctx)

	addrChan := make(chan swarm.Address)
	var stat sampleStat

	indexStore := store.IndexStore()
	chunkStore := store.ChunkStore()

	t := time.Now()

	// Phase 1: Iterate chunk addresses
	g.Go(func() error {
		defer close(addrChan)
		iterationStart := time.Now()

		err := indexStore.Iterate(storage.Query{
			Factory: func() storage.Item {
				return &chunkBinItem{Bin: storageRadius}
			},
			PrefixAtStart: true,
		}, func(res storage.Result) (bool, error) {

			item := res.Entry.(*chunkBinItem)

			select {
			case addrChan <- item.Address:
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

	r.logger.Info("sampler done", "duration", time.Since(t), "storage_radius", storageRadius, "consensus_time_ns", consensusTime, "stats", stat, "sample", sample)

	return sample, nil
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b []byte) bool {
	return bytes.Compare(a, b) == -1
}
