// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncutil

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestWaitWithTimeout(t *testing.T) {
	var wg sync.WaitGroup

	synctest.Test(t, func(t *testing.T) {
		if !WaitWithTimeout(&wg, 10*time.Millisecond) {
			t.Fatal("want timeout; have none")
		}

		wg.Add(1)
		if WaitWithTimeout(&wg, 10*time.Millisecond) {
			t.Fatal("have timeout; want none")
		}

		wg.Done()
		if !WaitWithTimeout(&wg, 10*time.Millisecond) {
			t.Fatal("want no timeout; have none")
		}
	})
}
