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
				// middle range should be more infrequent (because of addressbook)
				Buckets: []float64{10, 12, 14, 16, 18, 20, 30, 45, 60, 90, 120, 180, 240, 300, 400, 420, 440, 460, 480, 550, 600},
			},
		),
		FullSyncDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: metrics.Namespace,
				Subsystem: subsystem,
				Name:      "full_sync_duration_minutes",
				Help:      "Duration in minutes for node full sync to complete",
				// middle range should be more frequent
				Buckets: []float64{80, 90, 100, 110,
					120, 125, 130, 135, 140, 145, 150, 155, 160, 165, 170, 175, 180, // 2-3 hours range
					190, 200, 210, 220, 230, 240},
			},
		),
	}
}

// RegisterMetrics registers all metrics from the package
func (m nodeMetrics) RegisterMetrics() {
	prometheus.MustRegister(m.WarmupDuration)
	prometheus.MustRegister(m.FullSyncDuration)
}
