// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/feeds/factory"
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	startedMetric  = "bee_feeds_lookup_started_total"
	durationMetric = "bee_feeds_lookup_duration_seconds"
)

func TestInstrumentedLookup(t *testing.T) {
	t.Parallel()

	chunk := swarm.NewChunk(swarm.NewAddress(make([]byte, swarm.HashSize)), []byte("payload"))
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)
	tests := []struct {
		name       string
		feedType   feeds.Type
		chunk      swarm.Chunk
		err        error
		ctx        context.Context
		wantType   string
		wantResult string
	}{
		{
			name:       "found",
			feedType:   feeds.Sequence,
			chunk:      chunk,
			wantType:   "sequence",
			wantResult: "found",
		},
		{
			name:       "not found",
			feedType:   feeds.Sequence,
			wantType:   "sequence",
			wantResult: "not_found",
		},
		{
			name:       "error",
			feedType:   feeds.Sequence,
			err:        errors.New("lookup failed"),
			wantType:   "sequence",
			wantResult: "error",
		},
		{
			name:       "canceled",
			feedType:   feeds.Sequence,
			err:        context.Canceled,
			wantType:   "sequence",
			wantResult: "canceled",
		},
		{
			name:       "wrapped canceled",
			feedType:   feeds.Sequence,
			err:        fmt.Errorf("get chunk: %w", context.Canceled),
			wantType:   "sequence",
			wantResult: "canceled",
		},
		{
			name:       "deadline exceeded",
			feedType:   feeds.Sequence,
			err:        context.DeadlineExceeded,
			wantType:   "sequence",
			wantResult: "error",
		},
		{
			name:       "deadline context",
			feedType:   feeds.Sequence,
			chunk:      chunk,
			ctx:        expired,
			wantType:   "sequence",
			wantResult: "error",
		},
		{
			name:       "epoch found",
			feedType:   feeds.Epoch,
			chunk:      chunk,
			wantType:   "epoch",
			wantResult: "found",
		},
		{
			name:       "epoch not found",
			feedType:   feeds.Epoch,
			wantType:   "epoch",
			wantResult: "not_found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := factory.New(nil)
			var cur, next feeds.Index = mockIndex("cur"), mockIndex("next")
			lookup := factory.WrapLookup(f, tc.feedType, &mockLookup{chunk: tc.chunk, cur: cur, next: next, err: tc.err})

			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			gotChunk, gotCur, gotNext, err := lookup.At(ctx, 0, 0)
			if !errors.Is(err, tc.err) {
				t.Fatalf("got error %v, want %v", err, tc.err)
			}
			if gotChunk != tc.chunk {
				t.Fatalf("got chunk %v, want %v", gotChunk, tc.chunk)
			}
			if gotCur != cur || gotNext != next {
				t.Fatalf("got indexes %v/%v, want %v/%v", gotCur, gotNext, cur, next)
			}

			counts, _ := gather(t, f)
			want := map[string]float64{
				startedMetric + "{type=" + tc.wantType + "}":                                     1,
				durationMetric + "_count{result=" + tc.wantResult + ",type=" + tc.wantType + "}": 1,
			}
			if !maps.Equal(counts, want) {
				t.Fatalf("got metrics %v, want %v", counts, want)
			}
		})
	}
}

func TestInstrumentedLookupDuration(t *testing.T) {
	t.Parallel()

	const delay = 50 * time.Millisecond
	f := factory.New(nil)
	lookup := factory.WrapLookup(f, feeds.Sequence, &mockLookup{delay: delay})

	if _, _, _, err := lookup.At(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}

	_, sums := gather(t, f)
	key := durationMetric + "_sum{result=not_found,type=sequence}"
	if got := sums[key]; got < delay.Seconds() {
		t.Fatalf("%s=%v, want at least %v", key, got, delay.Seconds())
	}
}

func TestInstrumentedLookupStartedBeforeCompletion(t *testing.T) {
	t.Parallel()

	f := factory.New(nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	lookup := factory.WrapLookup(f, feeds.Sequence, &mockLookup{entered: entered, release: release})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _, _ = lookup.At(context.Background(), 0, 0)
	}()

	<-entered
	inFlight, _ := gather(t, f)
	close(release)
	<-done

	want := map[string]float64{startedMetric + "{type=sequence}": 1}
	if !maps.Equal(inFlight, want) {
		t.Fatalf("got metrics during lookup %v, want %v", inFlight, want)
	}
}

func TestNewLookupRecordsMetrics(t *testing.T) {
	t.Parallel()

	notFound := storage.GetterFunc(func(context.Context, swarm.Address) (swarm.Chunk, error) {
		return nil, storage.ErrNotFound
	})
	failing := storage.GetterFunc(func(context.Context, swarm.Address) (swarm.Chunk, error) {
		return nil, errors.New("getter failed")
	})

	tests := []struct {
		name       string
		feedType   feeds.Type
		getter     storage.Getter
		wantType   string
		wantResult string
	}{
		{
			name:       "sequence not found",
			feedType:   feeds.Sequence,
			getter:     notFound,
			wantType:   "sequence",
			wantResult: "not_found",
		},
		{
			name:       "sequence error",
			feedType:   feeds.Sequence,
			getter:     failing,
			wantType:   "sequence",
			wantResult: "error",
		},
		{
			name:       "epoch not found",
			feedType:   feeds.Epoch,
			getter:     notFound,
			wantType:   "epoch",
			wantResult: "not_found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := factory.New(tc.getter)
			lookup, err := f.NewLookup(tc.feedType, feeds.New([]byte("topic"), common.Address{}))
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, _ = lookup.At(context.Background(), time.Now().Unix(), 0)

			counts, _ := gather(t, f)
			want := map[string]float64{
				startedMetric + "{type=" + tc.wantType + "}":                                     1,
				durationMetric + "_count{result=" + tc.wantResult + ",type=" + tc.wantType + "}": 1,
			}
			if !maps.Equal(counts, want) {
				t.Fatalf("got metrics %v, want %v", counts, want)
			}
		})
	}
}

func TestNewLookupSequenceCanceledAfterFirstChunk(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	chunk := swarm.NewChunk(swarm.NewAddress(make([]byte, swarm.HashSize)), []byte("payload"))
	getter := storage.GetterFunc(func(ctx context.Context, _ swarm.Address) (swarm.Chunk, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The client goes away after the first update is fetched. Later probes
		// observe the canceled context and the sequence finder treats them as
		// not found, returning this chunk with a nil error.
		cancel()
		return chunk, nil
	})

	f := factory.New(getter)
	lookup, err := f.NewLookup(feeds.Sequence, feeds.New([]byte("topic"), common.Address{}))
	if err != nil {
		t.Fatal(err)
	}

	got, _, _, err := lookup.At(ctx, time.Now().Unix(), 0)
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	if got != chunk {
		t.Fatalf("got chunk %v, want the chunk fetched before cancel", got)
	}

	counts, _ := gather(t, f)
	want := map[string]float64{
		startedMetric + "{type=sequence}":                        1,
		durationMetric + "_count{result=canceled,type=sequence}": 1,
	}
	if !maps.Equal(counts, want) {
		t.Fatalf("got metrics %v, want %v", counts, want)
	}
}

func TestNewLookupUnknownType(t *testing.T) {
	t.Parallel()

	f := factory.New(nil)
	_, err := f.NewLookup(feeds.Type(99), feeds.New([]byte("topic"), common.Address{}))
	if !errors.Is(err, feeds.ErrFeedTypeNotFound) {
		t.Fatalf("got %v, want %v", err, feeds.ErrFeedTypeNotFound)
	}

	if counts, _ := gather(t, f); len(counts) != 0 {
		t.Fatalf("got metrics %v, want none", counts)
	}
}

// gather registers the factory metrics in a fresh registry and returns
// counter values and histogram sample counts, and histogram sample sums,
// keyed by metric name and sorted labels.
func gather(t *testing.T, f feeds.Factory) (counts, sums map[string]float64) {
	t.Helper()

	c, ok := f.(metrics.Collector)
	if !ok {
		t.Fatal("factory does not implement metrics.Collector")
	}
	registry := prometheus.NewRegistry()
	for _, collector := range c.Metrics() {
		if err := registry.Register(collector); err != nil {
			t.Fatal(err)
		}
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	counts = make(map[string]float64)
	sums = make(map[string]float64)
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			labels := make([]string, 0, len(m.GetLabel()))
			for _, l := range m.GetLabel() {
				labels = append(labels, l.GetName()+"="+l.GetValue())
			}
			slices.Sort(labels)
			key := "{" + strings.Join(labels, ",") + "}"

			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				counts[mf.GetName()+key] = m.GetCounter().GetValue()
			case dto.MetricType_HISTOGRAM:
				counts[mf.GetName()+"_count"+key] = float64(m.GetHistogram().GetSampleCount())
				sums[mf.GetName()+"_sum"+key] = m.GetHistogram().GetSampleSum()
			}
		}
	}
	return counts, sums
}

type mockIndex string

func (i mockIndex) MarshalBinary() ([]byte, error) { return []byte(i), nil }
func (i mockIndex) Next(int64, uint64) feeds.Index { return i }
func (i mockIndex) String() string                 { return string(i) }

type mockLookup struct {
	chunk     swarm.Chunk
	cur, next feeds.Index
	err       error
	delay     time.Duration
	entered   chan struct{}
	release   chan struct{}
}

func (m *mockLookup) At(context.Context, int64, uint64) (swarm.Chunk, feeds.Index, feeds.Index, error) {
	if m.entered != nil {
		close(m.entered)
		<-m.release
	}
	time.Sleep(m.delay)
	return m.chunk, m.cur, m.next, m.err
}
