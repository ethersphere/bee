// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type nodeMetrics struct {
	// WarmupDuration measures time in seconds for the node warmup to complete
	WarmupDuration prometheus.Histogram
	// FullSyncDuration measures time in seconds for the full sync to complete
	FullSyncDuration prometheus.Histogram
	// IsFullySynced is 1 while the node passes the sync check that the
	// storage incentives agent reads (isFullySynced), 0 otherwise, read
	// when collected. It reads 0 until a full node sets the check
	// (setFullySynced), and always on light and boot nodes, which have
	// none.
	IsFullySynced prometheus.GaugeFunc

	// fullySynced is the sync check IsFullySynced reads.
	fullySynced *syncCheck
}

// syncCheck holds the node's sync check, once set.
type syncCheck struct {
	fn atomic.Pointer[func() bool]
}

// value returns 1 when the check is set and passes, else 0.
func (c *syncCheck) value() float64 {
	if fn := c.fn.Load(); fn != nil && (*fn)() {
		return 1
	}
	return 0
}

// setFullySynced sets the sync check that IsFullySynced reads.
func (m nodeMetrics) setFullySynced(fn func() bool) {
	m.fullySynced.fn.Store(&fn)
}

func newMetrics() nodeMetrics {
	subsystem := "init"
	fullySynced := new(syncCheck)

	return nodeMetrics{
		WarmupDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: metrics.Namespace,
				Subsystem: subsystem,
				Name:      "warmup_duration_seconds",
				Help:      "Duration in seconds for node warmup to complete",
				// middle range should be more infrequent (because of addressbook)
				Buckets: []float64{10, 20, 25, 30, 35, 40, 45, 50, 60, 70, 90, 120, 180, 240, 300, 350, 380, 400, 420, 440, 460, 480, 550, 600},
			},
		),
		FullSyncDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: metrics.Namespace,
				Subsystem: subsystem,
				Name:      "full_sync_duration_minutes",
				Help:      "Duration in minutes for node full sync to complete",
				// middle range should be more frequent
				Buckets: []float64{
					80, 90, 100, 110,
					120, 125, 130, 135, 140, 145, 150, 155, 160, 165, 170, 175, 180, // 2-3 hours range
					190, 200, 210, 220, 230, 240,
				},
			},
		),
		IsFullySynced: prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: metrics.Namespace,
				Subsystem: "node",
				Name:      "is_fully_synced",
				Help:      "1 while the node passes the sync check of the storage incentives agent (reserve at least half full, sync rate 0, topology stabilized), else 0; 0 on light and boot nodes.",
			},
			fullySynced.value,
		),
		fullySynced: fullySynced,
	}
}

func getMetrics(nodeMetrics nodeMetrics) []prometheus.Collector {
	return metrics.PrometheusCollectorsFromFields(nodeMetrics)
}
