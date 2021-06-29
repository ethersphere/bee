// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ratelimit provides a mechanism to rate limit requests based on a string key,
// refill rate and burst amount. Under the hood, it's a token bucket of size burst amount,
// that refills at the refill rate.

package ratelimit

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var ErrRateLimitExceeded = errors.New("rate limit exceeded")

type Limiter struct {
	mux     sync.Mutex
	limiter map[string]*rate.Limiter
	rate    rate.Limit
	burst   int
}

// New returns a new Limiter object with refresh rate and burst amount
func New(r time.Duration, b int) *Limiter {
	return &Limiter{
		limiter: make(map[string]*rate.Limiter),
		rate:    rate.Every(r),
		burst:   b,
	}
}

// Allow checks if the limiter that belongs to 'key' has not exceeded the limit.
func (l *Limiter) Allow(key string, count int) bool {

	l.mux.Lock()
	defer l.mux.Unlock()

	limiter, ok := l.limiter[key]
	if !ok {
		limiter = rate.NewLimiter(l.rate, l.burst)
		l.limiter[key] = limiter
	}

	return limiter.AllowN(time.Now(), count)
}

// Clear deletes the limiter that belongs to 'key'
func (l *Limiter) Clear(key string) {

	l.mux.Lock()
	defer l.mux.Unlock()

	delete(l.limiter, key)
}
