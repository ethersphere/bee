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
	"github.com/ethersphere/bee/v2/pkg/safe"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ErrSwarmageddon is returned in case of a vis mayor called Swarmageddon.
// Swarmageddon is the situation when none of the replicas can be retrieved.
// If 2^{depth} replicas were uploaded and they all have valid postage stamps
// then the probability of Swarmageddon is less than 0.000001
// assuming the error rate of chunk retrievals stays below the level expressed
// as depth by the publisher.
var (
	ErrSwarmageddon = errors.New("swarmageddon has begun")
	// errGetterExhausted is returned when the retry loop exhausts all levels without
	// receiving a result or enough errors to trigger ErrSwarmageddon.
	// This path should never be reached under normal operation.
	errGetterExhausted = errors.New("replicas getter: exhausted all levels without result (unexpected)")
)

// getter is the private implementation of storage.Getter, an interface for
// retrieving chunks. This getter embeds the original simple chunk getter and extends it
// to a multiplexed variant that fetches chunks with replicas.
//
// the strategy to retrieve a chunk that has replicas can be configured with a few parameters:
//   - RetryInterval: the delay before a new batch of replicas is fetched.
//   - depth: 2^{depth} is the total number of additional replicas that have been uploaded
//     (by default, it is assumed to be 4, ie. total of 16)
//   - (not implemented) pivot: replicas with address in the proximity of pivot will be tried first
type getter struct {
	wg sync.WaitGroup
	storage.Getter
	level redundancy.Level
}

// NewGetter is the getter constructor
func NewGetter(g storage.Getter, level redundancy.Level) storage.Getter {
	return &getter{Getter: g, level: level}
}

// Get makes the getter satisfy the storage.Getter interface
func (g *getter) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	if g.level == redundancy.NONE {
		return g.Getter.Get(ctx, addr)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// channel that the results (retrieved chunks) are gathered to from concurrent
	// workers each fetching a replica
	resultC := make(chan swarm.Chunk)
	// errc collects the errors
	errc := make(chan error, 17)
	var errs error
	errcnt := 0

	// concurrently call to retrieve chunk using original CAC address
	g.wg.Go(func() {
		err := safe.RunFunc(nil, "replicas-get-original", func() error {
			ch, err := g.Getter.Get(ctx, addr)
			if err != nil {
				return err
			}

			select {
			case resultC <- ch:
			case <-ctx.Done():
			}
			return nil
		})()
		if err != nil {
			errc <- err
		}
	})
	// counters
	n := 0      // counts the replica addresses tried
	target := 2 // the number of replicas attempted to download in this batch
	total := g.level.GetReplicaCount()

	// The replicator feeds replica addresses in batches that double each RetryInterval
	// (2, 2, 4, 8, 16 for PARANOID). The replicator exhausts exactly at the last iteration,
	// which then drains a nil from `next` and becomes an indefinite wait on resultC/errc.
	// The loop therefore always terminates via resultC (success) or Swarmageddon (all fail),
	// never by falling through to the return below.
	rr := newReplicator(addr, g.level)
	next := rr.c
	var wait <-chan time.Time // nil channel to disable case
	// addresses used are doubling each period of search expansion
	// (at intervals of RetryInterval)
	ticker := time.NewTicker(RetryInterval)
	defer ticker.Stop()
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
			wait = nil
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

			g.wg.Go(func() {
				err := safe.RunFunc(nil, "replicas-get-replica", func() error {
					ch, err := g.Getter.Get(ctx, swarm.NewAddress(so.addr))
					if err != nil {
						return err
					}

					soc, err := soc.FromChunk(ch)
					if err != nil {
						return err
					}

					select {
					case resultC <- soc.WrappedChunk():
					case <-ctx.Done():
					}
					return nil
				})()
				if err != nil {
					errc <- err
				}
			})
			n++
			if n < target {
				continue
			}
			next = nil
			wait = ticker.C
		}
	}

	// unreachable: the loop always exits via resultC or Swarmageddon (see comment above)
	return nil, errGetterExhausted
}
