// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalSubscribePullRequests         prometheus.Counter
	TotalSubscribePullRequestsComplete prometheus.Counter
	SubscribePullsStarted              prometheus.Counter
	SubscribePullsComplete             prometheus.Counter
	SubscribePullsFailures             prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "pullstorage"

	return metrics{
		TotalSubscribePullRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_requests",
			Help:      "Total subscribe pull requests.",
		}),
		TotalSubscribePullRequestsComplete: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pull_requests_complete",
			Help:      "Total subscribe pull requests completed.",
		}),
		SubscribePullsStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pulls_started",
			Help:      "Total subscribe pulls started.",
		}),
		SubscribePullsComplete: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pulls_complete",
			Help:      "Total subscribe pulls complete.",
		}),
		SubscribePullsFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscribe_pulls_failures",
			Help:      "Total subscribe pulls failures.",
		})}
}

func (s *PullStorer) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
