// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package getter

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// RecoveryFactory is a function that creates a recovery decoder on demand
type RecoveryFactory func() storage.Getter

// ReDecoder is a wrapper around a Getter that first attempts to fetch a chunk directly
// from the network, and only falls back to recovery if the network fetch fails.
// This is used to handle cases where previously recovered chunks have been evicted from cache.
type ReDecoder struct {
	fetcher         storage.Getter    // Direct fetcher (usually netstore)
	recoveryFactory RecoveryFactory   // Factory function to create recovery decoder on demand
	logger          log.Logger
}

// NewReDecoder creates a new ReDecoder instance with the provided fetcher and recovery factory.
// The recovery decoder will only be created if needed (when network fetch fails).
func NewReDecoder(fetcher storage.Getter, recoveryFactory RecoveryFactory, logger log.Logger) *ReDecoder {
	return &ReDecoder{
		fetcher:         fetcher,
		recoveryFactory: recoveryFactory,
		logger:          logger,
	}
}

// Get implements the storage.Getter interface.
// It first attempts to fetch the chunk directly from the network.
// If that fails with ErrNotFound, it then creates the recovery decoder and attempts to recover the chunk.
func (rd *ReDecoder) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	// First try to get the chunk directly from the network
	chunk, err := rd.fetcher.Get(ctx, addr)
	if err == nil {
		return chunk, nil
	}

	// Only attempt recovery if the chunk was not found
	if err != storage.ErrNotFound {
		return nil, err
	}

	// Log that we're falling back to recovery
	rd.logger.Debug("chunk not found in network, creating recovery decoder", "address", addr)

	// Create the recovery decoder on demand
	recovery := rd.recoveryFactory()

	// Attempt to recover the chunk
	return recovery.Get(ctx, addr)
}
