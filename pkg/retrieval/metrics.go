// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"github.com/prometheus/client_golang/prometheus"

	m "github.com/ethersphere/bee/pkg/metrics"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection

	RequestCounter             prometheus.Counter
	PeerRequestCounter         prometheus.Counter
	TotalRetrieved             prometheus.Counter
	InvalidChunkRetrieved      prometheus.Counter
	RetrieveChunkPeerPOTimer   prometheus.HistogramVec
	RetrieveChunkPOGainCounter prometheus.CounterVec
	ChunkPrice                 prometheus.Summary
	TotalErrors                prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "retrieval"

	return metrics{
		RequestCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_count",
			Help:      "Number of requests to retrieve chunks.",
		}),
		PeerRequestCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_request_count",
			Help:      "Number of request to single peer.",
		}),
		TotalRetrieved: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_retrieved",
			Help:      "Total chunks retrieved.",
		}),
		InvalidChunkRetrieved: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_chunk_retrieved",
			Help:      "Invalid chunk retrieved from peer.",
		}),
		RetrieveChunkPeerPOTimer: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "retrieve_po_time",
				Help:      "Histogram for time taken to retrieve a chunk per PO.",
				Buckets:   []float64{0.01, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"po"},
		),
		RetrieveChunkPOGainCounter: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "chunk_po_gain_count",
				Help:      "Counter of chunk retrieval requests per address PO hop distance.",
			},
			[]string{"gain"},
		),
		ChunkPrice: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunk_price",
			Help:      "The price of the chunk that was paid.",
		}),
		TotalErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_errors",
			Help:      "Total number of errors while retrieving chunk.",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
