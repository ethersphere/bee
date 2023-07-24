// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"sync/atomic"
	"time"
)

// A WaitingCounter waits for a counter to go to zero.
type WaitingCounter atomic.Int32

// Add increments the counter.
// As opposed to sync.WaitGroup, it is safe to be called after Wait has been called.
func (r *WaitingCounter) Add(c int32) {
	(*atomic.Int32)(r).Add(c)
}

// Done decrements the counter.
// If the counter goes bellow zero a panic will be raised.
func (r *WaitingCounter) Done() {
	if nv := (*atomic.Int32)(r).Add(-1); nv < 0 {
		panic("negative counter value")
	}
}

// Wait blocks waiting for the counter value to reach zero or for the timeout to be reached.
// Is guaranteed to wait for at least a hundred milliseconds regardless of given duration if the counter is positive value.
func (r *WaitingCounter) Wait(waitFor time.Duration) int {
	deadline := time.Now().Add(waitFor)
	for {
		c := int((*atomic.Int32)(r).Load())

		if c <= 0 { // we're done
			return c
		}

		if time.Now().After(deadline) { // timeout
			return c
		}

		time.Sleep(100 * time.Millisecond)
	}
}
