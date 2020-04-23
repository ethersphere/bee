// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breaker

import (
	"errors"
	"sync"
	"time"
)

var _ Interface = (*breaker)(nil)

var (
	ErrClosed = errors.New("breaker closed")
	timeNow   = time.Now // used instead of time.Since() so it can be mocked
)

type Interface interface {
	Execute(f func() error) error
}

type breaker struct {
	limit                int
	consFailedCalls      int
	firstFailedTimestamp time.Time
	closedTimestamp      time.Time
	backoff              time.Duration
	maxBackoff           time.Duration
	failInterval         time.Duration
	mtx                  sync.Mutex
}

type Options struct {
	Limit        int
	FailInterval time.Duration
	StartBackoff time.Duration
	MaxBackoff   time.Duration
}

func NewBreaker(o Options) Interface {
	breaker := &breaker{
		limit:        o.Limit,
		backoff:      o.StartBackoff,
		maxBackoff:   o.MaxBackoff,
		failInterval: o.FailInterval,
	}

	if o.Limit == 0 {
		breaker.limit = 100
	}

	if o.FailInterval == 0 {
		breaker.failInterval = 30 * time.Minute
	}

	if o.MaxBackoff == 0 {
		breaker.maxBackoff = time.Hour
	}

	if o.StartBackoff == 0 {
		breaker.backoff = 2 * time.Minute
	}

	return breaker
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
		if b.closedTimestamp.IsZero() || timeNow().Sub(b.closedTimestamp) < b.backoff {
			return ErrClosed
		}

		b.resetFailed()
		if newBackoff := b.backoff * 2; newBackoff <= b.maxBackoff {
			b.backoff = newBackoff
		} else {
			b.backoff = b.maxBackoff
		}
	}

	if !b.firstFailedTimestamp.IsZero() && timeNow().Sub(b.firstFailedTimestamp) >= b.failInterval {
		b.resetFailed()
	}

	return nil
}

func (b *breaker) afterf(err error) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if err != nil {
		if b.consFailedCalls == 0 {
			b.firstFailedTimestamp = timeNow()
		}

		b.consFailedCalls++
		if b.consFailedCalls == b.limit {
			b.closedTimestamp = timeNow()
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
