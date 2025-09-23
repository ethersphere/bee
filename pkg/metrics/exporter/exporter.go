// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exporter

import (
	exporter "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsSourcer interface {
	MustRegister(...prometheus.Collector)
	Register(prometheus.Collector) error
	Unregister(prometheus.Collector) bool
}

// The goroutine leak `go.opencensus.io/stats/view.(*worker).start` appears solely because of
// importing the `exporter.Options` type from `go.opencensus.io` package
// (either directly or transitively).
type ExporterOptions struct {
	Namespace string
	Registry  MetricsSourcer
}

func NewExporter(o ExporterOptions) error {
	r, _ := o.Registry.(*prometheus.Registry)
	opts := exporter.Options{
		Namespace: o.Namespace,
		Registry:  r,
	}
	_, err := exporter.NewExporter(opts)
	return err
}
