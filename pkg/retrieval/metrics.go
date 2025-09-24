// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection

	RequestCounter        m.Counter
	RequestSuccessCounter m.Counter
	RequestFailureCounter m.Counter
	RequestDurationTime   m.Histogram
	RequestAttempts       m.Histogram
	PeerRequestCounter    m.Counter
	TotalRetrieved        m.Counter
	InvalidChunkRetrieved m.Counter
	ChunkPrice            m.Summary
	TotalErrors           m.Counter
	ChunkRetrieveTime     m.Histogram
}

func newMetrics() metrics {
	subsystem := "retrieval"

	return metrics{
		RequestCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_count",
			Help:      "Number of requests to retrieve chunks.",
		}),
		RequestSuccessCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_success_count",
			Help:      "Number of requests which succeeded to retrieve chunk.",
		}),
		RequestFailureCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_failure_count",
			Help:      "Number of requests which failed to retrieve chunk.",
		}),
		RequestDurationTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_duration_time",
			Help:      "Histogram for time taken to complete retrieval request",
		}),
		RequestAttempts: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_attempts",
			Help:      "Histogram for total retrieval attempts pre each request.",
		}),
		PeerRequestCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "peer_request_count",
			Help:      "Number of request to single peer.",
		}),
		TotalRetrieved: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_retrieved",
			Help:      "Total chunks retrieved.",
		}),
		InvalidChunkRetrieved: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_chunk_retrieved",
			Help:      "Invalid chunk retrieved from peer.",
		}),
		ChunkPrice: m.NewSummary(m.SummaryOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunk_price",
			Help:      "The price of the chunk that was paid.",
		}),
		TotalErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_errors",
			Help:      "Total number of errors while retrieving chunk.",
		}),
		ChunkRetrieveTime: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "retrieve_chunk_time",
			Help:      "Histogram for time taken to retrieve a chunk.",
		},
		),
	}
}

func (s *Service) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}

// StatusMetrics exposes metrics that are exposed on the status protocol.
func (s *Service) StatusMetrics() []m.Collector {
	return []m.Collector{
		s.metrics.RequestAttempts,
		s.metrics.ChunkRetrieveTime,
		s.metrics.RequestDurationTime,
	}
}
