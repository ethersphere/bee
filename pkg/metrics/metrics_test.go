// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !nometrics
// +build !nometrics

package metrics_test

import (
	"strings"
	"testing"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

func TestPrometheusCollectorsFromFields(t *testing.T) {
	t.Parallel()

	s := newService()
	collectors := m.PrometheusCollectorsFromFields(s)

	if l := len(collectors); l != 2 {
		t.Fatalf("got %v collectors %+v, want 2", l, collectors)
	}

	m1 := collectors[0].(m.Metric).Desc().String()
	if !strings.Contains(m1, "api_request_count") {
		t.Errorf("unexpected metric %s", m1)
	}

	m2 := collectors[1].(m.Metric).Desc().String()
	if !strings.Contains(m2, "api_response_duration_seconds") {
		t.Errorf("unexpected metric %s", m2)
	}
}

type service struct {
	// valid metrics
	RequestCount     m.Counter
	ResponseDuration m.Histogram
	// invalid metrics
	unexportedCount    m.Counter
	UninitializedCount m.Counter
}

func newService() *service {
	subsystem := "api"
	return &service{
		RequestCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "request_count",
			Help:      "Number of API requests.",
		}),
		ResponseDuration: m.NewHistogram(m.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "response_duration_seconds",
			Help:      "Histogram of API response durations.",
			Buckets:   []float64{0.01, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),
		unexportedCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "unexported_count",
			Help:      "This metrics should not be discoverable by metrics.PrometheusCollectorsFromFields.",
		}),
	}
}
