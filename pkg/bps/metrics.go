// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"sync/atomic"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Counters count, per cohort, the frames the broker dropped and the streams
// it ended.
type Counters struct {
	WrongStream    uint64 // kind not allowed for the stream's role
	UnknownKind    uint64 // unknown kind
	WrongChallenge uint64 // challenge does not match the stream
	InvalidSOC     uint64 // chunk failed verification
	AuthTimeout    uint64 // pending stream timed out before AUTH
	Retransmit     uint64 // index below the cursor
	QueueReset     uint64 // stream reset for a full queue
	InvalidRoster  uint64 // roster payload does not decode
	Revoked        uint64 // frame from a revoked stream
	Ended          uint64 // frame from a stream of an ended cohort
}

type counters struct {
	wrongStream    atomic.Uint64
	unknownKind    atomic.Uint64
	wrongChallenge atomic.Uint64
	invalidSOC     atomic.Uint64
	authTimeout    atomic.Uint64
	retransmit     atomic.Uint64
	queueReset     atomic.Uint64
	invalidRoster  atomic.Uint64
	revoked        atomic.Uint64
	ended          atomic.Uint64
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
		InvalidRoster:  c.invalidRoster.Load(),
		Revoked:        c.revoked.Load(),
		Ended:          c.ended.Load(),
	}
}

type metrics struct {
	Outcomes          *prometheus.CounterVec
	Delivered         prometheus.Counter
	Cohorts           prometheus.Gauge
	SubscriberDropped *prometheus.CounterVec
}

func newMetrics() metrics {
	subsystem := "bps"

	return metrics{
		Outcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broker_outcomes",
			Help:      "Frames the broker dropped and streams it ended, by outcome.",
		}, []string{"outcome"}),
		Delivered: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accepted_updates",
			Help:      "DATA updates accepted by the broker, from every publisher.",
		}),
		Cohorts: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cohorts",
			Help:      "Live cohorts on the broker.",
		}),
		SubscriberDropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "subscriber_dropped_deliveries",
			Help:      "Valid deliveries a subscriber dropped, by reason.",
		}, []string{"reason"}),
	}
}

// Metrics returns the prometheus collectors of the service.
func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}

// count increments c and the prometheus counter for outcome.
func (s *Service) count(c *atomic.Uint64, outcome string) {
	c.Add(1)
	s.metrics.Outcomes.WithLabelValues(outcome).Inc()
}
