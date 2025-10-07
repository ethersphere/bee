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

// getter is the private implementation of storage.Getter, an interface for
// retrieving chunks. This getter embeds the original simple chunk getter and extends it
// to a multiplexed variant that fetches chunks with replicas for SOC.
//
// the strategy to retrieve a chunk that has replicas can be configured with a few parameters:
//   - SOCRetryInterval: the delay before a new batch of replicas is fetched.
//   - depth: 2^{depth} is the total number of additional replicas that have been uploaded
//     (by default, it is assumed to be 4, ie. total of 16)
//   - (not implemented) pivot: replicas with address in the proximity of pivot will be tried first
type socGetter struct {
	storage.Getter
	level redundancy.Level
}

var SOCRetryInterval = 300 * time.Millisecond

// NewSocGetter is the getter constructor
func NewSocGetter(g storage.Getter, level redundancy.Level) storage.Getter {
	return &socGetter{Getter: g, level: level}
}

// Get makes the socGetter satisfy the storage.Getter interface
func (g *socGetter) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()

	// channel that the results (retrieved chunks) are gathered to from concurrent
	// workers each fetching a replica
	resultC := make(chan swarm.Chunk)
	// errc collects the errors
	errc := make(chan error, g.level.GetReplicaCount()+1)
	var errs error
	errcnt := 0

	// concurrently call to retrieve chunk using original SOC address
	wg.Go(func() {
		ch, err := g.Getter.Get(ctx, addr)
		if err != nil {
			errc <- err
			return
		}

		select {
		case resultC <- ch:
		case <-ctx.Done():
		}
	})
	// counters
	n := 0      // counts the replica addresses tried
	target := 2 // the number of replicas attempted to download in this batch
	total := g.level.GetReplicaCount()

	//
	rr := newSocReplicator(addr, g.level)
	next := rr.c
	var wait <-chan time.Time // nil channel to disable case
	// addresses used are doubling each period of search expansion
	// (at intervals of RetryInterval)
	for level := uint8(0); level <= uint8(g.level); {
		select {
		// at least one chunk is retrieved, cancel the rest and return early
		case chunk := <-resultC:
			cancel()
			return chunk, nil

		case err = <-errc:
			errs = errors.Join(errs, err)
			errcnt++
			if errcnt > total {
				return nil, errors.Join(ErrSwarmageddon, errs)
			}

			// ticker switches on the address channel
		case <-wait:
			next = rr.c
			level++
			target = 1 << level
			n = 0
			continue

			// getting the addresses in order
		case so := <-next:
			if so == nil {
				next = nil
				continue
			}

			wg.Go(func() {
				ch, err := g.Getter.Get(ctx, swarm.NewAddress(so.addr))
				if err != nil {
					errc <- err
					return
				}

				select {
				case resultC <- ch:
				case <-ctx.Done():
				}
			})
			n++
			if n < target {
				continue
			}
			next = nil
			wait = time.After(SOCRetryInterval)
		}
	}

	return nil, nil
}
