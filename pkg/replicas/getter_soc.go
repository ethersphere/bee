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

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas/combinator"
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
	var errs error

	for replicaAddr := range combinator.IterateAddressCombinations(addr, int(g.level)) {
		ctx, cancel := context.WithTimeout(ctx, RetryInterval)

		// Download the replica.
		ch, err := g.Getter.Get(ctx, replicaAddr)
		if err != nil {
			cancel()
			errs = errors.Join(errs, fmt.Errorf("get chunk replica address %v: %w", replicaAddr, err))
			continue
		}
		cancel()

		// Construct the original chunk with the original address.
		originalChunk := swarm.NewChunk(addr, ch.Data())

		// Validate that the data of the chunk is correct against the original address.
		isValid := cac.Valid(originalChunk)
		if !isValid {
			errs = errors.Join(errs, fmt.Errorf("validate data at replica address %v: %w", replicaAddr, swarm.ErrInvalidChunk))
			continue
		}

		return originalChunk, nil
	}

	return nil, errors.Join(errs, ErrSwarmageddon)
}
