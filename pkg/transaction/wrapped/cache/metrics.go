// Copyright 2025 The Swarm Authors. All rights reserved.
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
	Invalidates prometheus.Counter
}

type Metrics struct {
	BlockNumber metricSet
	Unknown     metricSet
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
		Invalidates: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      prefix + "_invalidations",
			Help:      prefix + " cache invalidations",
		}),
	}
}

func (mtr *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		mtr.BlockNumber.Hits,
		mtr.BlockNumber.Misses,
		mtr.BlockNumber.Loads,
		mtr.BlockNumber.SharedLoads,
		mtr.BlockNumber.LoadErrors,
		mtr.BlockNumber.Invalidates,

		mtr.Unknown.Hits,
		mtr.Unknown.Misses,
		mtr.Unknown.Loads,
		mtr.Unknown.SharedLoads,
		mtr.Unknown.LoadErrors,
		mtr.Unknown.Invalidates,
	}
}
