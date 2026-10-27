// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// metrics are the reconciler's metrics.
type metrics struct {
	// SyncRate is the reconciler's own SyncRate (§10 sync_rate).
	SyncRate prometheus.GaugeFunc
	// Sessions counts initiated sessions by result.
	Sessions *prometheus.CounterVec
	// Failures counts PeerScorer failures by reason (failureReason).
	Failures *prometheus.CounterVec
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
	// StripeQueueWait is a stripe job's wait in the queue, from its
	// registration to the stripe worker taking it, by kind (jobOwn,
	// jobClaim), observed when it starts: a job dropped from the queue
	// (preempted, Close) observes none.
	StripeQueueWait *prometheus.HistogramVec
	// FirstChunk is the time from the start of catch-up work to the first
	// chunk it stores, by kind: a stripe job (jobOwn, jobClaim, as it
	// started), from the worker taking it, in its transfer or its
	// verification, and a catch-up fetch (jobCatchUp), from the fetch's
	// start. Observed once per job or fetch; work that stores nothing
	// observes none.
	FirstChunk *prometheus.HistogramVec
}

// The kinds of catch-up work (StripeQueueWait, FirstChunk): this node's own
// stripe job (stripeJob.own: a bootstrap or a radius decrease's new half), a
// peer's claim, and a catch-up fetch (CatchUpMinDiff wants or more) of a
// session or its retries.
const (
	jobOwn     = "own"
	jobClaim   = "claim"
	jobCatchUp = "catch_up"
)

// jobKind returns the kind of a stripe job: own or a claim.
func jobKind(own bool) string {
	if own {
		return jobOwn
	}
	return jobClaim
}

// waitBuckets are the buckets of StripeQueueWait and FirstChunk, in
// seconds: from catchUpGrace (2 s) through claimIdleTimeout (2 min), 10
// minutes and StripeTimeout (15 min) to the hours a bootstrap runs, for
// which claims queue.
var waitBuckets = []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 14400}

// The reasons of peer_failures_total (failureReason), after the sentinel
// error a failure wraps.
const (
	// failureStripeVerification: a deliverer's stripe lacked chunks at its
	// verification (ErrStripeVerification).
	failureStripeVerification = "stripe_verification"
	// failureVerifierWithheld: a verifier listed IDs it did not deliver
	// (ErrVerifierWithheld).
	failureVerifierWithheld = "verifier_withheld"
	// failureFetchShortfall: a fetch's shortfall past the tolerance, scored
	// or, blatant or repeated, a violation (rangesync.ErrFetchShortfall).
	failureFetchShortfall = "fetch_shortfall"
	// failureInvalidMessage: any other protocol violation
	// (rangesync.ErrInvalidMessage): an invalid message, a fetch that
	// stalled or was reset too often, an unsolicited delivery, Status spam.
	failureInvalidMessage = "invalid_message"
	// failureOther: none of the above (no caller reports one today).
	failureOther = "other"
)

var failureReasons = []string{failureStripeVerification, failureVerifierWithheld, failureFetchShortfall, failureInvalidMessage, failureOther}

// failureReason returns the peer_failures_total reason of a failure err, the
// most specific first: a shortfall that is a violation wraps both
// ErrFetchShortfall and ErrInvalidMessage.
func failureReason(err error) string {
	switch {
	case errors.Is(err, ErrStripeVerification):
		return failureStripeVerification
	case errors.Is(err, ErrVerifierWithheld):
		return failureVerifierWithheld
	case errors.Is(err, rangesync.ErrFetchShortfall):
		return failureFetchShortfall
	case errors.Is(err, rangesync.ErrInvalidMessage):
		return failureInvalidMessage
	default:
		return failureOther
	}
}

func newMetrics(syncRate, roundInterval func() float64, peers func() map[fsmState]int, initiatedSessionCap func() float64) metrics {
	const subsystem = "rangesync"

	mt := metrics{
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
		Failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_failures_total",
			Help:      "Failures reported to the peer score, by reason (stripe_verification, verifier_withheld, fetch_shortfall, invalid_message, other).",
		}, []string{"reason"}),
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
		StripeQueueWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stripe_queue_wait_seconds",
			Help:      "Wait of a stripe job in the stripe worker's queue, from its registration to its start, by kind (own, claim).",
			Buckets:   waitBuckets,
		}, []string{"kind"}),
		FirstChunk: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "first_chunk_seconds",
			Help:      "Time from the start of a stripe job (own, claim) or a catch-up fetch (catch_up) to the first chunk it stores.",
			Buckets:   waitBuckets,
		}, []string{"kind"}),
	}
	// every series is reported from the start, at 0
	for _, reason := range failureReasons {
		mt.Failures.WithLabelValues(reason)
	}
	for _, kind := range []string{jobOwn, jobClaim} {
		mt.StripeQueueWait.WithLabelValues(kind)
	}
	for _, kind := range []string{jobOwn, jobClaim, jobCatchUp} {
		mt.FirstChunk.WithLabelValues(kind)
	}
	return mt
}

func (m metrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.SyncRate, m.Sessions, m.Failures, m.Stripes, m.VerificationDiff, m.RoundInterval, m.Peers, m.InitiatedSessionCap, m.StripeQueueWait, m.FirstChunk}
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
