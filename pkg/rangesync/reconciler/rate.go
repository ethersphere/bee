// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"sync"
	"time"
)

// rateWindow is pkg/rate's moving-window rate (copied here so that its clock
// can be injected): chunks per second over the current and the previous
// window, the previous one interpolated by how far the current one is along.
type rateWindow struct {
	mu      sync.Mutex
	windows map[int64]int
	size    int64 // window size in milliseconds
	now     func() time.Time
}

func newRateWindow(size time.Duration, now func() time.Time) *rateWindow {
	return &rateWindow{windows: make(map[int64]int), size: size.Milliseconds(), now: now}
}

// Add counts n chunks in the current window.
func (r *rateWindow) Add(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := r.now().UnixMilli() / r.size
	r.windows[w] += n
	r.cleanupLocked(w)
}

// Rate returns the rate in chunks per second.
func (r *rateWindow) Rate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UnixMilli()
	w := now / r.size
	r.cleanupLocked(w)
	interpolate := 1 - float64(now-w*r.size)/float64(r.size)
	return 1000 * (float64(r.windows[w]) + interpolate*float64(r.windows[w-1])) / float64(r.size)
}

func (r *rateWindow) cleanupLocked(w int64) {
	for k := range r.windows {
		if k <= w-2 {
			delete(r.windows, k)
		}
	}
}
