// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

// metrics groups handshake related prometheus counters.
type metrics struct {
	SynRx          m.Counter
	SynRxFailed    m.Counter
	SynAckTx       m.Counter
	SynAckTxFailed m.Counter
	AckRx          m.Counter
	AckRxFailed    m.Counter
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "handshake"

	return metrics{
		SynRx: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_rx",
			Help:      "The number of syn messages that were successfully read.",
		}),
		SynRxFailed: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_rx_failed",
			Help:      "The number of syn messages that were unsuccessfully read.",
		}),
		SynAckTx: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_ack_tx",
			Help:      "The number of syn-ack messages that were successfully written.",
		}),
		SynAckTxFailed: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "syn_ack_tx_failed",
			Help:      "The number of syn-ack messages that were unsuccessfully written.",
		}),
		AckRx: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ack_rx",
			Help:      "The number of ack messages that were successfully read.",
		}),
		AckRxFailed: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "ack_rx_failed",
			Help:      "The number of ack messages that were unsuccessfully read.",
		}),
	}
}

// Metrics returns set of prometheus collectors.
func (s *Service) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
