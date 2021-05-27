// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalSubscribePullRequests prometheus.Counter
	SubscribePullsExecuted     prometheus.Counter
	SubscribePullsFailures     prometheus.Counter
	TotalSubscribePullsTime    prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "pullsync"

	return metrics{
		TotalSubscribePullRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_requests",
			Help:      "Total subscribe pull requests.",
		}),
		SubscribePullsExecuted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pulls_executed",
			Help:      "Total subscribe pulls executed.",
		}),
		SubscribePullsFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pulls_failures",
			Help:      "Total subscribe pulls failures.",
		}),
		TotalSubscribePullsTime: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pulls_time",
			Help:      "Total time taken for pull subscriptions.",
		})}
}

func (s *ps) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
