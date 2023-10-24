// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spinlock

import (
	"errors"
	"time"
)

var ErrTimedOut = errors.New("timed out waiting for condition")

// Wait blocks execution until condition is satisfied or until it times out.
func Wait(timeoutDur time.Duration, cond func() bool) error {
	return WaitWithInterval(timeoutDur, time.Millisecond*50, cond)
}

// WaitWithInterval blocks execution until condition is satisfied or until it times out.
// Condition is checked on specified checkIntervalDur.
func WaitWithInterval(timeoutDur, checkIntervalDur time.Duration, cond func() bool) error {
	timeout := time.NewTimer(timeoutDur)
	defer timeout.Stop()

	condCheckTicker := time.NewTicker(checkIntervalDur)
	defer condCheckTicker.Stop()

	for {
		select {
		case <-timeout.C:
			if cond() {
				return nil
			}
			return ErrTimedOut

		case <-condCheckTicker.C:
			if cond() {
				return nil
			}
		}
	}
}
