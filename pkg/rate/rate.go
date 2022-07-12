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
	mtx        sync.Mutex
	windows    map[int64]int
	windowSize int64 // window size in milliseconds
	now        func() time.Time
}

// New returns a new rate tracker with a defined window size that must be greater than one millisecond.
func New(windowsSize time.Duration) *Rate {
	return &Rate{
		windows:    make(map[int64]int),
		windowSize: windowsSize.Milliseconds(),
		now:        func() time.Time { return time.Now() },
	}

}

// add uses the current time and rounds it down to a window
// and increments the window's value.
func (r *Rate) Add(count int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	defer r.cleanup()

	window := r.now().UnixMilli() / r.windowSize
	r.windows[window] += count
}

// rate uses the current window and previous windows' counter to compute a moving-window rate in seconds.
// the rate is computed by first calculating how far along the current time is in the current window
// as a ratio between 0 and 1.0. Then, the sum of currentWindowCount + (1 - ratio) * previousWindowCounter
// is returned as the interpolated counter between the two windows.
func (r *Rate) Rate() float64 {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	defer r.cleanup()

	now := r.now().UnixMilli()
	window := now / r.windowSize

	interpolate := 1 - float64(now-(window*r.windowSize))/float64(r.windowSize)

	return milliInSeconds * ((float64(r.windows[window])) + ((interpolate) * float64(r.windows[window-1]))) / float64(r.windowSize)
}

// cleanup removes windows older than the most recent two windows.
// Must be called under lock.
func (r *Rate) cleanup() {

	window := r.now().UnixMilli() / r.windowSize
	for k := range r.windows {
		if k <= window-2 {
			delete(r.windows, k)
		}
	}
}
