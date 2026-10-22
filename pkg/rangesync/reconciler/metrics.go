// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"github.com/prometheus/client_golang/prometheus"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

// metrics are the reconciler's metrics.
type metrics struct {
	// SyncRate is the reconciler's own SyncRate (§10 sync_rate).
	SyncRate prometheus.GaugeFunc
	// Sessions counts initiated sessions by result.
	Sessions *prometheus.CounterVec
	// Failures counts PeerScorer failures.
	Failures prometheus.Counter
	// Stripes counts stripe transfers by result: done, reassigned or
	// timeout; dropped counts the ranges, or one session's ranges, not
	// registered past a registry bound, and idle the peers' claims ended
	// for storing nothing (claimIdleTimeout).
	Stripes *prometheus.CounterVec
	// VerificationDiff is the number of differences (chunks this node lacked
	// and the verifier delivered; IDs only listed are not counted) the
	// verification round found per verified stripe.
	VerificationDiff prometheus.Histogram
	// RoundInterval is the current adaptive round interval (§8.2).
	RoundInterval prometheus.GaugeFunc
	// Peers is the number of connected peers the reconciler follows, by the
	// state of their machine (§8.1), read when collected.
	Peers peersCollector
	// InitiatedSessionCap is the current per-node cap on the sessions this
	// node opens at once (InitiatedSessionCap, set by each round from its
	// eligible peers).
	InitiatedSessionCap prometheus.GaugeFunc
}

func newMetrics(syncRate, roundInterval func() float64, peers func() map[fsmState]int, initiatedSessionCap func() float64) metrics {
	const subsystem = "rangesync"

	return metrics{
		SyncRate: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sync_rate",
			Help:      "Catch-up chunks per second fetched by rangesync (reported through SyncRate).",
		}, syncRate),
		Sessions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sessions_total",
			Help:      "Sessions initiated by the reconciler, by result.",
		}, []string{"result"}),
		Failures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_failures_total",
			Help:      "Protocol violations reported to the peer score.",
		}),
		Stripes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stripes_total",
			Help:      "Striped transfers of a stripe, by result (done, reassigned, timeout; dropped: ranges not registered; idle: claims ended for storing nothing).",
		}, []string{"result"}),
		VerificationDiff: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "verification_diff",
			Help:      "Chunks delivered per stripe verified with a peer other than its deliverer.",
			Buckets:   []float64{0, 1, 4, 16, 64, 256, 1024, 4096, 16384, 65536},
		}),
		RoundInterval: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "round_interval_seconds",
			Help:      "Current adaptive interval between reconciliation rounds.",
		}, roundInterval),
		Peers: peersCollector{
			desc: prometheus.NewDesc(
				prometheus.BuildFQName(m.Namespace, subsystem, "peers"),
				"Connected peers the reconciler follows, by the state of their machine.",
				[]string{"state"}, nil,
			),
			count: peers,
		},
		InitiatedSessionCap: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "initiated_session_cap",
			Help:      "Current cap on the sessions this node opens at once, set by each round from its eligible peers.",
		}, initiatedSessionCap),
	}
}

func (m metrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.SyncRate, m.Sessions, m.Failures, m.Stripes, m.VerificationDiff, m.RoundInterval, m.Peers, m.InitiatedSessionCap}
}

// peersCollector is the peers{state} gauge: count returns the number of
// peers in each state when it is collected, and every state is reported, 0
// when no peer is in it. Removed counts the peers dropped from sessions
// (disjoint neighborhoods or another Mapping) that are still connected.
type peersCollector struct {
	desc  *prometheus.Desc
	count func() map[fsmState]int
}

var _ prometheus.Collector = peersCollector{}

func (c peersCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c peersCollector) Collect(ch chan<- prometheus.Metric) {
	counts := c.count()
	for s := stateProbing; s <= stateRemoved; s++ {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(counts[s]), s.String())
	}
}
