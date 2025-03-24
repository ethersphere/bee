// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/status/internal/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestHistogram(t *testing.T) {
	h := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "test",
		Name:      "response_duration_seconds",
		Help:      "Histogram of API response durations.",
		Buckets:   []float64{0.01, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		ConstLabels: prometheus.Labels{
			"test": "label",
		},
	})

	points := []float64{0.25, 5.2, 1.5, 1, 5.2, 0, 65}
	var sum float64
	for _, p := range points {
		h.Observe(p)
		sum += p
	}

	m := new(dto.Metric)
	if err := h.Write(m); err != nil {
		t.Fatal(err)
	}

	got := status.NewHistogram(m)

	want := &pb.Histogram{
		Labels: []*pb.Label{
			{
				Name:  "test",
				Value: "label",
			},
		},
		SampleCount: uint64(len(points)),
		SampleSum:   sum,
		Buckets: []*pb.Histogram_Bucket{
			{CumulativeCount: 1, UpperBound: 0.01},
			{CumulativeCount: 1, UpperBound: 0.1},
			{CumulativeCount: 2, UpperBound: 0.25},
			{CumulativeCount: 2, UpperBound: 0.5},
			{CumulativeCount: 3, UpperBound: 1},
			{CumulativeCount: 4, UpperBound: 2.5},
			{CumulativeCount: 4, UpperBound: 5},
			{CumulativeCount: 6, UpperBound: 10},
		},
	}

	if !proto.Equal(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
