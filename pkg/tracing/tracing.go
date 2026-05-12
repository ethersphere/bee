// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tracing wraps the OpenTelemetry SDK and exposes a small set of
// helpers for starting spans, propagating span context across libp2p streams
// and HTTP requests, and annotating loggers with trace ids.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// ErrContextNotFound is returned when tracing context is not present in
// p2p Headers, HTTP headers, or the go context.
var ErrContextNotFound = errors.New("tracing context not found")

// LogField is the key in log message field that holds the tracing id value.
const LogField = "traceID"

// instrumentationName identifies spans produced by this package to the OTel SDK.
const instrumentationName = "github.com/ethersphere/bee/v2/pkg/tracing"

// p2pCarrierVersion is the leading byte of the libp2p binary span context
// payload. It exists so the wire format can evolve without silently breaking
// peers running an older binary.
const p2pCarrierVersion byte = 1

// p2pCarrierLen is the encoded payload size:
// 1 version + 16 TraceID + 8 SpanID + 1 TraceFlags.
const p2pCarrierLen = 26

// shutdownTimeout bounds how long Close waits for the OTel batch processor to
// flush pending spans on shutdown.
const shutdownTimeout = 5 * time.Second

// noopTracer is returned when tracing is disabled or *Tracer is nil so callers
// always operate against a working trace.Tracer.
var noopTracer = &Tracer{tracer: noop.NewTracerProvider().Tracer(instrumentationName)}

// Tracer wraps an OTel Tracer and provides p2p/HTTP carriers plus helpers
// aligned with bee's tracing API.
type Tracer struct {
	tracer trace.Tracer
}

// Options are the constructor parameters for Tracer.
type Options struct {
	// Enabled toggles span recording. When false the tracer is a no-op.
	Enabled bool
	// Endpoint is the OTLP/HTTP collector endpoint, e.g. "127.0.0.1:4318".
	Endpoint string
	// ServiceName is reported as the OTel service.name resource attribute.
	ServiceName string
	// Insecure disables TLS for the OTLP exporter (useful for a local collector).
	Insecure bool
	// SamplingRatio is the head-based sampling ratio for the parent-based
	// sampler in the range [0, 1]. 0 disables sampling for non-parented spans;
	// 1 samples everything. Negative values are clamped to 0.
	SamplingRatio float64
	// Protocol selects the OTLP exporter transport: "http" or "grpc". Empty
	// defaults to "http".
	Protocol string
}

// NewTracer creates a new Tracer and returns a closer that flushes pending
// spans and shuts down the OTel pipeline.
func NewTracer(o *Options) (*Tracer, io.Closer, error) {
	if o == nil {
		o = new(Options)
	}

	if !o.Enabled {
		return noopTracer, noopCloser{}, nil
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName(o.ServiceName)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("otel resource: %w", err)
	}

	ratio := o.SamplingRatio
	if ratio < 0 {
		ratio = 0
	}

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	}

	// If no OTLP endpoint is configured, omit the exporter entirely. Spans are
	// still created with valid contexts (useful for local development and unit
	// tests) but nothing is shipped over the network.
	if o.Endpoint != "" {
		client, err := newOTLPClient(o)
		if err != nil {
			return nil, nil, err
		}
		exporter, err := otlptrace.New(context.Background(), client)
		if err != nil {
			return nil, nil, fmt.Errorf("otlp exporter: %w", err)
		}
		tpOpts = append(tpOpts, sdktrace.WithBatcher(exporter))
	}

	tp := sdktrace.NewTracerProvider(tpOpts...)
	return &Tracer{tracer: tp.Tracer(instrumentationName)}, providerCloser{tp: tp}, nil
}

// Supported OTLP transport values for Options.Protocol.
const (
	ProtocolHTTP = "http"
	ProtocolGRPC = "grpc"
)

// newOTLPClient builds the OTLP client for the configured transport. An empty
// Protocol defaults to HTTP for backward compatibility with the initial OTLP
// rollout.
func newOTLPClient(o *Options) (otlptrace.Client, error) {
	switch o.Protocol {
	case "", ProtocolHTTP:
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(o.Endpoint)}
		if o.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.NewClient(opts...), nil
	case ProtocolGRPC:
		opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(o.Endpoint)}
		if o.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.NewClient(opts...), nil
	default:
		return nil, fmt.Errorf("unsupported otlp protocol %q (want %q or %q)", o.Protocol, ProtocolHTTP, ProtocolGRPC)
	}
}

// StartSpanFromContext starts a new span as a child of any span context already
// present in ctx. If logger is non-nil, a derived logger annotated with the
// trace id is returned alongside the new context.
func (t *Tracer) StartSpanFromContext(ctx context.Context, operationName string, l log.Logger, opts ...trace.SpanStartOption) (trace.Span, log.Logger, context.Context) {
	if t == nil {
		t = noopTracer
	}

	if parent := FromContext(ctx); parent.IsValid() {
		ctx = trace.ContextWithSpanContext(ctx, parent)
	}

	ctx, span := t.tracer.Start(ctx, operationName, opts...)
	return span, loggerWithTraceID(span.SpanContext(), l), ctx
}

// FollowSpanFromContext starts a new span with a Link to the span context in
// ctx. Links are the OTel equivalent of OpenTracing's FollowsFrom relation:
// the new span is causally related but not a direct child.
func (t *Tracer) FollowSpanFromContext(ctx context.Context, operationName string, l log.Logger, opts ...trace.SpanStartOption) (trace.Span, log.Logger, context.Context) {
	if t == nil {
		t = noopTracer
	}

	if parent := FromContext(ctx); parent.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: parent}))
	}

	ctx, span := t.tracer.Start(ctx, operationName, opts...)
	return span, loggerWithTraceID(span.SpanContext(), l), ctx
}

// AddContextHeader serialises the active span context into the bee p2p header.
// It is safe to call on a nil receiver.
func (t *Tracer) AddContextHeader(ctx context.Context, headers p2p.Headers) error {
	sc := FromContext(ctx)
	if !sc.IsValid() {
		return ErrContextNotFound
	}

	headers[p2p.HeaderNameTracingSpanContext] = encodeP2PSpanContext(sc)
	return nil
}

// FromHeaders extracts the span context from the bee p2p header. ErrContextNotFound
// is returned when the header is absent or when its payload is undecodable —
// the latter lets mixed-version peers degrade to per-hop trace continuity loss
// rather than failing the stream. Safe to call on a nil receiver.
func (t *Tracer) FromHeaders(headers p2p.Headers) (trace.SpanContext, error) {
	v := headers[p2p.HeaderNameTracingSpanContext]
	if v == nil {
		return trace.SpanContext{}, ErrContextNotFound
	}
	sc, ok := decodeP2PSpanContext(v)
	if !ok {
		return trace.SpanContext{}, ErrContextNotFound
	}
	return sc, nil
}

// WithContextFromHeaders extracts a span context from the p2p header and
// returns a new context carrying it. Safe to call on a nil receiver.
func (t *Tracer) WithContextFromHeaders(ctx context.Context, headers p2p.Headers) (context.Context, error) {
	sc, err := t.FromHeaders(headers)
	if err != nil {
		return ctx, err
	}
	return WithContext(ctx, sc), nil
}

// httpPropagator carries trace context across HTTP via the W3C TraceContext
// standard headers (traceparent, tracestate).
var httpPropagator propagation.TextMapPropagator = propagation.TraceContext{}

// AddContextHTTPHeader injects the active span context into HTTP headers.
// Safe to call on a nil receiver.
func (t *Tracer) AddContextHTTPHeader(ctx context.Context, headers http.Header) error {
	sc := FromContext(ctx)
	if !sc.IsValid() {
		return ErrContextNotFound
	}

	httpPropagator.Inject(trace.ContextWithSpanContext(ctx, sc), propagation.HeaderCarrier(headers))
	return nil
}

// FromHTTPHeaders extracts a span context from a W3C traceparent header.
// Safe to call on a nil receiver.
func (t *Tracer) FromHTTPHeaders(headers http.Header) (trace.SpanContext, error) {
	ctx := httpPropagator.Extract(context.Background(), propagation.HeaderCarrier(headers))
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return trace.SpanContext{}, ErrContextNotFound
	}
	return sc, nil
}

// WithContextFromHTTPHeaders extracts a span context from HTTP headers and
// returns a new context carrying it. Safe to call on a nil receiver.
func (t *Tracer) WithContextFromHTTPHeaders(ctx context.Context, headers http.Header) (context.Context, error) {
	sc, err := t.FromHTTPHeaders(headers)
	if err != nil {
		return ctx, err
	}
	return WithContext(ctx, sc), nil
}

// WithContext stores a span context in ctx using the standard OTel context
// key, so any OTel-aware code (propagators, exporters) can find it.
func WithContext(ctx context.Context, sc trace.SpanContext) context.Context {
	return trace.ContextWithSpanContext(ctx, sc)
}

// FromContext returns the span context currently associated with ctx. The
// returned SpanContext's IsValid() reports false when none is present.
func FromContext(ctx context.Context) trace.SpanContext {
	return trace.SpanContextFromContext(ctx)
}

// RecordError attaches an error event to the span, marks the span status as
// Error, and records the supplied attributes alongside the error event. It is
// the OTel equivalent of the OpenTracing ext.LogError pattern bee used previously.
func RecordError(span trace.Span, err error, attrs ...attribute.KeyValue) {
	span.RecordError(err, trace.WithAttributes(attrs...))
	span.SetStatus(codes.Error, err.Error())
}

// NewLoggerWithTraceID returns a logger annotated with the trace id from ctx,
// or the original logger if no valid span context is present.
func NewLoggerWithTraceID(ctx context.Context, l log.Logger) log.Logger {
	return loggerWithTraceID(FromContext(ctx), l)
}

func loggerWithTraceID(sc trace.SpanContext, l log.Logger) log.Logger {
	if l == nil {
		return nil
	}
	if !sc.HasTraceID() {
		return l
	}
	return l.WithValues(LogField, sc.TraceID().String()).Build()
}

// encodeP2PSpanContext writes the trace+span ids and flags into a fixed-width
// payload. TraceState is omitted intentionally — bee does not use vendor
// tracestate routing.
func encodeP2PSpanContext(sc trace.SpanContext) []byte {
	buf := make([]byte, p2pCarrierLen)
	buf[0] = p2pCarrierVersion
	tid := sc.TraceID()
	sid := sc.SpanID()
	copy(buf[1:17], tid[:])
	copy(buf[17:25], sid[:])
	buf[25] = byte(sc.TraceFlags())
	return buf
}

func decodeP2PSpanContext(b []byte) (trace.SpanContext, bool) {
	if len(b) != p2pCarrierLen || b[0] != p2pCarrierVersion {
		return trace.SpanContext{}, false
	}
	var tid trace.TraceID
	var sid trace.SpanID
	copy(tid[:], b[1:17])
	copy(sid[:], b[17:25])
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.TraceFlags(b[25]),
		Remote:     true,
	})
	if !sc.IsValid() {
		return trace.SpanContext{}, false
	}
	return sc, true
}

// noopCloser is returned when tracing is disabled.
type noopCloser struct{}

func (noopCloser) Close() error { return nil }

// providerCloser flushes pending spans and stops the batch processor on Close.
type providerCloser struct {
	tp *sdktrace.TracerProvider
}

func (c providerCloser) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return c.tp.Shutdown(ctx)
}
