// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rate

// Package rate is a thread-safe rate tracker with per second resolution.
// Under the hood, it uses a moving window to interpolate a rate.

import (
	"sync"
	"time"
)

const milliInSeconds = 1000

type Rate struct {
	buckets    map[int64]int
	mtx        sync.Mutex
	windowSize int64        // window size in milliseconds
	now        func() int64 // func that returns the current time in milliseconds
}

// New returns a new rate tracker with a defined window size that must be greater than one millisecond.
func New(windowsSize time.Duration) *Rate {
	return &Rate{
		buckets:    make(map[int64]int),
		windowSize: int64(windowsSize / time.Millisecond),
		now:        func() int64 { return time.Now().UnixMilli() },
	}
}

// add uses the current time and rounds it down to window-sized buckets
// and increments the bucket's value.
func (r *Rate) Add(count int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	defer r.cleanup()

	bucket := r.now() / r.windowSize
	r.buckets[bucket] += count
}

// rate uses the current bucket and previous buckets' counter to compute a moving-window rate in seconds.
// the rate is computed by first calculating how far along the current time is in the current bucket
// as a ratio between 0 and 1.0. Then, the sum of currentBucketCount + (1 - ratio) * previousBucketCounter
// is returned as the interpolated counter between the two windows.
func (r *Rate) Rate() float64 {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	defer r.cleanup()

	now := r.now()
	bucket := now / r.windowSize

	interpolate := 1 - float64(now-(bucket*r.windowSize))/float64(r.windowSize)

	return milliInSeconds * ((float64(r.buckets[bucket])) + ((interpolate) * float64(r.buckets[bucket-1]))) / float64(r.windowSize)
}

// cleanup removes buckets older than the most recent two buckets
func (r *Rate) cleanup() {

	bucket := r.now() / r.windowSize

	for k := range r.buckets {
		if k <= bucket-2 {
			delete(r.buckets, k)
		}
	}
}

func (r *Rate) SetTimeFunc(f func() int64) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.now = f
}
