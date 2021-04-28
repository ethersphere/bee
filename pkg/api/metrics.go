// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"time"

	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	RequestCount       prometheus.Counter
	ResponseDuration   prometheus.Histogram
	PingRequestCount   prometheus.Counter
	ResponseCodeCounts *prometheus.CounterVec
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
		ResponseCodeCounts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "response_code_count",
				Help:      "Response count grouped by status code",
			},
			[]string{"code", "method"},
		),
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

func (s *server) responseCodeMetricsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapper := newResponseWriter(w)
		h.ServeHTTP(wrapper, r)
		s.metrics.ResponseCodeCounts.WithLabelValues(
			fmt.Sprintf("%d", wrapper.statusCode),
			r.Method,
		).Inc()
	})
}

// UpgradedResponseWriter adds more functionality on top of ResponseWriter
type UpgradedResponseWriter interface {
	http.ResponseWriter
	http.Pusher
	http.Hijacker
	http.Flusher
	// staticcheck SA1019 CloseNotifier interface is required by gorilla compress handler
	// nolint:staticcheck
	http.CloseNotifier // skipcq: SCC-SA1019
}

type responseWriter struct {
	UpgradedResponseWriter
	statusCode  int
	wroteHeader bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	// StatusOK is called by default if nothing else is called
	uw := w.(UpgradedResponseWriter)
	return &responseWriter{uw, http.StatusOK, false}
}

func (rw *responseWriter) Status() int {
	return rw.statusCode
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.statusCode = code
	rw.UpgradedResponseWriter.WriteHeader(code)
	rw.wroteHeader = true
}
