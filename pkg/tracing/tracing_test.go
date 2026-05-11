// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestSpanFromHeaders(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	headers := make(p2p.Headers)
	if err := tracer.AddContextHeader(ctx, headers); err != nil {
		t.Fatal(err)
	}

	gotSpanContext, err := tracer.FromHeaders(headers)
	if err != nil {
		t.Fatal(err)
	}

	if !gotSpanContext.IsValid() {
		t.Fatal("got invalid span context")
	}

	wantSpanContext := span.SpanContext()
	if !wantSpanContext.IsValid() {
		t.Fatal("got invalid start span context")
	}

	if gotSpanContext.TraceID() != wantSpanContext.TraceID() {
		t.Errorf("got trace id %s, want %s", gotSpanContext.TraceID(), wantSpanContext.TraceID())
	}
	if gotSpanContext.SpanID() != wantSpanContext.SpanID() {
		t.Errorf("got span id %s, want %s", gotSpanContext.SpanID(), wantSpanContext.SpanID())
	}
}

func TestSpanWithContextFromHeaders(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	headers := make(p2p.Headers)
	if err := tracer.AddContextHeader(ctx, headers); err != nil {
		t.Fatal(err)
	}

	ctx, err := tracer.WithContextFromHeaders(context.Background(), headers)
	if err != nil {
		t.Fatal(err)
	}

	gotSpanContext := tracing.FromContext(ctx)
	if !gotSpanContext.IsValid() {
		t.Fatal("got invalid span context")
	}

	wantSpanContext := span.SpanContext()
	if gotSpanContext.TraceID() != wantSpanContext.TraceID() {
		t.Errorf("got trace id %s, want %s", gotSpanContext.TraceID(), wantSpanContext.TraceID())
	}
}

// TestFromHeaders_undecodablePayload exercises the rolling-upgrade tolerance:
// peers running an older binary may send a payload in a different format and we
// must not fail the stream, just return ErrContextNotFound so trace continuity
// silently breaks at that hop.
func TestFromHeaders_undecodablePayload(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	headers := p2p.Headers{
		p2p.HeaderNameTracingSpanContext: []byte("not-a-valid-binary-carrier"),
	}

	_, err := tracer.FromHeaders(headers)
	if err != tracing.ErrContextNotFound {
		t.Errorf("got error %v, want %v", err, tracing.ErrContextNotFound)
	}
}

func TestFromContext(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	wantSpanContext := span.SpanContext()
	if !wantSpanContext.IsValid() {
		t.Fatal("got invalid start span context")
	}

	gotSpanContext := tracing.FromContext(ctx)
	if !gotSpanContext.IsValid() {
		t.Fatal("got invalid span context")
	}

	if gotSpanContext.TraceID() != wantSpanContext.TraceID() {
		t.Errorf("got trace id %s, want %s", gotSpanContext.TraceID(), wantSpanContext.TraceID())
	}
}

func TestWithContext(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	wantSpanContext := span.SpanContext()
	if !wantSpanContext.IsValid() {
		t.Fatal("got invalid start span context")
	}

	ctx := tracing.WithContext(context.Background(), wantSpanContext)
	gotSpanContext := tracing.FromContext(ctx)
	if gotSpanContext.TraceID() != wantSpanContext.TraceID() {
		t.Errorf("got trace id %s, want %s", gotSpanContext.TraceID(), wantSpanContext.TraceID())
	}
}

func TestStartSpanFromContext_logger(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	buf := new(bytes.Buffer)

	span, logger, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", log.NewLogger("test", log.WithSink(buf), log.WithJSONOutput()))
	defer span.End()

	wantTraceID := span.SpanContext().TraceID().String()

	logger.Info("msg")
	data := make(map[string]any)
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

	if gotTraceID != wantTraceID {
		t.Errorf("got trace id %q, want %q", gotTraceID, wantTraceID)
	}
}

func TestStartSpanFromContext_nilLogger(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, logger, _ := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	if logger != nil {
		t.Error("logger is not nil")
	}
}

func TestNewLoggerWithTraceID(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	buf := new(bytes.Buffer)

	logger := tracing.NewLoggerWithTraceID(ctx, log.NewLogger("test", log.WithSink(buf), log.WithJSONOutput()))

	wantTraceID := span.SpanContext().TraceID().String()

	logger.Info("msg")
	data := make(map[string]any)
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

	if gotTraceID != wantTraceID {
		t.Errorf("got trace id %q, want %q", gotTraceID, wantTraceID)
	}
}

func TestNewLoggerWithTraceID_nilLogger(t *testing.T) {
	t.Parallel()

	tracer := newTracer(t)

	span, _, ctx := tracer.StartSpanFromContext(context.Background(), "some-operation", nil)
	defer span.End()

	logger := tracing.NewLoggerWithTraceID(ctx, nil)

	if logger != nil {
		t.Error("logger is not nil")
	}
}

// newTracer returns a Tracer wired to a real OTel SDK with no exporter, so
// spans receive valid contexts but nothing is shipped over the network.
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
