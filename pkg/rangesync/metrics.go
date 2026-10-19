// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"github.com/prometheus/client_golang/prometheus"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
)

// metrics are the protocol metrics of the rangesync service.
type metrics struct {
	// MappingMismatch counts received statuses whose Mapping differs from
	// this node's.
	MappingMismatch prometheus.Counter
	// InvalidMessage counts peers disconnected for breaking the protocol.
	InvalidMessage prometheus.Counter
	// Busy counts sessions answered with Busy.
	Busy prometheus.Counter
	// StreamLimitExceeded counts peers disconnected for opening too many
	// concurrent streams.
	StreamLimitExceeded prometheus.Counter
	// StatusRateLimited counts received statuses dropped because the peer
	// sent them over its rate.
	StatusRateLimited prometheus.Counter
	// FetchSkipped counts IDs Fetch did not request, by reason.
	FetchSkipped *prometheus.CounterVec
	// Delivered counts fetched chunks that were stored.
	Delivered prometheus.Counter
	// DeliveryRejected counts fetched chunks the stamp validator failed:
	// rejected (the stamp is invalid) or deferred (another cause).
	DeliveryRejected prometheus.Counter
	// CrossProtocolDuplicates counts fetched chunks that were in the
	// reserve already when they would be stored.
	CrossProtocolDuplicates prometheus.Counter
	// SessionRounds observes the number of Messages this side handled in
	// each completed reconcile session.
	SessionRounds prometheus.Histogram
	// PairwiseDiff observes, per completed reconcile session, the number of
	// IDs the peer listed that this node does not hold (its side of n_Δ;
	// the two sides of a session together give n_Δ).
	PairwiseDiff prometheus.Histogram
	// DecodeCorruption counts the peer's symbol streams the decoder found
	// corrupt; their ranges fall back to an exact item exchange.
	DecodeCorruption prometheus.Counter
	// SymbolsPerDifference observes, per decoded symbol stream with at
	// least one difference, the symbols received (overshoot included) per
	// difference recovered.
	SymbolsPerDifference prometheus.Histogram
	// MoreRequests counts the More parts sent.
	MoreRequests prometheus.Counter
	// StreamsExhausted counts the peer's symbol streams that could not grow
	// before they decoded, by level of the range.
	StreamsExhausted *prometheus.CounterVec
	// PartsSent and PartsReceived count the parts of reconcile Messages, by
	// type.
	PartsSent     *prometheus.CounterVec
	PartsReceived *prometheus.CounterVec
	// BytesMetadata counts the wire bytes of Status, reconcile and Want
	// messages, by direction.
	BytesMetadata *prometheus.CounterVec
	// BytesChunks counts the wire bytes of Delivery messages, by direction.
	BytesChunks *prometheus.CounterVec
	// WantClaims counts the outcomes of the want set for the IDs Fetch
	// would request (§10 want_claims): the claim results (granted, denied,
	// peer_full, full, rejected, closed), and, for granted claims that
	// expired before their fetch ended them, expired, and late for those of
	// them whose chunk was delivered anyway (late is a subset of expired).
	WantClaims *prometheus.CounterVec
	// RejectedCacheSize is the number of IDs in the want set's rejected
	// cache (§10 rejected_cache_size).
	RejectedCacheSize prometheus.GaugeFunc
}

// Label values of the parts and bytes metrics.
const (
	dirIn  = "in"
	dirOut = "out"

	partFingerprint = "fingerprint"
	partItems       = "items"
	partSymbols     = "symbols"
	partMore        = "more"
	partUnknown     = "unknown"
)

// Label values of the want_claims metric.
const (
	claimGranted  = "granted"
	claimDenied   = "denied"
	claimPeerFull = "peer_full"
	claimFull     = "full"
	claimRejected = "rejected"
	claimClosed   = "closed"
	claimExpired  = "expired"
	claimLate     = "late"
)

// claimLabel returns the want_claims label of the claim result r.
func claimLabel(r wantset.Result) string {
	switch r {
	case wantset.Granted:
		return claimGranted
	case wantset.Held:
		return claimDenied
	case wantset.PeerFull:
		return claimPeerFull
	case wantset.Full:
		return claimFull
	case wantset.Rejected:
		return claimRejected
	case wantset.Closed:
		return claimClosed
	default:
		return r.String()
	}
}

// newMetrics returns the metrics; rejected reports the size of the rejected
// cache.
func newMetrics(rejected func() float64) metrics {
	const subsystem = "rangesync"

	return metrics{
		MappingMismatch: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "mapping_mismatch_total",
			Help:      "Received statuses whose symbol mapping differs from this node's.",
		}),
		InvalidMessage: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_message_total",
			Help:      "Peers disconnected for sending an invalid message.",
		}),
		Busy: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "session_busy_total",
			Help:      "Reconcile sessions answered with Busy.",
		}),
		StreamLimitExceeded: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stream_limit_exceeded_total",
			Help:      "Peers disconnected for exceeding the concurrent stream limit.",
		}),
		StatusRateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "status_rate_limited_total",
			Help:      "Received statuses dropped because the peer sent them over its rate.",
		}),
		FetchSkipped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fetch_skipped_total",
			Help:      "IDs not requested by fetch, by reason.",
		}, []string{"reason"}),
		Delivered: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fetch_delivered_total",
			Help:      "Fetched chunks stored in the reserve.",
		}),
		DeliveryRejected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fetch_rejected_total",
			Help:      "Fetched chunks dropped because the stamp validator failed.",
		}),
		CrossProtocolDuplicates: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cross_protocol_duplicates_total",
			Help:      "Fetched chunks already in the reserve when they would be stored.",
		}),
		SessionRounds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "session_rounds",
			Help:      "Messages handled by this side per completed reconcile session.",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12),
		}),
		PairwiseDiff: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pairwise_diff_half",
			Help:      "IDs listed by the peer and missing locally per completed reconcile session (this side's half of the session difference).",
			Buckets:   append([]float64{0}, prometheus.ExponentialBuckets(1, 4, 10)...),
		}),
		DecodeCorruption: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "decode_corruption_total",
			Help:      "Symbol streams of peers that decoded corrupt and fell back to an item exchange.",
		}),
		SymbolsPerDifference: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "symbols_per_difference",
			Help:      "Coded symbols received per difference recovered, per decoded symbol stream with at least one difference.",
			Buckets:   prometheus.LinearBuckets(1, 0.25, 17),
		}),
		MoreRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "more_requests_total",
			Help:      "More requests sent for the peer's symbol streams.",
		}),
		StreamsExhausted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "streams_exhausted_total",
			Help:      "Symbol streams of peers that ended before they decoded, by level of the range.",
		}, []string{"level"}),
		PartsSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "parts_sent",
			Help:      "Parts of reconcile messages sent, by type.",
		}, []string{"type"}),
		PartsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "parts_received",
			Help:      "Parts of reconcile messages received, by type.",
		}, []string{"type"}),
		BytesMetadata: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "bytes_metadata",
			Help:      "Wire bytes of status, reconcile and want messages, by direction.",
		}, []string{"dir"}),
		BytesChunks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "bytes_chunks",
			Help:      "Wire bytes of chunk deliveries, by direction.",
		}, []string{"dir"}),
		WantClaims: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "want_claims",
			Help:      "Want set outcomes of the IDs fetch would request: claim results (granted, denied, peer_full, full, rejected, closed), and granted claims that expired before their fetch ended them (expired; late: delivered anyway, a subset of expired).",
		}, []string{"result"}),
		RejectedCacheSize: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "rejected_cache_size",
			Help:      "IDs skipped by fetch for RejectTTL because their stamp was rejected or older than the stored one.",
		}, rejected),
	}
}

func (mt metrics) collectors() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(mt)
}

// wireSize returns the size of m on the wire: its encoding and the varint
// length prefix.
func wireSize(m interface{ Size() int }) int {
	n := m.Size()
	return n + uvarintLen(uint64(n))
}

func uvarintLen(x uint64) int {
	n := 1
	for ; x >= 0x80; x >>= 7 {
		n++
	}
	return n
}

// partType returns the parts metric label of p.
func partType(p *pb.Part) string {
	switch p.GetKind().(type) {
	case *pb.Part_FP:
		return partFingerprint
	case *pb.Part_Items:
		return partItems
	case *pb.Part_Symbols:
		return partSymbols
	case *pb.Part_More:
		return partMore
	default:
		return partUnknown
	}
}

// reconcileMessage counts the parts and bytes of a reconcile Message sent
// (dir dirOut) or received (dirIn).
func (mt metrics) reconcileMessage(dir string, msg *pb.Message) {
	parts := mt.PartsReceived
	if dir == dirOut {
		parts = mt.PartsSent
	}
	for _, p := range msg.GetParts() {
		parts.WithLabelValues(partType(p)).Inc()
	}
	mt.BytesMetadata.WithLabelValues(dir).Add(float64(wireSize(msg)))
}

// sessionStats counts the symbol-stream outcomes of a session.
func (mt metrics) sessionStats(st SessionStats) {
	mt.DecodeCorruption.Add(float64(st.DecodeCorruptions))
	mt.MoreRequests.Add(float64(st.MoreRequests))
	for l, n := range st.ExhaustedByLevel {
		if n > 0 {
			mt.StreamsExhausted.WithLabelValues(levelNames[l]).Add(float64(n))
		}
	}
	for _, v := range st.SymbolsPerDifference {
		mt.SymbolsPerDifference.Observe(v)
	}
}

// sessionDone observes a completed session: the Messages handled and the
// IDs the peer listed that this node lacks.
func (mt metrics) sessionDone(handled, wants int) {
	mt.SessionRounds.Observe(float64(handled))
	mt.PairwiseDiff.Observe(float64(wants))
}
