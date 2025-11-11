// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// the code below implements the integration of dispersed replicas in chunk fetching.
// using storage.Getter interface.
package replicas

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// socGetter is the private implementation of storage.Getter, an interface for
// retrieving chunks. This getter embeds the original simple chunk getter and extends it
// to a multiplexed variant that fetches chunks with replicas for SOC.
//
// the strategy to retrieve a chunk that has replicas can be configured with a few parameters:
//   - RetryInterval: the delay before a new batch of replicas is fetched.
//   - depth: 2^{depth} is the total number of additional replicas that have been uploaded
//     (by default, it is assumed to be 4, ie. total of 16)
//   - (not implemented) pivot: replicas with address in the proximity of pivot will be tried first
type socGetter struct {
	storage.Getter
	level redundancy.Level
}

// NewSocGetter is the getter constructor
func NewSocGetter(g storage.Getter, level redundancy.Level) storage.Getter {
	return &socGetter{Getter: g, level: level}
}

// Get makes the socGetter satisfy the storage.Getter interface
// It attempts to fetch the chunk by its original address first.
// If the original address does not return a result within RetryInterval,
// it starts dispatching exponentially growing batches of replica requests
// at each RetryInterval until a chunk is found or all replicas are tried.
func (g *socGetter) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	// try original address first
	ch, err = g.Getter.Get(ctx, addr)
	if err == nil {
		return ch, nil
	}

	var errs error
	errs = errors.Join(errs, err)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	replicas := NewSocReplicator(addr, g.level).Replicas()

	resultC := make(chan swarm.Chunk)
	errc := make(chan error, len(replicas))

	worker := func(chunkAddr swarm.Address) {
		defer wg.Done()

		ch, err := g.Getter.Get(ctx, chunkAddr)
		if err != nil {
			errc <- err
			return
		}

		select {
		case resultC <- ch:
		case <-ctx.Done():
		}
	}

	go func() {
		replicaIndex := 0
		batchLevel := uint8(1) // start with 1 (batch size 1 << 1 = 2)

		dispatchBatch := func() (done bool) {
			batchSize := 1 << batchLevel // 2, 4, 8...
			sentInBatch := 0

			for sentInBatch < batchSize && replicaIndex < len(replicas) {
				addr := replicas[replicaIndex]
				replicaIndex++
				sentInBatch++

				wg.Add(1)
				go worker(addr)
			}

			batchLevel++

			// we are done if all replicas are sent
			return replicaIndex >= len(replicas)
		}

		if done := dispatchBatch(); done {
			return
		}

		timer := time.NewTimer(RetryInterval)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				if done := dispatchBatch(); done {
					return
				}
				timer.Reset(RetryInterval)

			case <-ctx.Done():
				return
			}
		}
	}()

	// collect results
	waitC := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitC)
	}()

	for {
		select {
		case chunk := <-resultC:
			cancel() // cancel the context to stop all other workers.
			return chunk, nil

		case err := <-errc:
			errs = errors.Join(errs, err)

		case <-waitC:
			return nil, errors.Join(ErrSwarmageddon, errs)
		}
	}
}
