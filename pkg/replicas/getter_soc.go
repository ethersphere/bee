// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// the code below implements the integration of dispersed replicas in chunk fetching.
// using storage.Getter interface.
package replicas

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas/combinator"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/semaphore"
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

const socGetterConcurrency = 4

// Get makes the socGetter satisfy the storage.Getter interface
// It attempts to fetch the chunk by its original address first.
// If the original address does not return a result,
// it starts dispatching parallel requests for replicas
// until a chunk is found or all replicas are tried.
func (g *socGetter) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	var (
		errs error
		mu   sync.Mutex
		wg   sync.WaitGroup
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := semaphore.NewWeighted(socGetterConcurrency)
	replicaIter := combinator.IterateAddressCombinations(addr, int(g.level))

	resultChan := make(chan swarm.Chunk, 1)
	doneChan := make(chan struct{})

	go func() {
		defer close(doneChan)
		for replicaAddr := range replicaIter {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if err := sem.Acquire(ctx, 1); err != nil {
				return
			}

			wg.Add(1)
			go func(replicaAddr swarm.Address) {
				defer sem.Release(1)
				defer wg.Done()

				ch, err := g.Getter.Get(ctx, replicaAddr)
				if err != nil {
					mu.Lock()
					errs = errors.Join(errs, fmt.Errorf("get chunk replica address %v: %w", replicaAddr, err))
					mu.Unlock()
					return
				}

				originalChunk := swarm.NewChunk(addr, ch.Data())
				if !cac.Valid(originalChunk) {
					mu.Lock()
					errs = errors.Join(errs, fmt.Errorf("validate data at replica address %v: %w", replicaAddr, swarm.ErrInvalidChunk))
					mu.Unlock()
					return
				}

				select {
				case resultChan <- originalChunk:
					cancel()
				case <-ctx.Done():
				}
			}(replicaAddr)
		}
		wg.Wait()
	}()

	select {
	case ch := <-resultChan:
		return ch, nil
	case <-doneChan:
		if errs == nil {
			return nil, ErrSwarmageddon
		}
		return nil, errors.Join(errs, ErrSwarmageddon)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
