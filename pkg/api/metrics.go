// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"time"

	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	RequestCount     prometheus.Counter
	ResponseDuration prometheus.Histogram
	PingRequestCount prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "api"

	return metrics{
		RequestCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_count",
			Help:      "Number of API requests.",
		}),
		ResponseDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "response_duration_seconds",
			Help:      "Histogram of API response durations.",
			Buckets:   []float64{0.01, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
	}
}

func (s *server) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}

func (s *server) pageviewMetricsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		s.metrics.RequestCount.Inc()
		h.ServeHTTP(w, r)
		s.metrics.ResponseDuration.Observe(time.Since(start).Seconds())
	})
}
