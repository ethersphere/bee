// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/events"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func (db *DB) Reserve() *reserve.Reserve {
	return db.reserve
}

func (db *DB) Events() *events.Subscriber {
	return db.events
}

func ReplaceSharkyShardLimit(val int) {
	sharkyNoOfShards = val
}

func (db *DB) WaitForBgCacheWorkers() (unblock func()) {
	for range defaultBgCacheWorkers {
		db.cacheLimiter.sem <- struct{}{}
	}
	return func() {
		for range defaultBgCacheWorkers {
			<-db.cacheLimiter.sem
		}
	}
}

func DefaultOptions() *Options {
	return defaultOptions()
}

// TransformedAddress exposes the sampler's per-chunk hashing so it can be
// benchmarked on its own.
//
// The exported signature takes a swarm.Chunk and is deliberately held stable
// even where transformedAddress itself does not, so that the same benchmark
// source can be run against branches that shape the internal function
// differently. Only this shim changes between them.
func TransformedAddress(hasher bmt.Hasher, ch swarm.Chunk, chType swarm.ChunkType) (swarm.Address, error) {
	return transformedAddress(hasher, ch.Address(), ch.Data(), chType)
}

// ReserveSampleAfterViewOpened runs ReserveSample and calls fn after it opens
// its sampling view and before it starts reading chunks.
func (db *DB) ReserveSampleAfterViewOpened(ctx context.Context, anchor []byte, depth uint8, consensusTime uint64, fn func()) (Sample, error) {
	open := func(ctx context.Context, anchor []byte, depth uint8) (*transaction.SamplingView, error) {
		view, err := db.storage.NewSamplingView(ctx, anchor, depth)
		if err == nil {
			fn()
		}
		return view, err
	}
	return db.reserveSample(ctx, anchor, depth, consensusTime, nil, open)
}
