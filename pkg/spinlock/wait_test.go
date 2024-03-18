// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spinlock_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/spinlock"
)

func TestWait(t *testing.T) {
	t.Parallel()

	t.Run("timed out", func(t *testing.T) {
		t.Parallel()

		err := spinlock.Wait(time.Millisecond*20, func() bool { return false })
		if !errors.Is(err, spinlock.ErrTimedOut) {
			t.Fatal("expecting to time out")
		}
	})

	t.Run("condition satisfied", func(t *testing.T) {
		t.Parallel()

		spinStartTime := time.Now()
		condCallCount := 0
		err := spinlock.Wait(time.Millisecond*200, func() bool {
			condCallCount++
			return time.Since(spinStartTime) >= time.Millisecond*100
		})
		if err != nil {
			t.Fatal("expecting to end wait without time out")
		}
		if condCallCount == 0 {
			t.Fatal("expecting condition function to be called")
		}
	})
}
