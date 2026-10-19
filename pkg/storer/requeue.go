// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// directUploadRequeueLimit is how many chunks of one direct upload may wait
// for their second push at the same time. Many failures in a short time point
// to a problem that a later push will not fix, so the upload then fails fast.
const directUploadRequeueLimit = 16

// requeued is a chunk of a direct upload waiting for its second push.
type requeued struct {
	ctx context.Context // the Put context, still alive at Done
	ch  swarm.Chunk
}

// requeue holds the chunks of a direct upload that failed once. They are
// pushed again at the end of the upload.
type requeue struct {
	mu    sync.Mutex
	limit int
	items []requeued
}

func newRequeue(limit int) *requeue {
	return &requeue{limit: limit}
}

// add appends r. When the queue is full, it removes the oldest entry in the
// same step and returns it; the caller must push that one now.
func (q *requeue) add(r requeued) (requeued, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) < q.limit {
		q.items = append(q.items, r)
		return requeued{}, false
	}
	oldest := q.items[0]
	q.items = append(q.items[1:], r)
	return oldest, true
}

// drain returns the entries, oldest first, and empties the queue.
func (q *requeue) drain() []requeued {
	q.mu.Lock()
	defer q.mu.Unlock()

	items := q.items
	q.items = nil
	return items
}
