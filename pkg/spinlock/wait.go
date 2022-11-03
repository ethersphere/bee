// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spinlock

import (
	"errors"
	"testing"
	"time"
)

var ErrTimedOut = errors.New("timed out waiting for condition")

// Wait blocks execution until condition is satisfied or until it times out.
func Wait(t *testing.T, timeoutDur time.Duration, cond func() bool) error {
	t.Helper()

	timeout := time.NewTimer(timeoutDur)
	defer timeout.Stop()

	condCheckTicker := time.NewTicker(time.Millisecond * 20)
	defer condCheckTicker.Stop()

	for {
		select {
		case <-timeout.C:
			return ErrTimedOut

		case <-condCheckTicker.C:
			if cond() {
				return nil
			}
		}
	}
}
