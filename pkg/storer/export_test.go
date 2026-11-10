// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/events"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	dto "github.com/prometheus/client_model/go"
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

const (
	DirectUploadRequeueLimit  = directUploadRequeueLimit
	DirectUploadRequeueMinAge = directUploadRequeueMinAge
)

// Requeue exposes requeue to tests with chunks only.
type Requeue struct{ q *requeue }

func NewRequeue(limit int, minAge time.Duration) *Requeue {
	return &Requeue{q: newRequeue(limit, minAge)}
}

func (r *Requeue) Add(ch swarm.Chunk, now time.Time) (swarm.Chunk, time.Duration, bool) {
	oldest, wait, full := r.q.add(requeued{ctx: context.Background(), ch: ch}, now)
	return oldest.ch, wait, full
}

func (r *Requeue) Drain() []swarm.Chunk {
	items := r.q.drain()
	chunks := make([]swarm.Chunk, len(items))
	for i, it := range items {
		chunks[i] = it.ch
	}
	return chunks
}

// DirectUploadRequeueFailed returns how many requeued direct upload chunks
// were counted as lost.
func (db *DB) DirectUploadRequeueFailed() float64 {
	m := &dto.Metric{}
	_ = db.metrics.DirectUploadRequeueFailed.Write(m)
	return m.GetCounter().GetValue()
}
