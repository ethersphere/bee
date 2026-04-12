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
type ReuseEvaluator[T any] func(value T, expiresAt time.Time) (bool, time.Time)

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

func (c *ExpiringSingleFlightCache[T]) Set(value T, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if expiresAt.After(c.expiresAt) {
		c.value = value
		c.expiresAt = expiresAt
	}
}

func (c *ExpiringSingleFlightCache[T]) PeekOrLoad(ctx context.Context, canReuse ReuseEvaluator[T], loader Loader[T]) (T, error) {
	c.mu.RLock()
	var (
		value     T
		expiresAt time.Time
	)

	hasEntry := !c.expiresAt.IsZero()
	if hasEntry {
		value = c.value
		expiresAt = c.expiresAt
	}
	c.mu.RUnlock()

	if hasEntry {
		reuse, newExpiresAt := canReuse(value, expiresAt)
		if reuse {
			c.metrics.Hits.Inc()
			if !newExpiresAt.IsZero() {
				c.Set(value, newExpiresAt)
			}
			return value, nil
		}
	}

	c.metrics.Misses.Inc()

	result, shared, err := c.group.Do(ctx, c.key, func(ctx context.Context) (any, error) {
		c.metrics.Loads.Inc()
		value, expiresAt, err := loader()
		if err != nil {
			c.metrics.LoadErrors.Inc()
			return value, err
		}
		c.Set(value, expiresAt)
		return value, nil
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
