// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	TotalReceivedPseudoSettlements  m.Counter
	TotalSentPseudoSettlements      m.Counter
	ReceivedPseudoSettlements       m.Counter
	SentPseudoSettlements           m.Counter
	ReceivedPseudoSettlementsErrors m.Counter
	SentPseudoSettlementsErrors     m.Counter
}

func newMetrics() metrics {
	subsystem := "pseudosettle"

	return metrics{
		TotalReceivedPseudoSettlements: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_received_pseudosettlements",
			Help:      "Amount of time settlements received from peers (income of the node)",
		}),
		TotalSentPseudoSettlements: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_sent_pseudosettlements",
			Help:      "Amount of  of time settlements sent to peers (costs paid by the node)",
		}),
		ReceivedPseudoSettlements: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "received_pseudosettlements",
			Help:      "Number of time settlements received from peers",
		}),
		SentPseudoSettlements: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sent_pseudosettlements",
			Help:      "Number of time settlements sent to peers",
		}),
		ReceivedPseudoSettlementsErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "received_pseudosettlements_errors",
			Help:      "Errors of time settlements received from peers",
		}),
		SentPseudoSettlementsErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sent_pseudosettlements_errorss",
			Help:      "Errors of time settlements sent to peers",
		}),
	}
}

func (s *Service) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
