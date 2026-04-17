// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"resenje.org/singleflight"
)

type (
	Loader[T any]         func() (T, error)
	ReuseEvaluator[T any] func(value T) bool
)

type SingleFlightCache[T any] struct {
	mu    sync.RWMutex
	value T

	group   singleflight.Group[string, any]
	key     string
	metrics metricSet
}

func NewSingleFlightCache[T any](metricsPrefix string) *SingleFlightCache[T] {
	return &SingleFlightCache[T]{
		key:     metricsPrefix,
		metrics: newMetricSet(metricsPrefix),
	}
}

func (c *SingleFlightCache[T]) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		c.metrics.Hits,
		c.metrics.Misses,
		c.metrics.Loads,
		c.metrics.SharedLoads,
		c.metrics.LoadErrors,
	}
}

func (c *SingleFlightCache[T]) Set(value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.value = value
}

func (c *SingleFlightCache[T]) PeekOrLoad(ctx context.Context, canReuse ReuseEvaluator[T], loader Loader[T]) (T, error) {
	c.mu.RLock()
	value := c.value
	c.mu.RUnlock()

	if canReuse(value) {
		c.metrics.Hits.Inc()
		return value, nil
	}

	c.metrics.Misses.Inc()

	result, shared, err := c.group.Do(ctx, c.key, func(ctx context.Context) (any, error) {
		c.metrics.Loads.Inc()
		value, err := loader()
		if err != nil {
			c.metrics.LoadErrors.Inc()
			return value, err
		}
		c.Set(value)
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
