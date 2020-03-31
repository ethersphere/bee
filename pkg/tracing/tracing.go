// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tracing

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
)

var (
	// ErrContextNotFound is returned when tracing context is not present
	// in p2p Headers or context.
	ErrContextNotFound = errors.New("tracing context not found")

	// contextKey is used to reference a tracing context span as context value.
	contextKey = struct{}{}
	// noopTracer is the tracer that does nothing to handle a nil Tracer usage.
	noopTracer = &Tracer{tracer: new(opentracing.NoopTracer)}
)

// Tracer connect to a tracing server and handles tracing spans and contexts
// by using opentracing Tracer.
type Tracer struct {
	tracer opentracing.Tracer
}

// Options are optional parameters for Tracer constructor.
type Options struct {
	Enabled     bool
	Endpoint    string
	ServiceName string
}

// NewTracer creates a new Tracer and returns a closer which needs to be closed
// when the Tracer is no longer used to flush remaining traces.
func NewTracer(o *Options) (*Tracer, io.Closer, error) {
	if o == nil {
		o = new(Options)
	}

	cfg := config.Configuration{
		Disabled:    !o.Enabled,
		ServiceName: o.ServiceName,
		Sampler: &config.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &config.ReporterConfig{
			LogSpans:            true,
			BufferFlushInterval: 1 * time.Second,
			LocalAgentHostPort:  o.Endpoint,
		},
	}

	t, closer, err := cfg.NewTracer()
	if err != nil {
		return nil, nil, err
	}
	return &Tracer{tracer: t}, closer, nil
}

// StartSpanFromContext starts a new tracing span that is either a root one or a
// child of existing one from the provided Context.
func (t *Tracer) StartSpanFromContext(ctx context.Context, operationName string, opts ...opentracing.StartSpanOption) (opentracing.Span, context.Context) {
	if t == nil {
		t = noopTracer
	}

	var span opentracing.Span
	if parentContext := FromContext(ctx); parentContext != nil {
		opts = append(opts, opentracing.ChildOf(parentContext))
		span = t.tracer.StartSpan(operationName, opts...)
	} else {
		span = t.tracer.StartSpan(operationName, opts...)
	}
	return span, WithContext(ctx, span.Context())
}

// AddContextHeader adds a tracing span context to provided p2p Headers from
// the go context. If the tracing span context is not present in go context,
// ErrContextNotFound is returned.
func (t *Tracer) AddContextHeader(ctx context.Context, headers p2p.Headers) error {
	if t == nil {
		t = noopTracer
	}

	c := FromContext(ctx)
	if c == nil {
		return ErrContextNotFound
	}

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	if err := t.tracer.Inject(c, opentracing.Binary, w); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}

	headers[p2p.HeaderNameTracingSpanContext] = b.Bytes()

	return nil
}

// FromHeaders returns tracing span context from p2p Headers. If the tracing
// span context is not present in go context, ErrContextNotFound is returned.
func (t *Tracer) FromHeaders(headers p2p.Headers) (opentracing.SpanContext, error) {
	if t == nil {
		t = noopTracer
	}

	v := headers[p2p.HeaderNameTracingSpanContext]
	if v == nil {
		return nil, ErrContextNotFound
	}
	c, err := t.tracer.Extract(opentracing.Binary, bytes.NewReader(v))
	if err != nil {
		if errors.Is(err, opentracing.ErrSpanContextNotFound) {
			return nil, ErrContextNotFound
		}
		return nil, err
	}

	return c, nil
}

// WithContextFromHeaders returns a new context with injected tracing span
// context if they are found in p2p Headers. If the tracing span context is not
// present in go context, ErrContextNotFound is returned.
func (t *Tracer) WithContextFromHeaders(ctx context.Context, headers p2p.Headers) (context.Context, error) {
	if t == nil {
		t = noopTracer
	}

	c, err := t.FromHeaders(headers)
	if err != nil {
		return ctx, err
	}
	return WithContext(ctx, c), nil
}

// WithContext adds tracing span context to go context.
func WithContext(ctx context.Context, c opentracing.SpanContext) context.Context {
	return context.WithValue(ctx, contextKey, c)
}

// FromContext return tracing span context from go context. If the tracing span
// context is not present in go context, nil is returned.
func FromContext(ctx context.Context) opentracing.SpanContext {
	c, ok := ctx.Value(contextKey).(opentracing.SpanContext)
	if !ok {
		return nil
	}
	return c
}
