// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocker

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	Flag      prometheus.Counter
	Unflag    prometheus.Counter
	Blocklist prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "blocker"

	return metrics{
		Flag: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "flag",
			Help:      "The nubmer of times peers have been flagged.",
		}),
		Unflag: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unflag",
			Help:      "The nubmer of times peers have been unflagged.",
		}),
		Blocklist: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "blocklist",
			Help:      "The nubmer of times peers have been blocklisted.",
		}),
	}
}

func (s *Blocker) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
