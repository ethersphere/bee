// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	CacheHit  m.Counter
	CacheMiss m.Counter
}

func newMetrics() metrics {
	subsystem := "storage_cache"

	return metrics{
		CacheHit: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cache_hit",
			Help:      "Total cache hits.",
		}),
		CacheMiss: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cache_miss",
			Help:      "Total cache misses.",
		}),
	}
}

func (c *Cache) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(c.metrics)
}
