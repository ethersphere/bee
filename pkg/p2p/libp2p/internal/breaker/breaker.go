// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breaker

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrClosed      = errors.New("breaker closed")
	timeNow        = time.Now // used instead of time.Since() so it can be mocked
	failInterval   = 10 * time.Minute
	initialBackoff = 2 * time.Minute
	backoffLimit   = 1 * time.Hour
)

type breaker struct {
	limit                int
	consFailedCalls      int
	firstFailedTimestamp time.Time
	closedTimestamp      time.Time
	backoff              time.Duration
	mtx                  sync.Mutex
}

func NewBreaker(limit int) *breaker {
	return &breaker{
		limit:   limit,
		backoff: initialBackoff,
	}
}

// executes f() if the limit number of consecutive failed calls is not reached within fail interval.
// f() call is not locked so it can still be executed concurently
// returns `errClosed` if the limit is reached and f() result otherwise
func (b *breaker) Execute(f func() error) error {
	if err := b.beforef(); err != nil {
		return err
	}

	return b.afterf(f())
}

func (b *breaker) beforef() error {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if b.consFailedCalls >= b.limit {
		if b.firstFailedTimestamp.IsZero() || timeNow().Sub(b.closedTimestamp) < b.backoff {
			return ErrClosed
		}

		b.resetFailed()
		if newBackoff := b.backoff * 2; newBackoff <= backoffLimit {
			b.backoff = newBackoff
		} else {
			b.backoff = backoffLimit
		}
	}

	if !b.firstFailedTimestamp.IsZero() && timeNow().Sub(b.firstFailedTimestamp) >= failInterval {
		b.resetFailed()
	}

	return nil
}

func (b *breaker) afterf(err error) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if err != nil {
		if b.consFailedCalls == 0 {
			b.firstFailedTimestamp = time.Now()
		}

		b.consFailedCalls++
		if b.consFailedCalls == b.limit {
			b.closedTimestamp = time.Now()
		}

		return err
	}

	b.resetFailed()
	return nil
}

func (b *breaker) resetFailed() {
	b.consFailedCalls = 0
	b.firstFailedTimestamp = time.Time{}
}
