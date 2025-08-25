// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type nodeMetrics struct {
	// WarmupDuration measures time in seconds for the node warmup to complete
	WarmupDuration prometheus.Histogram
	// FullSyncDuration measures time in seconds for the full sync to complete
	FullSyncDuration prometheus.Histogram
}

func newMetrics() nodeMetrics {
	subsystem := "init"

	return nodeMetrics{
		WarmupDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: metrics.Namespace,
				Subsystem: subsystem,
				Name:      "warmup_duration_seconds",
				Help:      "Duration in seconds for node warmup to complete",
			},
		),
		FullSyncDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: metrics.Namespace,
				Subsystem: subsystem,
				Name:      "full_sync_duration_seconds",
				Help:      "Duration in seconds for node warmup to complete",
			},
		),
	}
}

func Metrics(nodeMetrics nodeMetrics) []prometheus.Collector {
	return metrics.PrometheusCollectorsFromFields(nodeMetrics)
}
