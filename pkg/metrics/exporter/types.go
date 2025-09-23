// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsSourcer interface {
	MustRegister(...prometheus.Collector)
	Register(prometheus.Collector) error
	Unregister(prometheus.Collector) bool
}

type ExporterOptions struct {
	Namespace string
	Registry  MetricsSourcer
}
