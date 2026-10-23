// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// directUploadRequeueLimit is how many chunks of one direct upload may
	// wait for their second push. When it is reached, the oldest is pushed
	// again, and the upload fails if that push fails too.
	directUploadRequeueLimit = 16

	// directUploadRequeueMinAge is how long a chunk waits before its second
	// push when the queue is full, so that a short outage can pass first.
	directUploadRequeueMinAge = 30 * time.Second
)

// requeued is a chunk of a direct upload waiting for its second push.
type requeued struct {
	ctx   context.Context // the Put context, still alive at Done
	ch    swarm.Chunk
	since time.Time // when the chunk was queued
}

// requeue holds the chunks of a direct upload that failed once. They are
// pushed again at the end of the upload.
type requeue struct {
	mu     sync.Mutex
	limit  int
	minAge time.Duration
	items  []requeued
}

func newRequeue(limit int, minAge time.Duration) *requeue {
	return &requeue{limit: limit, minAge: minAge}
}

// add appends r. When the queue is full, it removes and returns the oldest
// entry and how long it still has to wait; the caller must push it.
func (q *requeue) add(r requeued, now time.Time) (oldest requeued, wait time.Duration, full bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	r.since = now
	if len(q.items) < q.limit {
		q.items = append(q.items, r)
		return requeued{}, 0, false
	}
	oldest = q.items[0]
	q.items = append(q.items[1:], r)
	return oldest, max(0, q.minAge-now.Sub(oldest.since)), true
}

// drain returns the entries, oldest first, and empties the queue.
func (q *requeue) drain() []requeued {
	q.mu.Lock()
	defer q.mu.Unlock()

	items := q.items
	q.items = nil
	return items
}
