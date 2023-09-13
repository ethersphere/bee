// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncutil

import (
	"sync"
	"time"
)

// WaitWithTimeout waits for the waitgroup to finish for the given timeout.
// It returns true if the waitgroup finished before the timeout, false otherwise.
func WaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()
	select {
	case <-c:
		return true
	case <-time.After(timeout):
		return false
	}
}
