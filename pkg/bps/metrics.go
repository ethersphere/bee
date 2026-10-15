// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"sync/atomic"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Counters are the per-cohort counters of the silent outcomes SWIP-74 makes a
// broker expose.
type Counters struct {
	WrongStream    uint64 // a frame from a subscriber stream
	UnknownKind    uint64 // a frame of a kind BPS-lite does not define
	WrongChallenge uint64 // a frame whose challenge is not its stream's
	InvalidSOC     uint64 // a chunk that does not validate at the derived id
	AuthTimeout    uint64 // a pending stream disconnected at the auth timeout
	Retransmit     uint64 // a DATA frame below the cursor
	QueueReset     uint64 // a subscriber stream reset for a full queue
}

type counters struct {
	wrongStream    atomic.Uint64
	unknownKind    atomic.Uint64
	wrongChallenge atomic.Uint64
	invalidSOC     atomic.Uint64
	authTimeout    atomic.Uint64
	retransmit     atomic.Uint64
	queueReset     atomic.Uint64
}

func (c *counters) snapshot() Counters {
	return Counters{
		WrongStream:    c.wrongStream.Load(),
		UnknownKind:    c.unknownKind.Load(),
		WrongChallenge: c.wrongChallenge.Load(),
		InvalidSOC:     c.invalidSOC.Load(),
		AuthTimeout:    c.authTimeout.Load(),
		Retransmit:     c.retransmit.Load(),
		QueueReset:     c.queueReset.Load(),
	}
}

type metrics struct {
	Outcomes  *prometheus.CounterVec
	Delivered prometheus.Counter
	Cohorts   prometheus.Gauge
}

func newMetrics() metrics {
	subsystem := "bps"

	return metrics{
		Outcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "dropped_frames",
			Help:      "Frames dropped or streams reset by the broker, by outcome.",
		}, []string{"outcome"}),
		Delivered: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accepted_updates",
			Help:      "DATA updates accepted by the broker.",
		}),
		Cohorts: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cohorts",
			Help:      "Live cohorts on the broker.",
		}),
	}
}

// Metrics returns the prometheus collectors of the service.
func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}

// count increments the per-cohort counter c and its prometheus outcome.
func (s *Service) count(c *atomic.Uint64, outcome string) {
	c.Add(1)
	s.metrics.Outcomes.WithLabelValues(outcome).Inc()
}
