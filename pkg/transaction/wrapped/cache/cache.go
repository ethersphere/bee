// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"resenje.org/singleflight"
)

type Loader[T any] func() (T, time.Time, error)
type ExpiringSingleFlightCache[T any] struct {
	mu        sync.RWMutex
	value     T
	expiresAt time.Time

	group   singleflight.Group[string, any]
	key     string
	metrics metricSet
}

func NewExpiringSingleFlightCache[T any](metricsPrefix string) *ExpiringSingleFlightCache[T] {
	return &ExpiringSingleFlightCache[T]{
		key:     metricsPrefix,
		metrics: newMetricSet(metricsPrefix),
	}
}

func (c *ExpiringSingleFlightCache[T]) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		c.metrics.Hits,
		c.metrics.Misses,
		c.metrics.Loads,
		c.metrics.SharedLoads,
		c.metrics.LoadErrors,
	}
}

func (c *ExpiringSingleFlightCache[T]) Get(now time.Time) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if now.Before(c.expiresAt) {
		return c.value, true
	}

	var zero T
	return zero, false
}

func (c *ExpiringSingleFlightCache[T]) Set(value T, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.value = value
	c.expiresAt = expiresAt
}

func (c *ExpiringSingleFlightCache[T]) GetOrLoad(ctx context.Context, now time.Time, loader Loader[T]) (T, error) {
	if v, ok := c.Get(now); ok {
		c.metrics.Hits.Inc()
		return v, nil
	}

	c.metrics.Misses.Inc()

	result, shared, err := c.group.Do(ctx, c.key, func(ctx context.Context) (any, error) {
		c.metrics.Loads.Inc()
		val, expiresAt, err := loader()
		if err != nil {
			c.metrics.LoadErrors.Inc()
			return val, err
		}
		c.Set(val, expiresAt)
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
