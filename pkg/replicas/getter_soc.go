// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package replicas

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas/combinator"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/semaphore"
)

// socGetter is the implementation of storage.Getter. This getter embeds the
// original simple chunk getter and extends it to a multiplexed variant that
// fetches chunks with replicas for SOC.
type socGetter struct {
	storage.Getter
	level redundancy.Level
}

// NewSocGetter is the getter constructor.
func NewSocGetter(g storage.Getter, level redundancy.Level) storage.Getter {
	return &socGetter{
		Getter: g,
		level:  min(level, maxRedundancyLevel),
	}
}

// Number of parallel replica get requests.
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

	// First try to get the original chunk.
	ch, err = g.Getter.Get(ctx, addr)
	if err != nil {
		errs = errors.Join(errs, fmt.Errorf("get chunk original address %v: %w", addr, err))
	} else {
		return ch, nil
	}

	// Try to retrieve replicas.
	// Context for cancellation of replica fetching.
	// Once a replica is found, this context is cancelled to stop further replica requests.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// sem is used to limit the number of concurrent replica fetch operations.
	sem := semaphore.NewWeighted(socGetterConcurrency)
	replicaIter := combinator.IterateReplicaAddresses(addr, int(g.level))

	// resultChan is used to send the first successfully fetched chunk back to the main goroutine.
	resultChan := make(chan swarm.Chunk, 1)
	// doneChan signals when all replica iteration and fetching attempts have concluded.
	doneChan := make(chan struct{})

	// This goroutine iterates through potential replica addresses and dispatches
	// concurrent fetch operations, respecting the concurrency limit.
	go func() {
		defer close(doneChan) // Ensure doneChan is closed when all replica attempts are finished.
		for replicaAddr := range replicaIter {
			select {
			case <-ctx.Done():
				// If the context is cancelled (e.g., a replica was found or parent context cancelled),
				// stop dispatching new replica requests.
				return
			default:
			}

			// Acquire a semaphore slot to limit concurrency.
			if err := sem.Acquire(ctx, 1); err != nil {
				// If context is cancelled while acquiring, stop.
				return
			}

			wg.Add(1)
			// Each replica fetch is performed in its own goroutine.
			go func(replicaAddr swarm.Address) {
				defer sem.Release(1) // Release the semaphore slot when done.
				defer wg.Done()      // Decrement the WaitGroup counter.

				ch, err := g.Getter.Get(ctx, replicaAddr)
				if err != nil {
					mu.Lock()
					errs = errors.Join(errs, fmt.Errorf("get chunk replica address %v: %w", replicaAddr, err))
					mu.Unlock()
					return
				}

				if !soc.Valid(swarm.NewChunk(addr, ch.Data())) {
					return
				}

				select {
				case resultChan <- ch:
					// If a chunk is successfully fetched and validated, send it to resultChan
					// and cancel the context to stop other in-flight replica fetches.
					cancel()
				case <-ctx.Done():
					// If the context is already cancelled, it means another goroutine found a chunk,
					// so this chunk is not needed.
				}
			}(replicaAddr)
		}
		wg.Wait() // Wait for all launched goroutines to complete.
	}()
	select {
	case ch := <-resultChan:
		return ch, nil
	case <-doneChan:
		if errs == nil {
			return nil, storage.ErrNotFound
		}
		return nil, errs
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
