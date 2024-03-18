// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	CacheHit  prometheus.Counter
	CacheMiss prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "storage_cache"

	return metrics{
		CacheHit: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cache_hit",
			Help:      "Total cache hits.",
		}),
		CacheMiss: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cache_miss",
			Help:      "Total cache misses.",
		}),
	}
}

func (c *Cache) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(c.metrics)
}
