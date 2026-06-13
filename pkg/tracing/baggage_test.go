// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"go.opentelemetry.io/otel/baggage"
)

// baggageValue returns the value of the baggage member with the given key in
// ctx, or ("", false) when absent.
func baggageValue(ctx context.Context, key string) (string, bool) {
	m := baggage.FromContext(ctx).Member(key)
	if m.Key() == "" {
		return "", false
	}
	return m.Value(), true
}

func TestBaggageRoundTripP2P(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	ctx, err := tracing.WithBaggageMember(ctx, "batch_id", "deadbeef")
	if err != nil {
		t.Fatal(err)
	}

	headers := make(p2p.Headers)
	if err := tracer.AddContextHeader(ctx, headers); err != nil {
		t.Fatal(err)
	}
	if headers[p2p.HeaderNameTracingBaggage] == nil {
		t.Fatal("baggage header was not set")
	}

	got, err := tracer.WithContextFromHeaders(context.Background(), headers)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := baggageValue(got, "batch_id"); !ok || v != "deadbeef" {
		t.Errorf("batch_id baggage = %q (present=%v), want %q", v, ok, "deadbeef")
	}
}

func TestBaggageRoundTripHTTP(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	ctx, err := tracing.WithBaggageMember(ctx, "batch_id", "deadbeef")
	if err != nil {
		t.Fatal(err)
	}

	headers := make(http.Header)
	if err := tracer.AddContextHTTPHeader(ctx, headers); err != nil {
		t.Fatal(err)
	}

	got, err := tracer.WithContextFromHTTPHeaders(context.Background(), headers)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := baggageValue(got, "batch_id"); !ok || v != "deadbeef" {
		t.Errorf("batch_id baggage = %q (present=%v), want %q", v, ok, "deadbeef")
	}
}

// TestBaggageOnlyWithoutSpanContext verifies that baggage is applied to the
// returned context even when no span context is present in the p2p headers.
func TestBaggageOnlyWithoutSpanContext(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	bag, err := baggage.Parse("batch_id=deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	headers := p2p.Headers{
		p2p.HeaderNameTracingBaggage: []byte(bag.String()),
	}

	got, err := tracer.WithContextFromHeaders(context.Background(), headers)
	if err == nil {
		t.Fatal("expected ErrContextNotFound when no span context is present")
	}
	if v, ok := baggageValue(got, "batch_id"); !ok || v != "deadbeef" {
		t.Errorf("batch_id baggage = %q (present=%v), want %q", v, ok, "deadbeef")
	}
}
