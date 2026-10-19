// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rate is a thread-safe rate tracker with per second resolution.
// Under the hood, it sums counts over a sliding window split into buckets.
// A count stays in the rate for at least one window and at most one window
// plus one bucket, so the rate reaches zero no earlier than one window after
// the last Add.
package rate

import (
	"sync"
	"time"
)

const (
	milliInSeconds   = 1000
	bucketsPerWindow = 30
)

type Rate struct {
	mtx        sync.Mutex
	buckets    map[int64]int
	windowSize int64 // window size in milliseconds
	bucketSize int64 // bucket size in milliseconds
	now        func() time.Time
}

// New returns a new rate tracker with a defined window size that must be greater than one millisecond.
func New(windowSize time.Duration) *Rate {
	ws := max(windowSize.Milliseconds(), 1)
	return &Rate{
		buckets:    make(map[int64]int),
		windowSize: ws,
		bucketSize: max(ws/bucketsPerWindow, 1),
		now:        func() time.Time { return time.Now() },
	}
}

// Add increments the bucket for the current time by count.
// Counts that are not positive are ignored.
func (r *Rate) Add(count int) {
	if count <= 0 {
		return
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()
	defer r.cleanup()

	r.buckets[r.currentBucket()] += count
}

// Rate returns the per second rate over the most recent window.
func (r *Rate) Rate() float64 {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	defer r.cleanup()

	current := r.currentBucket()
	oldest := r.oldestBucket()

	var sum int
	for bucket, count := range r.buckets {
		// buckets ahead of current are left over from a backward clock jump
		if bucket >= oldest && bucket <= current {
			sum += count
		}
	}

	return milliInSeconds * float64(sum) / float64(r.windowSize)
}

// currentBucket returns the bucket for the current time.
// Must be called under lock.
func (r *Rate) currentBucket() int64 {
	return r.now().UnixMilli() / r.bucketSize
}

// oldestBucket returns the oldest bucket still inside the window. The current
// bucket is only partly elapsed, so one extra bucket is kept to not expire
// counts before a full window has passed.
// Must be called under lock.
func (r *Rate) oldestBucket() int64 {
	return r.currentBucket() - r.windowSize/r.bucketSize
}

// cleanup removes buckets that have fallen out of the window.
// Must be called under lock.
func (r *Rate) cleanup() {
	oldest := r.oldestBucket()
	for bucket := range r.buckets {
		if bucket < oldest {
			delete(r.buckets, bucket)
		}
	}
}
