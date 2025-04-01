// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ethersphere/bee/v2"
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const bytesInKB = 1000

var fileSizeBucketsKBytes = []int64{100, 500, 2500, 4999, 5000, 10000}

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	RequestCount       prometheus.Counter
	ResponseDuration   prometheus.Histogram
	PingRequestCount   prometheus.Counter
	ResponseCodeCounts *prometheus.CounterVec

	ContentApiDuration *prometheus.HistogramVec
	UploadSpeed        *prometheus.HistogramVec
	DownloadSpeed      *prometheus.HistogramVec
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
		ContentApiDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "content_api_duration",
			Help:      "Histogram of file upload API response durations.",
			Buckets:   []float64{0.5, 1, 2.5, 5, 10, 30, 60},
		}, []string{"filesize", "method"}),
		UploadSpeed: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "upload_speed",
			Help:      "Histogram of upload speed in B/s.",
			Buckets:   []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3, 4, 5},
		}, []string{"endpoint", "mode"}),
		DownloadSpeed: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "download_speed",
			Help:      "Histogram of download speed in B/s.",
			Buckets:   []float64{0.5, 1, 1.5, 2, 2.5, 3, 4, 5, 6, 7, 8, 9},
		}, []string{"endpoint"}),
	}
}

func toFileSizeBucket(bytes int64) int64 {

	for _, s := range fileSizeBucketsKBytes {
		if (s * bytesInKB) >= bytes {
			return s * bytesInKB
		}
	}

	return fileSizeBucketsKBytes[len(fileSizeBucketsKBytes)-1] * bytesInKB
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}

// StatusMetrics exposes metrics that are exposed on the status protocol.
func (s *Service) StatusMetrics() []prometheus.Collector {
	return []prometheus.Collector{
		s.metrics.UploadSpeed,
		s.metrics.DownloadSpeed,
	}
}

func (s *Service) pageviewMetricsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		s.metrics.RequestCount.Inc()
		h.ServeHTTP(w, r)
		s.metrics.ResponseDuration.Observe(time.Since(start).Seconds())
	})
}

func (s *Service) responseCodeMetricsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapper := newResponseWriter(w)
		h.ServeHTTP(wrapper, r)
		s.metrics.ResponseCodeCounts.WithLabelValues(
			strconv.Itoa(wrapper.statusCode),
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
}

type responseWriter struct {
	UpgradedResponseWriter
	statusCode  int
	wroteHeader bool
	size        int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	// StatusOK is called by default if nothing else is called
	uw := w.(UpgradedResponseWriter)
	return &responseWriter{uw, http.StatusOK, false, 0}
}

func (rw *responseWriter) Status() int {
	return rw.statusCode
}

func (rr *responseWriter) Write(b []byte) (int, error) {
	size, err := rr.UpgradedResponseWriter.Write(b)
	rr.size += size
	return size, err
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.statusCode = code
	rw.UpgradedResponseWriter.WriteHeader(code)
	rw.wroteHeader = true
}

func newDebugMetrics() (r *prometheus.Registry) {
	r = prometheus.NewRegistry()

	// register standard metrics
	r.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
			Namespace: m.Namespace,
		}),
		collectors.NewGoCollector(),
		prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Name:      "info",
			Help:      "Bee information.",
			ConstLabels: prometheus.Labels{
				"version": bee.Version,
			},
		}),
	)

	return r
}

func (s *Service) MetricsRegistry() *prometheus.Registry {
	return s.metricsRegistry
}

func (s *Service) MustRegisterMetrics(cs ...prometheus.Collector) {
	s.metricsRegistry.MustRegister(cs...)
}
