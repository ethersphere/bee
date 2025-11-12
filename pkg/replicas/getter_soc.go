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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := semaphore.NewWeighted(socGetterConcurrency)
	replicaIter := combinator.IterateReplicaAddresses(addr, int(g.level))

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

				select {
				case resultChan <- ch:
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
			return nil, storage.ErrNotFound
		}
		return nil, errs
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
