// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netstore

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	LocalChunksCounter        prometheus.Counter
	InvalidLocalChunksCounter prometheus.Counter
	RetrievedChunksCounter    prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "netstore"

	return metrics{
		LocalChunksCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "local_chunks_retrieved",
			Help:      "Total no. of chunks retrieved locally.",
		}),
		InvalidLocalChunksCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_local_chunks_retrieved",
			Help:      "Total no. of chunks retrieved locally that are invalid.",
		}),
		RetrievedChunksCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_retrieved_from_network",
			Help:      "Total no. of chunks retrieved from network.",
		}),
	}
}

func (s *store) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
