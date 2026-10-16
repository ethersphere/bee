// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
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
	// FetchSkipped counts IDs Fetch did not request, by reason.
	FetchSkipped *prometheus.CounterVec
	// Delivered counts fetched chunks that were stored.
	Delivered prometheus.Counter
	// DeliveryRejected counts fetched chunks whose stamp was rejected.
	DeliveryRejected prometheus.Counter
	// CrossProtocolDuplicates counts fetched chunks that were in the
	// reserve already when they would be stored.
	CrossProtocolDuplicates prometheus.Counter
}

func newMetrics() metrics {
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
			Help:      "Fetched chunks dropped because their stamp is invalid.",
		}),
		CrossProtocolDuplicates: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "cross_protocol_duplicates_total",
			Help:      "Fetched chunks already in the reserve when they would be stored.",
		}),
	}
}

func (mt metrics) collectors() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(mt)
}
