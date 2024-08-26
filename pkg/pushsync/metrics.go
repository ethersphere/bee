// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalSent               prometheus.Counter
	TotalReceived           prometheus.Counter
	TotalHandlerErrors      prometheus.Counter
	TotalRequests           prometheus.Counter
	TotalSendAttempts       prometheus.Counter
	TotalFailedSendAttempts prometheus.Counter
	TotalOutgoing           prometheus.Counter
	TotalOutgoingErrors     prometheus.Counter
	InvalidStampErrors      prometheus.Counter
	StampValidationTime     prometheus.HistogramVec
	Forwarder               prometheus.Counter
	Storer                  prometheus.Counter
	TotalHandlerTime        prometheus.HistogramVec
	PushToPeerTime          prometheus.HistogramVec

	ReceiptDepth        *prometheus.CounterVec
	ShallowReceiptDepth *prometheus.CounterVec
	ShallowReceipt      prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "pushsync"

	return metrics{
		TotalSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_sent",
			Help:      "Total chunks sent.",
		}),
		TotalReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_received",
			Help:      "Total chunks received.",
		}),
		TotalHandlerErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_handler_errors",
			Help:      "Total no of error occurred while handling an incoming delivery (either while storing or forwarding).",
		}),
		TotalRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_requests",
			Help:      "Total no of requests to push a chunk into the network (from origin nodes or not).",
		}),
		TotalSendAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_send_attempts",
			Help:      "Total no of attempts to push chunk.",
		}),
		TotalFailedSendAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_failed_send_attempts",
			Help:      "Total no of failed attempts to push chunk.",
		}),
		TotalOutgoing: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outgoing",
			Help:      "Total no of chunks requested to be synced (calls on exported PushChunkToClosest)",
		}),
		TotalOutgoingErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_outgoing_errors",
			Help:      "Total no of errors of entire operation to sync a chunk (multiple attempts included)",
		}),
		InvalidStampErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "invalid_stamps",
			Help:      "No of invalid stamp errors.",
		}),
		StampValidationTime: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "stamp_validation_time",
			Help:      "Time taken to validate stamps.",
		}, []string{"status"}),
		Forwarder: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "forwarder",
			Help:      "No of times the peer is a forwarder node.",
		}),
		Storer: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "storer",
			Help:      "No of times the peer is a storer node.",
		}),
		TotalHandlerTime: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "total_handler_time",
				Help:      "Histogram for time taken for the handler.",
			}, []string{"status"},
		),
		PushToPeerTime: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "push_peer_time",
				Help:      "Histogram for time taken to push a chunk to a peer.",
			}, []string{"status"},
		),
		ShallowReceiptDepth: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "shallow_receipt_depth",
				Help:      "Counter of shallow receipts received at different depths.",
			},
			[]string{"depth"},
		),
		ShallowReceipt: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "shallow_receipt",
			Help:      "Total shallow receipts.",
		}),
		ReceiptDepth: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "receipt_depth",
				Help:      "Counter of receipts received at different depths.",
			},
			[]string{"depth"},
		),
	}
}

func (s *PushSync) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
