// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics_test

import (
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestPrometheusCollectorsFromFields(t *testing.T) {
	s := newService()
	collectors := metrics.PrometheusCollectorsFromFields(s)

	if l := len(collectors); l != 2 {
		t.Fatalf("got %v collectors %+v, want 2", l, collectors)
	}

	m1 := collectors[0].(prometheus.Metric).Desc().String()
	if !strings.Contains(m1, "api_request_count") {
		t.Errorf("unexpected metric %s", m1)
	}

	m2 := collectors[1].(prometheus.Metric).Desc().String()
	if !strings.Contains(m2, "api_response_duration_seconds") {
		t.Errorf("unexpected metric %s", m2)
	}
}

type service struct {
	// valid metrics
	RequestCount     prometheus.Counter
	ResponseDuration prometheus.Histogram
	// invalid metrics
	unexportedCount    prometheus.Counter
	UninitializedCount prometheus.Counter
}

func newService() *service {
	subsystem := "api"
	return &service{
		RequestCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metrics.Namespace,
			Subsystem: subsystem,
			Name:      "request_count",
			Help:      "Number of API requests.",
		}),
		ResponseDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: metrics.Namespace,
			Subsystem: subsystem,
			Name:      "response_duration_seconds",
			Help:      "Histogram of API response durations.",
			Buckets:   []float64{0.01, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
		unexportedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metrics.Namespace,
			Subsystem: subsystem,
			Name:      "unexported_count",
			Help:      "This metrics should not be discoverable by metrics.PrometheusCollectorsFromFields.",
		}),
	}
}
