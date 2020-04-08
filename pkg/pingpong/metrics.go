// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pingpong

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	PingSentCount     prometheus.Counter
	PongSentCount     prometheus.Counter
	PingReceivedCount prometheus.Counter
	PongReceivedCount prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "pingpong"

	return metrics{
		PingSentCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_sent_count",
			Help:      "Number ping requests sent.",
		}),
		PongSentCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pong_sent_count",
			Help:      "Number of pong responses sent.",
		}),
		PingReceivedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ping_received_count",
			Help:      "Number ping requests received.",
		}),
		PongReceivedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pong_received_count",
			Help:      "Number of pong responses received.",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
