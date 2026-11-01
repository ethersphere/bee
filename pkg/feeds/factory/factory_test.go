// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/feeds/factory"
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

func TestFactoryMetricsCollectors(t *testing.T) {
	t.Parallel()

	f := factory.New(nil)
	c, ok := f.(metrics.Collector)
	if !ok {
		t.Fatal("factory does not implement metrics.Collector")
	}

	collectors := c.Metrics()
	if len(collectors) != 2 {
		t.Fatalf("got %d collectors, want 2", len(collectors))
	}

	names := make([]string, 0, len(collectors))
	for _, collector := range collectors {
		ch := make(chan *prometheus.Desc, 1)
		collector.Describe(ch)
		close(ch)
		for d := range ch {
			names = append(names, d.String())
		}
	}

	joined := strings.Join(names, "\n")
	for _, want := range []string{
		"feeds_lookup_started_total",
		"feeds_lookup_duration_seconds",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing metric %q in %s", want, joined)
		}
	}
}

func TestInstrumentedLookup(t *testing.T) {
	t.Parallel()

	chunk := swarm.NewChunk(swarm.NewAddress(make([]byte, swarm.HashSize)), []byte("payload"))
	tests := []struct {
		name       string
		feedType   feeds.Type
		chunk      swarm.Chunk
		err        error
		wantResult string
	}{
		{
			name:       "sequence found",
			feedType:   feeds.Sequence,
			chunk:      chunk,
			wantResult: "found",
		},
		{
			name:       "sequence not found",
			feedType:   feeds.Sequence,
			wantResult: "not_found",
		},
		{
			name:       "sequence error",
			feedType:   feeds.Sequence,
			err:        errors.New("lookup failed"),
			wantResult: "error",
		},
		{
			name:       "sequence canceled",
			feedType:   feeds.Sequence,
			err:        context.Canceled,
			wantResult: "canceled",
		},
		{
			name:       "epoch found",
			feedType:   feeds.Epoch,
			chunk:      chunk,
			wantResult: "found",
		},
		{
			name:       "epoch not found",
			feedType:   feeds.Epoch,
			wantResult: "not_found",
		},
		{
			name:       "epoch error",
			feedType:   feeds.Epoch,
			err:        errors.New("lookup failed"),
			wantResult: "error",
		},
		{
			name:       "epoch canceled",
			feedType:   feeds.Epoch,
			err:        context.Canceled,
			wantResult: "canceled",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := factory.New(nil).(*factory.Factory)
			lookup := f.WrapLookup(tc.feedType, &mockLookup{chunk: tc.chunk, err: tc.err})

			ch, _, _, atErr := lookup.At(context.Background(), 0, 0)
			if !errors.Is(atErr, tc.err) {
				t.Fatalf("got error %v, want %v", atErr, tc.err)
			}
			if tc.chunk == nil && ch != nil {
				t.Fatal("expected nil chunk")
			}
			if tc.chunk != nil && !ch.Address().Equal(tc.chunk.Address()) {
				t.Fatalf("got chunk %s, want %s", ch.Address(), tc.chunk.Address())
			}

			typ := strings.ToLower(tc.feedType.String())
			if got := counterValue(t, f.MetricsStruct().LookupStarted.WithLabelValues(typ)); got != 1 {
				t.Fatalf("lookup_started_total{%s}=%v, want 1", typ, got)
			}
			if got := histogramCount(t, f.MetricsStruct().LookupDuration.WithLabelValues(typ, tc.wantResult)); got != 1 {
				t.Fatalf("lookup_duration_seconds{%s,%s} count=%v, want 1", typ, tc.wantResult, got)
			}
		})
	}
}

func TestNewLookupUnknownType(t *testing.T) {
	t.Parallel()

	f := factory.New(nil)
	_, err := f.NewLookup(feeds.Type(99), feeds.New([]byte("topic"), common.Address{}))
	if !errors.Is(err, feeds.ErrFeedTypeNotFound) {
		t.Fatalf("got %v, want %v", err, feeds.ErrFeedTypeNotFound)
	}
}

func TestMetricsRegistryScrape(t *testing.T) {
	t.Parallel()

	f := factory.New(nil).(*factory.Factory)
	registry := prometheus.NewRegistry()
	for _, c := range f.Metrics() {
		if err := registry.Register(c); err != nil {
			t.Fatal(err)
		}
	}

	chunk := swarm.NewChunk(swarm.NewAddress(make([]byte, swarm.HashSize)), []byte("payload"))
	lookups := []struct {
		typ   feeds.Type
		chunk swarm.Chunk
		err   error
	}{
		{feeds.Sequence, chunk, nil},
		{feeds.Sequence, nil, nil},
		{feeds.Sequence, nil, errors.New("boom")},
		{feeds.Sequence, nil, context.Canceled},
		{feeds.Epoch, chunk, nil},
		{feeds.Epoch, nil, nil},
	}
	for _, tc := range lookups {
		_, _, _, _ = f.WrapLookup(tc.typ, &mockLookup{chunk: tc.chunk, err: tc.err}).At(context.Background(), 0, 0)
	}

	gathered, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]map[string]float64)
	for _, mf := range gathered {
		series := make(map[string]float64)
		for _, metric := range mf.GetMetric() {
			labels := make([]string, 0, len(metric.GetLabel()))
			for _, l := range metric.GetLabel() {
				labels = append(labels, l.GetName()+"="+l.GetValue())
			}
			sort.Strings(labels)
			key := strings.Join(labels, ",")
			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				series[key] = metric.GetCounter().GetValue()
			case dto.MetricType_HISTOGRAM:
				series[key] = float64(metric.GetHistogram().GetSampleCount())
			}
		}
		got[mf.GetName()] = series
	}

	wantStarted := map[string]float64{
		"type=sequence": 4,
		"type=epoch":    2,
	}
	if err := cmpMaps(got["bee_feeds_lookup_started_total"], wantStarted); err != nil {
		t.Fatalf("lookup_started_total: %v", err)
	}

	wantDuration := map[string]float64{
		"result=canceled,type=sequence":  1,
		"result=error,type=sequence":     1,
		"result=found,type=epoch":        1,
		"result=found,type=sequence":     1,
		"result=not_found,type=epoch":    1,
		"result=not_found,type=sequence": 1,
	}
	if err := cmpMaps(got["bee_feeds_lookup_duration_seconds"], wantDuration); err != nil {
		t.Fatalf("lookup_duration_seconds: %v\ngot=%v", err, got["bee_feeds_lookup_duration_seconds"])
	}

	var body strings.Builder
	enc := expfmt.NewEncoder(&body, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range gathered {
		if err := enc.Encode(mf); err != nil {
			t.Fatal(err)
		}
	}
	if closer, ok := enc.(expfmt.Closer); ok {
		_ = closer.Close()
	}
	text := body.String()
	t.Logf("prometheus scrape:\n%s", text)
	for _, want := range []string{
		"# HELP bee_feeds_lookup_started_total",
		"# TYPE bee_feeds_lookup_started_total counter",
		`bee_feeds_lookup_started_total{type="sequence"} 4`,
		`bee_feeds_lookup_started_total{type="epoch"} 2`,
		"# HELP bee_feeds_lookup_duration_seconds",
		"# TYPE bee_feeds_lookup_duration_seconds histogram",
		`bee_feeds_lookup_duration_seconds_count{result="found",type="sequence"} 1`,
		`bee_feeds_lookup_duration_seconds_count{result="not_found",type="sequence"} 1`,
		`bee_feeds_lookup_duration_seconds_count{result="error",type="sequence"} 1`,
		`bee_feeds_lookup_duration_seconds_count{result="canceled",type="sequence"} 1`,
		`bee_feeds_lookup_duration_seconds_count{result="found",type="epoch"} 1`,
		`bee_feeds_lookup_duration_seconds_count{result="not_found",type="epoch"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("scrape missing %q\n%s", want, text)
		}
	}
}

type mockLookup struct {
	chunk swarm.Chunk
	err   error
}

func (m *mockLookup) At(context.Context, int64, uint64) (swarm.Chunk, feeds.Index, feeds.Index, error) {
	return m.chunk, nil, nil, m.err
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	metric := &dto.Metric{}
	if err := c.Write(metric); err != nil {
		t.Fatal(err)
	}
	return metric.GetCounter().GetValue()
}

func histogramCount(t *testing.T, o prometheus.Observer) float64 {
	t.Helper()
	m, ok := o.(prometheus.Metric)
	if !ok {
		t.Fatal("observer is not a prometheus.Metric")
	}
	metric := &dto.Metric{}
	if err := m.Write(metric); err != nil {
		t.Fatal(err)
	}
	return float64(metric.GetHistogram().GetSampleCount())
}

func cmpMaps(got, want map[string]float64) error {
	for k, wantV := range want {
		if got[k] != wantV {
			return fmt.Errorf("key %q got %v want %v", k, got[k], wantV)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			return fmt.Errorf("unexpected key %q", k)
		}
	}
	return nil
}
