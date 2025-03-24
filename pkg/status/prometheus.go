// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status

import (
	"github.com/ethersphere/bee/v2/pkg/status/internal/pb"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func newHistogram(m *dto.Metric) *pb.Histogram {
	if m == nil {
		return nil
	}
	if m.Histogram == nil {
		return nil
	}
	return &pb.Histogram{
		Labels:      newLabels(m.Label),
		SampleSum:   m.Histogram.GetSampleSum(),
		SampleCount: m.Histogram.GetSampleCount(),
		Buckets:     newHistogramBuckets(m.Histogram.Bucket),
	}
}

func newLabels(labels []*dto.LabelPair) []*pb.Label {
	out := make([]*pb.Label, 0, len(labels))
	for _, l := range labels {
		out = append(out, &pb.Label{
			Name:  l.GetName(),
			Value: l.GetValue(),
		})
	}
	return out
}

func newHistogramBuckets(buckets []*dto.Bucket) []*pb.Histogram_Bucket {
	out := make([]*pb.Histogram_Bucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, &pb.Histogram_Bucket{
			CumulativeCount: b.GetCumulativeCount(),
			UpperBound:      b.GetUpperBound(),
		})
	}
	return out
}

func getMetric(m prometheus.Metric) (*dto.Metric, error) {
	if m == nil {
		return nil, nil
	}

	out := new(dto.Metric)
	if err := m.Write(out); err != nil {
		return nil, err
	}

	return out, nil
}
