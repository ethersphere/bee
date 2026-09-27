// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics groups handshake related prometheus counters.
type metrics struct {
	SynRx                          prometheus.Counter
	SynRxFailed                    prometheus.Counter
	SynAckTx                       prometheus.Counter
	SynAckTxFailed                 prometheus.Counter
	AckRx                          prometheus.Counter
	AckRxFailed                    prometheus.Counter
	AdvertisableUnderlaysTruncated prometheus.Counter
	ObservedUnderlaysTruncated     prometheus.Counter
	AddressMinted                  prometheus.Counter
	TimestampRejected              *prometheus.CounterVec
	ChequebookVerification         *prometheus.CounterVec
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "handshake"

	return metrics{
		SynRx: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_rx",
			Help:      "The number of syn messages that were successfully read.",
		}),
		SynRxFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_rx_failed",
			Help:      "The number of syn messages that were unsuccessfully read.",
		}),
		SynAckTx: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_ack_tx",
			Help:      "The number of syn-ack messages that were successfully written.",
		}),
		SynAckTxFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_ack_tx_failed",
			Help:      "The number of syn-ack messages that were unsuccessfully written.",
		}),
		AckRx: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ack_rx",
			Help:      "The number of ack messages that were successfully read.",
		}),
		AckRxFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ack_rx_failed",
			Help:      "The number of ack messages that were unsuccessfully read.",
		}),
		AdvertisableUnderlaysTruncated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "advertisable_underlays_truncated",
			Help:      "Number of times own advertisable underlays were truncated before signing.",
		}),
		ObservedUnderlaysTruncated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "observed_underlays_truncated",
			Help:      "Number of times observed peer underlays were truncated before sending.",
		}),
		AddressMinted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "address_minted_total",
			Help:      "Number of session-stable signed addresses minted. Plateaus per session; linear growth indicates advertised-underlay churn.",
		}),
		TimestampRejected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "timestamp_rejected_total",
				Help:      "Number of handshake ack messages rejected by timestamp validation. The 'reason' label is one of: invalid, in_future, stale.",
			},
			[]string{"reason"},
		),
		ChequebookVerification: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "chequebook_verification_total",
				Help:      "Outcomes of chequebook verification during handshake. The 'result' label is one of: success, missing, issuer_mismatch, bytecode_mismatch, insufficient_balance, already_associated, verify_error.",
			},
			[]string{"result"},
		),
	}
}

// Metrics returns set of prometheus collectors.
func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
