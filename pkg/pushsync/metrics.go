// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection

	TotalChunksStoredInDB      prometheus.Counter
	ChunksSentCounter          prometheus.Counter
	ChunksReceivedCounter      prometheus.Counter
	SendChunkErrorCounter      prometheus.Counter
	ReceivedChunkErrorCounter  prometheus.Counter
	ReceiptsReceivedCounter    prometheus.Counter
	ReceiptsSentCounter        prometheus.Counter
	SendReceiptErrorCounter    prometheus.Counter
	ReceiveReceiptErrorCounter prometheus.Counter
	RetriesExhaustedCounter    prometheus.Counter
	InvalidReceiptReceived     prometheus.Counter
	SendChunkTimer             prometheus.Histogram
	ReceiptRTT                 prometheus.Histogram
}

func newMetrics() metrics {
	subsystem := "pushsync"

	return metrics{
		TotalChunksStoredInDB: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_chunk_stored_in_DB",
			Help:      "Total chunks stored succesfully in local store.",
		}),
		ChunksSentCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sent_chunk",
			Help:      "Total chunks sent.",
		}),
		ChunksReceivedCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "received_chunk",
			Help:      "Total chunks received.",
		}),
		SendChunkErrorCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "send_chunk_error",
			Help:      "Total no of time error received while sending chunk.",
		}),
		ReceivedChunkErrorCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "received_chunk_error",
			Help:      "Total no of time error received while receiving chunk.",
		}),
		ReceiptsReceivedCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "received_receipts",
			Help:      "Total no of times receipts received.",
		}),
		ReceiptsSentCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sent_receipts",
			Help:      "Total no of times receipts are sent.",
		}),
		SendReceiptErrorCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sent_receipts_error",
			Help:      "Total no of times receipts were sent and error was encountered.",
		}),
		ReceiveReceiptErrorCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "receive_receipt_error",
			Help:      "Total no of time error received while receiving receipt.",
		}),
		RetriesExhaustedCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunk_retries_exhausted",
			Help:      "CHunk retries exhausted.",
		}),
		InvalidReceiptReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_receipt_receipt",
			Help:      "Invalid receipt received from peer.",
		}),
		SendChunkTimer: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "send_chunk_time_histogram",
			Help:      "Histogram for Time taken to send a chunk.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
		}),
		ReceiptRTT: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "receipt_rtt_histogram",
			Help:      "Histogram of RTT for receiving receipt for a pushed chunk.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 60},
		}),
	}
}

func (s *PushSync) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
