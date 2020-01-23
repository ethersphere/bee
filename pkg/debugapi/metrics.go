// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"github.com/janos/bee"
	"github.com/prometheus/client_golang/prometheus"
)

func newMetricsRegistry() (r *prometheus.Registry) {
	r = prometheus.NewRegistry()

	// register standard metrics
	r.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
		prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bee_info",
			Help: "Bee information.",
			ConstLabels: prometheus.Labels{
				"version": bee.Version,
			},
		}),
	)

	return r
}

func (s *server) MustRegisterMetrics(cs ...prometheus.Collector) {
	s.metricsRegistry.MustRegister(cs...)
}
