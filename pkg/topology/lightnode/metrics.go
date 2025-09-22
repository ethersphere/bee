// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lightnode

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

// metrics groups lightnode related m counters.
type metrics struct {
	CurrentlyConnectedPeers    m.Gauge
	CurrentlyDisconnectedPeers m.Gauge
}

// newMetrics is a convenient constructor for creating new metrics.
func newMetrics() metrics {
	const subsystem = "lightnode"

	return metrics{
		CurrentlyConnectedPeers: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "currently_connected_peers",
			Help:      "Number of currently connected peers.",
		}),
		CurrentlyDisconnectedPeers: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "currently_disconnected_peers",
			Help:      "Number of currently disconnected peers.",
		})}
}

// Metrics returns set of m collectors.
func (c *Container) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(c.metrics)
}
