// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	TotalReceivedPseudoSettlements prometheus.Counter
	TotalSentPseudoSettlements     prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "pseudosettle"

	return metrics{
		TotalReceivedPseudoSettlements: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_received_pseudosettlements",
			Help:      "Amount of pseudotokens received from peers (income of the node)",
		}),
		TotalSentPseudoSettlements: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_sent_pseudosettlements",
			Help:      "Amount of pseudotokens sent to peers (costs paid by the node)",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
