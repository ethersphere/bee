//go:build nometrics
// +build nometrics

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"io"
	"net/http"
)

type counterNoop struct{}

func (c counterNoop) Desc() *Desc              { return &Desc{} }
func (c counterNoop) Write(_ *MetricDTO) error { return nil }
func (c counterNoop) Describe(_ chan<- *Desc)  { return }
func (c counterNoop) Collect(_ chan<- Metric)  { return }
func (c counterNoop) Inc()                     { return }
func (c counterNoop) Add(_ float64)            { return }

func NewCounter(_ CounterOpts) Counter {
	return &counterNoop{}
}

var _ CounterMetricVector = (*counterVecNoop)(nil)

type counterVecNoop struct{}

func (c counterVecNoop) WithLabelValues(lvs ...string) Counter {
	return NewCounter(CounterOpts{})
}

func NewCounterVec(opts CounterOpts, names []string) CounterMetricVector {
	return &counterVecNoop{}
}

type gaugeNoop struct{}

func (g gaugeNoop) Desc() *Desc              { return &Desc{} }
func (g gaugeNoop) Write(_ *MetricDTO) error { return nil }
func (g gaugeNoop) Describe(_ chan<- *Desc)  { return }
func (g gaugeNoop) Collect(_ chan<- Metric)  { return }
func (g gaugeNoop) Set(_ float64)            { return }
func (g gaugeNoop) Inc()                     { return }
func (g gaugeNoop) Dec()                     { return }
func (g gaugeNoop) Add(_ float64)            { return }
func (g gaugeNoop) Sub(_ float64)            { return }
func (g gaugeNoop) SetToCurrentTime()        { return }

func NewGauge(_ GaugeOpts) Gauge {
	return &gaugeNoop{}
}

var _ GaugeMetricVector = (*gaugeVecNoop)(nil)

type gaugeVecNoop struct{}

func (g gaugeVecNoop) WithLabelValues(lvs ...string) Gauge {
	return NewGauge(GaugeOpts{})
}

func NewGaugeVec(opts GaugeOpts, names []string) GaugeMetricVector {
	return &gaugeVecNoop{}
}

var _ Histogram = (*histogramNoop)(nil)

type histogramNoop struct{}

func (h histogramNoop) Desc() *Desc              { return &Desc{} }
func (h histogramNoop) Write(_ *MetricDTO) error { return nil }
func (h histogramNoop) Describe(_ chan<- *Desc)  { return }
func (h histogramNoop) Collect(_ chan<- Metric)  { return }
func (h histogramNoop) Observe(_ float64)        { return }

func NewHistogram(_ HistogramOpts) Histogram {
	return &histogramNoop{}
}

var _ HistogramMetricVector = (*histogramVecNoop)(nil)

type histogramVecNoop struct{}

func (h histogramVecNoop) Describe(descs chan<- *Desc)   { return }
func (h histogramVecNoop) Collect(metrics chan<- Metric) { return }
func (h histogramVecNoop) WithLabelValues(lvs ...string) Observer {
	return NewHistogram(HistogramOpts{})
}

func NewHistogramVec(opts HistogramOpts, names []string) HistogramMetricVector {
	return &histogramVecNoop{}
}

var _ Summary = (*summaryNoop)(nil)

type summaryNoop struct{}

func (s summaryNoop) Desc() *Desc              { return &Desc{} }
func (s summaryNoop) Write(_ *MetricDTO) error { return nil }
func (s summaryNoop) Describe(_ chan<- *Desc)  { return }
func (s summaryNoop) Collect(_ chan<- Metric)  { return }
func (s summaryNoop) Observe(_ float64)        { return }

func NewSummary(_ SummaryOpts) Summary {
	return &summaryNoop{}
}

var _ Collector = (*collectorNoop)(nil)

type collectorNoop struct{}

func (c collectorNoop) Describe(_ chan<- *Desc) { return }
func (c collectorNoop) Collect(_ chan<- Metric) { return }

func NewGoCollector() Collector {
	return &collectorNoop{}
}

func NewProcessCollector(opts ProcessCollectorOpts) Collector {
	return &collectorNoop{}
}

var _ MetricsRegistererGatherer = (*registryNoop)(nil)

type registryNoop struct{}

func (r registryNoop) Register(collector Collector) error {
	return nil
}

func (r registryNoop) Unregister(collector Collector) bool {
	return true
}

func (r registryNoop) MustRegister(collector ...Collector) { return }
func (r registryNoop) Gather() ([]*MetricFamily, error) {
	return []*MetricFamily{}, nil
}

func NewRegistry() MetricsRegistererGatherer {
	return &registryNoop{}
}

func NewExporter(o ExporterOptions) error {
	return nil
}

func MustRegister(_ ...Collector) {
	// pass
}

func InstrumentMetricHandler(_ MetricsRegistererGatherer, h http.Handler) http.Handler {
	return h
}

func HandlerFor(_ MetricsRegistererGatherer, _ HandlerOpts) http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { return })
}

var _ Encoder = (*encoderNoop)(nil)

type encoderNoop struct{}

func (e encoderNoop) Encode(family *MetricFamily) error { return nil }

func NewEncoder(w io.Writer, format Format, options ...EncoderOption) Encoder {
	return &encoderNoop{}
}

func NewFormat(t FormatType) Format {
	return `<unknown>`
}
