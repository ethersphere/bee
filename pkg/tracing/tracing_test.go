// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/uber/jaeger-client-go"
)

func TestSpanFromHeaders(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

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
	t.Parallel()

	tracer := newTracer(t)

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
	t.Parallel()

	tracer := newTracer(t)

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
	t.Parallel()

	tracer := newTracer(t)

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
	t.Parallel()

	tracer := newTracer(t)

	buf := new(bytes.Buffer)

	span, logger, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", log.NewLogger("test", log.WithSink(buf), log.WithJSONOutput()))
	defer span.Finish()

	wantTraceID := span.Context().(jaeger.SpanContext).TraceID()

	logger.Info("msg")
	data := make(map[string]interface{})
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, ok := data[tracing.LogField]
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
	t.Parallel()

	tracer := newTracer(t)

	span, logger, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	if logger != nil {
		t.Error("logger is not nil")
	}
}

func TestNewLoggerWithTraceID(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	buf := new(bytes.Buffer)

	logger := tracing.NewLoggerWithTraceID(ctx, log.NewLogger("test", log.WithSink(buf), log.WithJSONOutput()))

	wantTraceID := span.Context().(jaeger.SpanContext).TraceID()

	logger.Info("msg")
	data := make(map[string]interface{})
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, ok := data[tracing.LogField]
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
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.Finish()

	logger := tracing.NewLoggerWithTraceID(ctx, nil)

	if logger != nil {
		t.Error("logger is not nil")
	}
}

func newTracer(t *testing.T) *tracing.Tracer {
	t.Helper()

	tracer, closer, err := tracing.NewTracer(&tracing.Options{
		Enabled:     true,
		ServiceName: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, closer)

	return tracer
}
