// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/uber/jaeger-client-go"
)

func TestSpanFromHeaders(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	headers := make(p2p.Headers)
	if err := tracer.AddContextHeader(ctx, headers); err != nil {
		t.Fatal(err)
	}

	gotSpanContext, err := tracer.FromHeaders(headers)
	if err != nil {
		t.Fatal(err)
	}

	if fmt.Sprint(gotSpanContext) == "" {
		t.Fatal("got empty span context")
	}

	wantSpanContext := span.Context()
	if fmt.Sprint(wantSpanContext) == "" {
		t.Fatal("got empty start span context")
	}

	if fmt.Sprint(gotSpanContext) != fmt.Sprint(wantSpanContext) {
		t.Errorf("got span context %+v, want %+v", gotSpanContext, wantSpanContext)
	}
}

func TestSpanWithContextFromHeaders(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	headers := make(p2p.Headers)
	if err := tracer.AddContextHeader(ctx, headers); err != nil {
		t.Fatal(err)
	}

	ctx, err := tracer.WithContextFromHeaders(context.Background(), headers)
	if err != nil {
		t.Fatal(err)
	}

	gotSpanContext := tracing.FromContext(ctx)
	if fmt.Sprint(gotSpanContext) == "" {
		t.Fatal("got empty span context")
	}

	wantSpanContext := span.Context()
	if fmt.Sprint(wantSpanContext) == "" {
		t.Fatal("got empty start span context")
	}

	if fmt.Sprint(gotSpanContext) != fmt.Sprint(wantSpanContext) {
		t.Errorf("got span context %+v, want %+v", gotSpanContext, wantSpanContext)
	}
}

func TestFromContext(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	wantSpanContext := span.Context()
	if fmt.Sprint(wantSpanContext) == "" {
		t.Fatal("got empty start span context")
	}

	gotSpanContext := tracing.FromContext(ctx)
	if fmt.Sprint(gotSpanContext) == "" {
		t.Fatal("got empty span context")
	}

	if fmt.Sprint(gotSpanContext) != fmt.Sprint(wantSpanContext) {
		t.Errorf("got span context %+v, want %+v", gotSpanContext, wantSpanContext)
	}
}

func TestWithContext(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, _, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	wantSpanContext := span.Context()
	if fmt.Sprint(wantSpanContext) == "" {
		t.Fatal("got empty start span context")
	}

	ctx := tracing.WithContext(context.Background(), span.Context())

	gotSpanContext := tracing.FromContext(ctx)
	if fmt.Sprint(gotSpanContext) == "" {
		t.Fatal("got empty span context")
	}

	if fmt.Sprint(gotSpanContext) != fmt.Sprint(wantSpanContext) {
		t.Errorf("got span context %+v, want %+v", gotSpanContext, wantSpanContext)
	}
}

func TestStartSpanFromContext_logger(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, logger, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", logging.New(io.Discard, 0))
	defer span.Finish()

	wantTraceID := span.Context().(jaeger.SpanContext).TraceID()

	v, ok := logger.Data[tracing.LogField]
	if !ok {
		t.Fatalf("log field %q not found", tracing.LogField)
	}

	gotTraceID, ok := v.(string)
	if !ok {
		t.Fatalf("log field %q is not string", tracing.LogField)
	}

	if gotTraceID != wantTraceID.String() {
		t.Errorf("got trace id %q, want %q", gotTraceID, wantTraceID.String())
	}
}

func TestStartSpanFromContext_nilLogger(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, logger, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	if logger != nil {
		t.Error("logger is not nil")
	}
}

func TestNewLoggerWithTraceID(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	logger := tracing.NewLoggerWithTraceID(ctx, logging.New(io.Discard, 0))

	wantTraceID := span.Context().(jaeger.SpanContext).TraceID()

	v, ok := logger.Data[tracing.LogField]
	if !ok {
		t.Fatalf("log field %q not found", tracing.LogField)
	}

	gotTraceID, ok := v.(string)
	if !ok {
		t.Fatalf("log field %q is not string", tracing.LogField)
	}

	if gotTraceID != wantTraceID.String() {
		t.Errorf("got trace id %q, want %q", gotTraceID, wantTraceID.String())
	}
}

func TestNewLoggerWithTraceID_nilLogger(t *testing.T) {
	tracer, closer := newTracer(t)
	defer closer.Close()

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	logger := tracing.NewLoggerWithTraceID(ctx, nil)

	if logger != nil {
		t.Error("logger is not nil")
	}
}

func newTracer(t *testing.T) (*tracing.Tracer, io.Closer) {
	t.Helper()

	tracer, closer, err := tracing.NewTracer(&tracing.Options{
		Enabled:     true,
		ServiceName: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	return tracer, closer
}
