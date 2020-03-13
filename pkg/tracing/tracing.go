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
	ErrContextNotFound = errors.New("tracing context not found")

	contextKey   = struct{}{}
	p2pHeaderKey = "tracing-span-context"
	noopTracer   = &Tracer{tracer: new(opentracing.NoopTracer)}
)

type Tracer struct {
	tracer opentracing.Tracer
}

type Options struct {
	Enabled     bool
	Endpoint    string
	ServiceName string
}

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

	headers[p2pHeaderKey] = b.Bytes()

	return nil
}

func (t *Tracer) FromHeaders(headers p2p.Headers) (opentracing.SpanContext, error) {
	if t == nil {
		t = noopTracer
	}

	v := headers[p2pHeaderKey]
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

func WithContext(ctx context.Context, c opentracing.SpanContext) context.Context {
	return context.WithValue(ctx, contextKey, c)
}

func FromContext(ctx context.Context) opentracing.SpanContext {
	c, ok := ctx.Value(contextKey).(opentracing.SpanContext)
	if !ok {
		return nil
	}
	return c
}
