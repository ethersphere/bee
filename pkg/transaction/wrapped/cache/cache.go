// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type Loader[T any] func() (T, error)

type ExpiringSingleFlightCache[T any] struct {
	ttl time.Duration

	mu        sync.RWMutex
	value     T
	valid     bool
	expiresAt time.Time

	group   singleflight.Group
	key     Key
	metrics metricSet
}

func NewExpiringSingleFlightCache[T any](ttl time.Duration, key Key) *ExpiringSingleFlightCache[T] {
	return &ExpiringSingleFlightCache[T]{
		ttl:     ttl,
		key:     key,
		metrics: newMetricSet(string(key)),
	}
}

func (c *ExpiringSingleFlightCache[T]) Get(now time.Time) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.valid && now.Before(c.expiresAt) {
		return c.value, true
	}

	var zero T
	return zero, false
}

func (c *ExpiringSingleFlightCache[T]) Set(value T, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.value = value
	c.valid = true
	c.expiresAt = now.Add(c.ttl)
}

func (c *ExpiringSingleFlightCache[T]) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.valid = false
	c.metrics.Invalidates.Inc()
}

func (c *ExpiringSingleFlightCache[T]) GetOrLoad(now time.Time, loader Loader[T]) (T, error) {
	if v, ok := c.Get(now); ok {
		c.metrics.Hits.Inc()
		return v, nil
	}

	c.metrics.Misses.Inc()

	result, err, shared := c.group.Do(string(c.key), func() (any, error) {
		c.metrics.Loads.Inc()
		val, err := loader()
		if err != nil {
			c.metrics.LoadErrors.Inc()
			return val, err
		}
		c.Set(val, time.Now())
		return val, nil
	})

	if shared {
		c.metrics.SharedLoads.Inc()
	}

	if err != nil {
		var zero T
		return zero, err
	}

	return result.(T), nil
}
