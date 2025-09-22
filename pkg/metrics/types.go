// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type MetricsError string

func (e MetricsError) Error() string {
	return string(e)
}

const (
	// Namespace is prefixed before every metric. If it is changed, it must be done
	// before any metrics collector is registered.
	Namespace = "bee"

	TypeTextPlain = expfmt.TypeTextPlain

	ErrNilRegistry = MetricsError("nil registry")
)

// Prometheus Vector type interfaces
type (
	MetricsCollector interface {
		Metrics() []prometheus.Collector
	}

	GaugeMetricVector interface {
		WithLabelValues(lvs ...string) Gauge
	}

	HistogramMetricVector interface {
		WithLabelValues(lvs ...string) Observer
		Describe(chan<- *Desc)
		Collect(chan<- Metric)
	}

	CounterMetricVector interface {
		WithLabelValues(lvs ...string) Counter
	}

	MetricsRegistererGatherer interface {
		Gather() ([]*MetricFamily, error)
		MetricsRegisterer
	}

	MetricsRegisterer interface {
		MustRegister(...Collector)
		Register(Collector) error
		Unregister(Collector) bool
	}
)

// Prometheus types aliases
type (
	Collector = prometheus.Collector
	Registry  = prometheus.Registry
	Observer  = prometheus.Observer
	Labels    = prometheus.Labels
	Metric    = prometheus.Metric
	Desc      = prometheus.Desc

	Counter     = prometheus.Counter
	CounterOpts = prometheus.CounterOpts
	CounterVec  = CounterMetricVector

	Gauge        = prometheus.Gauge
	GaugeVec     = GaugeMetricVector
	GaugeVecOpts = prometheus.GaugeVecOpts
	GaugeOpts    = prometheus.GaugeOpts

	Histogram        = prometheus.Histogram
	HistogramOpts    = prometheus.HistogramOpts
	HistogramVec     = HistogramMetricVector
	HistogramVecOpts = prometheus.HistogramVecOpts

	Summary     = prometheus.Summary
	SummaryOpts = prometheus.SummaryOpts
	SummaryVec  = prometheus.SummaryVec

	ProcessCollectorOpts = collectors.ProcessCollectorOpts
	HandlerOpts          = promhttp.HandlerOpts

	MetricDTO    = dto.Metric
	MetricFamily = dto.MetricFamily

	Encoder       = expfmt.Encoder
	Format        = expfmt.Format
	FormatType    = expfmt.FormatType
	EncoderOption = expfmt.EncoderOption
)

// The goroutine leak `go.opencensus.io/stats/view.(*worker).start` appears solely because of
// importing the `exporter.Options` type from `go.opencensus.io` package
// (either directly or transitively).
type ExporterOptions struct {
	Namespace string
	Registry  MetricsRegistererGatherer
}
