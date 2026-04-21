// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metricSet struct {
	Hits        prometheus.Counter
	Misses      prometheus.Counter
	Loads       prometheus.Counter
	SharedLoads prometheus.Counter
	LoadErrors  prometheus.Counter
}

func newMetricSet(prefix string) metricSet {
	subsystem := "eth_backend_cache"
	return metricSet{
		Hits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      prefix + "_hits",
			Help:      prefix + " cache hits",
		}),
		Misses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      prefix + "_misses",
			Help:      prefix + " cache misses",
		}),
		Loads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      prefix + "_loads",
			Help:      prefix + " cache backend loads",
		}),
		SharedLoads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      prefix + "_shared_loads",
			Help:      prefix + " cache shared loads",
		}),
		LoadErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      prefix + "_load_errors",
			Help:      prefix + " cache load errors",
		}),
	}
}
