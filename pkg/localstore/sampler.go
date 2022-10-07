// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
)

const sampleSize = 16

var errDbClosed = errors.New("database closed")

type sampleStat struct {
	TotalIterated     atomic.Int64
	NotFound          atomic.Int64
	NewIgnored        atomic.Int64
	IterationDuration atomic.Int64
	GetDuration       atomic.Int64
	HmacrDuration     atomic.Int64
}

func (s *sampleStat) String() string {
	return fmt.Sprintf(
		"Total: %d NotFound: %d New Ignored: %d Iteration Duration: %d secs GetDuration: %d secs HmacrDuration: %d",
		s.TotalIterated.Load(),
		s.NotFound.Load(),
		s.NewIgnored.Load(),
		s.IterationDuration.Load()/1000000,
		s.GetDuration.Load()/1000000,
		s.HmacrDuration.Load()/1000000,
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
	storageDepth uint8,
	consensusTime int64,
) (storage.Sample, error) {

	g, ctx := errgroup.WithContext(ctx)
	addrChan := make(chan swarm.Address)
	var stat sampleStat
	logger := db.logger.WithName("sampler").V(1).Register()

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
				Address: generateAddressAt(db.baseKey, int(storageDepth)),
			},
		})
		if err != nil {
			logger.Error(err, "sampler: failed iteration")
			return err
		}
		stat.IterationDuration.Add(time.Since(iterationStart).Microseconds())
		return nil
	})

	// Phase 2: Get the chunk data and calculate transformed hash
	sampleItemChan := make(chan swarm.Address)
	const workers = 6
	for i := 0; i < workers; i++ {
		g.Go(func() error {
			hmacr := hmac.New(swarm.NewHasher, anchor)

			for addr := range addrChan {
				getStart := time.Now()
				chItem, err := db.get(ctx, storage.ModeGetSync, addr)
				stat.GetDuration.Add(time.Since(getStart).Microseconds())
				if err != nil {
					stat.NotFound.Inc()
					continue
				}

				if chItem.StoreTimestamp > consensusTime {
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
				stat.HmacrDuration.Add(time.Since(hmacrStart).Microseconds())

				select {
				case sampleItemChan <- swarm.NewAddress(taddr):
					// continue
				case <-ctx.Done():
					return ctx.Err()
				case <-db.close:
					return errDbClosed
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
		if le(item.Bytes(), currentMaxAddr.Bytes()) || len(sampleItems) < sampleSize {
			insert(item)
		}
	}

	if err := g.Wait(); err != nil {
		return storage.Sample{}, fmt.Errorf("sampler: failed creating sample: %w", err)
	}

	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	for _, s := range sampleItems {
		_, err := hasher.Write(s.Bytes())
		if err != nil {
			return storage.Sample{}, fmt.Errorf("sampler: failed creating root hash of sample: %w", err)
		}
	}
	hash := hasher.Sum(nil)

	sample := storage.Sample{
		Items: sampleItems,
		Hash:  swarm.NewAddress(hash),
	}
	logger.Info("Sampler done", "Stats", stat.String(), "Sample", sample)

	return sample, nil
}

// less function uses the byte compare to check for lexicographic ordering
func le(a, b []byte) bool {
	return bytes.Compare(a, b) == -1
}
