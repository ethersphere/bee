// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !nometrics
// +build !nometrics

package metrics

import (
	"fmt"
	"io"
	"net/http"

	exporter "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
)

func NewCounter(opts CounterOpts) Counter {
	return prometheus.NewCounter(opts)
}

func NewEncoder(w io.Writer, format expfmt.Format, options ...expfmt.EncoderOption) expfmt.Encoder {
	return expfmt.NewEncoder(w, format, options...)
}

func NewFormat(t expfmt.FormatType) expfmt.Format {
	return expfmt.NewFormat(t)
}

func NewGoCollector() Collector {
	return collectors.NewGoCollector()
}

func NewGaugeVec(opts GaugeOpts, names []string) GaugeMetricVector {
	return prometheus.NewGaugeVec(opts, names)
}
func NewGauge(opts GaugeOpts) Gauge {
	return prometheus.NewGauge(opts)
}

func NewHistogram(opts HistogramOpts) Histogram {
	return prometheus.NewHistogram(opts)
}

func NewHistogramVec(opts HistogramOpts, names []string) HistogramMetricVector {
	return prometheus.NewHistogramVec(opts, names)
}

func NewCounterVec(opts CounterOpts, names []string) CounterMetricVector {
	return prometheus.NewCounterVec(opts, names)
}

func NewRegistry() MetricsRegistererGatherer {
	return prometheus.NewRegistry()
}

func NewProcessCollector(opts ProcessCollectorOpts) Collector {
	return collectors.NewProcessCollector(opts)
}

func NewSummary(opts SummaryOpts) Summary {
	return prometheus.NewSummary(opts)
}

func InstrumentMetricHandler(reg MetricsRegistererGatherer, handler http.Handler) http.Handler {
	return promhttp.InstrumentMetricHandler(reg, handler)
}

func HandlerFor(reg MetricsRegistererGatherer, opts HandlerOpts) http.Handler {
	return promhttp.HandlerFor(reg, opts)
}

func NewExporter(o ExporterOptions) error {
	if o.Registry == nil {
		return ErrNilRegistry
	}
	r, ok := o.Registry.(*prometheus.Registry)
	if !ok {
		return fmt.Errorf("invalid exporter type: %T", o.Registry)
	}
	opts := exporter.Options{
		Namespace: o.Namespace,
		Registry:  r,
	}
	_, err := exporter.NewExporter(opts)
	return err
}

func MustRegister(cs ...Collector) {
	prometheus.MustRegister(cs...)
}
