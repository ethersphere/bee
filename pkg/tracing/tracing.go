// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tracing wraps the OpenTelemetry SDK and exposes a small set of
// helpers for starting spans, propagating span context across libp2p streams
// and HTTP requests, and annotating loggers with trace ids.
package tracing

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
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
	"google.golang.org/grpc/credentials"
)

// ErrContextNotFound is returned when tracing context is not present in
// p2p Headers, HTTP headers, or the go context.
var ErrContextNotFound = errors.New("tracing context not found")

// logField is the key in log message field that holds the tracing id value.
const logField = "traceID"

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

// Supported OTLP transport values for Options.Protocol.
const (
	protocolHTTP = "http"
	protocolGRPC = "grpc"
)

// noopTracer is returned when tracing is disabled or *Tracer is nil so callers
// always operate against a working trace.Tracer.
var noopTracer = &Tracer{tracer: noop.NewTracerProvider().Tracer(instrumentationName)}

// httpPropagator carries trace context and baggage across HTTP via the W3C
// TraceContext (traceparent, tracestate) and Baggage (baggage) standard headers.
var httpPropagator propagation.TextMapPropagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

// Tracer wraps an OTel Tracer and provides p2p/HTTP carriers plus helpers
// aligned with bee's tracing API.
type Tracer struct {
	tracer trace.Tracer
}

// otel returns the underlying OTel tracer, falling back to a no-op tracer when
// the receiver or its tracer is nil. This keeps the exported Tracer safe to use
// even when obtained without NewTracer (e.g. a zero-value &Tracer{}).
func (t *Tracer) otel() trace.Tracer {
	if t == nil || t.tracer == nil {
		return noopTracer.tracer
	}
	return t.tracer
}

// Options are the constructor parameters for Tracer.
type Options struct {
	// Enabled toggles span recording. When false the tracer is a no-op.
	Enabled bool
	// Endpoint is the OTLP collector endpoint, e.g. "127.0.0.1:4318" for http
	// or "127.0.0.1:4317" for grpc. Required when Enabled is true.
	Endpoint string
	// ServiceName is reported as the OTel service.name resource attribute.
	ServiceName string
	// ServiceVersion is reported as the OTel service.version resource
	// attribute. When empty the attribute is omitted.
	ServiceVersion string
	// Environment is reported as the OTel deployment.environment resource
	// attribute (e.g. "mainnet", "testnet"). When empty the attribute is omitted.
	Environment string
	// InstanceID is reported as the OTel service.instance.id resource attribute
	// (the node's overlay address). When empty the attribute is omitted.
	InstanceID string
	// Insecure disables TLS for the OTLP exporter (useful for a local collector).
	Insecure bool
	// CAFile is an optional path to a PEM-encoded CA bundle used to verify
	// the OTLP collector certificate. Ignored when Insecure is true. When
	// empty and Insecure is false, the system root CAs are used.
	CAFile string
	// SamplingRatio is the head-based sampling ratio for the parent-based
	// sampler in the range [0, 1]. 0 disables sampling for non-parented spans;
	// 1 samples everything. Values outside [0, 1] are clamped to the nearest bound.
	SamplingRatio float64
	// Protocol selects the OTLP exporter transport: "http" or "grpc". Empty
	// defaults to "http".
	Protocol string
	// Logger, when set, receives a confirmation line once tracing is wired up
	// and OTLP exporter errors (e.g. an unreachable collector) via the OTel
	// global error handler. Optional.
	Logger log.Logger
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

	if o.Endpoint == "" {
		return nil, nil, errors.New("tracing-endpoint is required when tracing is enabled")
	}

	res, err := newResource(o)
	if err != nil {
		return nil, nil, fmt.Errorf("otel resource: %w", err)
	}

	ratio := o.SamplingRatio
	if ratio < 0 {
		ratio = 0
	} else if ratio > 1 {
		ratio = 1
	}

	client, err := newOTLPClient(o)
	if err != nil {
		return nil, nil, err
	}
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, nil, fmt.Errorf("otlp exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		// Batch processor keeps SDK defaults; tunable via OTEL_BSP_* env vars.
		sdktrace.WithBatcher(exporter),
	)
	if o.Logger != nil {
		// Route async OTLP export failures (e.g. an unreachable collector) to the
		// node logger so they are visible instead of silently dropped.
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
			o.Logger.Warning("tracing exporter error", "error", err)
		}))
		o.Logger.Info("tracing enabled", "endpoint", o.Endpoint, "protocol", o.Protocol, "sampling_ratio", ratio)
	}

	return &Tracer{tracer: tp.Tracer(instrumentationName)}, providerCloser{tp: tp}, nil
}

// NewTracerFromProvider wraps an existing OTel TracerProvider in a Tracer. It is
// primarily useful for tests that need a recording tracer (e.g. one backed by an
// in-memory span recorder) rather than the OTLP exporter pipeline NewTracer builds.
func NewTracerFromProvider(tp trace.TracerProvider) *Tracer {
	return &Tracer{tracer: tp.Tracer(instrumentationName)}
}

// newResource builds the OTel resource describing this node. The env options
// come before WithAttributes so the configured values win over colliding
// OTEL_SERVICE_NAME/OTEL_RESOURCE_ATTRIBUTES, while the environment can still
// add attributes (e.g. cluster, region).
func newResource(o *Options) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{semconv.ServiceName(o.ServiceName)}
	if o.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(o.ServiceVersion))
	}
	if o.Environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironment(o.Environment))
	}
	if o.InstanceID != "" {
		attrs = append(attrs, semconv.ServiceInstanceID(o.InstanceID))
	}

	return resource.New(context.Background(),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(attrs...),
	)
}

// newOTLPClient builds the OTLP client for the configured transport. An empty
// Protocol defaults to HTTP for backward compatibility with the initial OTLP
// rollout.
func newOTLPClient(o *Options) (otlptrace.Client, error) {
	switch o.Protocol {
	case "", protocolHTTP:
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(o.Endpoint),
			otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
		}
		if o.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		} else if o.CAFile != "" {
			tlsConfig, err := loadCAFile(o.CAFile)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlptracehttp.WithTLSClientConfig(tlsConfig))
		}
		return otlptracehttp.NewClient(opts...), nil
	case protocolGRPC:
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(o.Endpoint),
			otlptracegrpc.WithCompressor("gzip"),
		}
		if o.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		} else if o.CAFile != "" {
			tlsConfig, err := loadCAFile(o.CAFile)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)))
		}
		return otlptracegrpc.NewClient(opts...), nil
	default:
		return nil, fmt.Errorf("unsupported otlp protocol %q (want %q or %q)", o.Protocol, protocolHTTP, protocolGRPC)
	}
}

// loadCAFile reads a PEM-encoded CA bundle from path and returns a *tls.Config
// that uses it as the only root for OTLP collector certificate verification.
func loadCAFile(path string) (*tls.Config, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tracing CA file %q: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("tracing CA file %q contains no valid PEM certificates", path)
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// StartSpanFromContext starts a new span as a child of any span context already
// present in ctx. If logger is non-nil, a derived logger annotated with the
// trace id is returned alongside the new context.
func (t *Tracer) StartSpanFromContext(ctx context.Context, operationName string, l log.Logger, opts ...trace.SpanStartOption) (trace.Span, log.Logger, context.Context) {
	ctx, span := t.otel().Start(ctx, operationName, opts...)
	return span, loggerWithTraceID(span.SpanContext(), l), ctx
}

// FollowSpanFromContext starts a new span with a Link to the span context in
// ctx. Links are the OTel equivalent of OpenTracing's FollowsFrom relation:
// the new span is causally related but not a direct child.
func (t *Tracer) FollowSpanFromContext(ctx context.Context, operationName string, l log.Logger, opts ...trace.SpanStartOption) (trace.Span, log.Logger, context.Context) {
	if parent := FromContext(ctx); parent.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: parent}))
	}

	ctx, span := t.otel().Start(ctx, operationName, opts...)
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
	if bag := baggage.FromContext(ctx); bag.Len() > 0 {
		headers[p2p.HeaderNameTracingBaggage] = []byte(bag.String())
	}
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

// WithContextFromHeaders extracts a span context and any baggage from the p2p
// headers and returns a new context carrying them. Baggage is applied even when
// no span context is present. Safe to call on a nil receiver.
func (t *Tracer) WithContextFromHeaders(ctx context.Context, headers p2p.Headers) (context.Context, error) {
	// Baggage arrives from an arbitrary remote peer, so treat it as untrusted:
	// undecodable payloads are ignored, and callers must only ever surface its
	// values as span attributes. Never promote baggage to metric labels or
	// unbounded log fields — arbitrary peers could blow up cardinality.
	if v := headers[p2p.HeaderNameTracingBaggage]; v != nil {
		if bag, err := baggage.Parse(string(v)); err == nil {
			ctx = baggage.ContextWithBaggage(ctx, bag)
		}
	}

	sc, err := t.FromHeaders(headers)
	if err != nil {
		return ctx, err
	}
	return WithContext(ctx, sc), nil
}

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

// WithContextFromHTTPHeaders extracts a span context and any baggage from HTTP
// headers and returns a new context carrying them. Baggage is applied even when
// no span context is present. Safe to call on a nil receiver.
func (t *Tracer) WithContextFromHTTPHeaders(ctx context.Context, headers http.Header) (context.Context, error) {
	ctx = httpPropagator.Extract(ctx, propagation.HeaderCarrier(headers))
	if sc := trace.SpanContextFromContext(ctx); !sc.IsValid() {
		return ctx, ErrContextNotFound
	}
	return ctx, nil
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

// WithBaggageMember returns a context carrying an additional baggage member
// (key=value) on top of any baggage already present. Baggage propagates across
// HTTP and p2p hops alongside the trace, letting a value such as a batch id
// follow a request end to end. The original context is returned unchanged if the
// key or value is not a valid baggage member.
func WithBaggageMember(ctx context.Context, key, value string) (context.Context, error) {
	member, err := baggage.NewMember(key, value)
	if err != nil {
		return ctx, err
	}
	bag, err := baggage.FromContext(ctx).SetMember(member)
	if err != nil {
		return ctx, err
	}
	return baggage.ContextWithBaggage(ctx, bag), nil
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
	return l.WithValues(logField, sc.TraceID().String()).Build()
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
