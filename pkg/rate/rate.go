// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rate is a thread-safe rate tracker with per second resolution.
// Under the hood, it sums counts over a sliding window split into buckets.
// A count stays in the rate for at least one window and at most one window
// (rounded up to whole buckets) plus one bucket, so the rate reaches zero no
// earlier than one window after the last Add.
package rate

import (
	"sync"
	"time"
)

const (
	milliInSeconds   = 1000
	bucketsPerWindow = 30
)

// bucket holds the count added during one bucketSize slice of time. id is the
// time in milliseconds divided by bucketSize, so it identifies which slice the
// count belongs to.
type bucket struct {
	id    int64
	count int
}

type Rate struct {
	mtx        sync.Mutex
	buckets    []bucket // ring buffer indexed by id modulo its length
	windowSize int64    // window size in milliseconds
	bucketSize int64    // bucket size in milliseconds
	now        func() time.Time
}

// New returns a new rate tracker with a defined window size that must be greater than one millisecond.
func New(windowSize time.Duration) *Rate {
	// a window shorter than 1ms would make the rate divide by zero
	ws := max(windowSize.Milliseconds(), 1)
	// split the window into bucketsPerWindow buckets; windows shorter than
	// bucketsPerWindow milliseconds get 1ms buckets
	bs := max(ws/bucketsPerWindow, 1)
	return &Rate{
		// one slot per bucket in the window, rounded up so the ring covers the
		// whole window, plus the current, partly elapsed one
		// e.g. 1s window: 33ms buckets, ceil(1000/33)=31 slots + 1 = 32
		buckets:    make([]bucket, (ws+bs-1)/bs+1),
		windowSize: ws,
		bucketSize: bs,
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

	id := r.now().UnixMilli() / r.bucketSize
	b := &r.buckets[r.index(id)]
	// the slot still holds a bucket that has left the window: start over
	if b.id != id {
		*b = bucket{id: id}
	}
	b.count += count
}

// Rate returns the per second rate over the most recent window.
func (r *Rate) Rate() float64 {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	current := r.now().UnixMilli() / r.bucketSize
	// the ring holds exactly the buckets from oldest to current
	oldest := current - int64(len(r.buckets)) + 1

	var sum int
	for _, b := range r.buckets {
		// stale slots are skipped and overwritten by the next Add that lands
		// on them; ids ahead of current are left over from a backward clock jump
		if b.id >= oldest && b.id <= current {
			sum += b.count
		}
	}

	return milliInSeconds * float64(sum) / float64(r.windowSize)
}

// index returns the ring slot for a bucket id.
func (r *Rate) index(id int64) int {
	n := int64(len(r.buckets))
	return int(((id % n) + n) % n)
}
