// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"sync/atomic"
	"time"
)

type WaitingCounter atomic.Int32

func (r *WaitingCounter) Add(c int32) {
	(*atomic.Int32)(r).Add(c)
}

func (r *WaitingCounter) Done() {
	r.Add(-1)
}

func (r *WaitingCounter) Wait(waitFor time.Duration) (runningGoroutines int) {
	finish := time.Now().Add(waitFor)
	for {
		runningGoroutines = int((*atomic.Int32)(r).Load())

		if runningGoroutines == 0 {
			return
		} else if runningGoroutines < 0 {
			panic("number of running goroutines can not be negative")
		}

		if time.Now().After(finish) {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}
