// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"gitlab.com/nolash/go-mockbytes"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

// TestTracingHTTPSpan verifies that the HTTP tracing middleware annotates the
// span with the standard server attributes (method, route, status code), marks
// it as a server span, and maps a 5xx response to an Error status while a 2xx
// response leaves the status Unset. These are the fields used to query spans in
// Tempo/Grafana.
func TestTracingHTTPSpan(t *testing.T) {
	t.Parallel()

	const resource = "/bytes"

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	storerMock := mockstorer.New()
	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
		Tracer: tracing.NewTracerFromProvider(tp),
		Logger: log.Noop,
		Post:   mockpost.New(mockpost.WithAcceptAll()),
	})

	// Upload some content so it can be downloaded with a 200 OK.
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(swarm.ChunkSize * 2)
	if err != nil {
		t.Fatal(err)
	}

	var res api.BytesPostResponse
	jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusCreated,
		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		jsonhttptest.WithUnmarshalJSONResponse(&res),
	)

	// Successful download -> 200 OK.
	jsonhttptest.Request(t, client, http.MethodGet, resource+"/"+res.Reference.String(), http.StatusOK,
		jsonhttptest.WithExpectedResponse(content),
	)

	// Invalid address triggers an internal error -> 500.
	jsonhttptest.Request(t, client, http.MethodGet, resource+"/abcd", http.StatusInternalServerError,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: "joiner failed",
			Code:    http.StatusInternalServerError,
		}),
	)

	// span.End() runs in the middleware's deferred call, which can fire after the
	// client has already received the response, so wait for both download spans.
	var spans []sdktrace.ReadOnlySpan
	if err := spinlock.Wait(time.Second, func() bool {
		spans = endedSpansByName(sr, "bytes-download")
		return len(spans) == 2
	}); err != nil {
		t.Fatalf("expected 2 bytes-download spans, got %d", len(spans))
	}

	var ok200, err500 sdktrace.ReadOnlySpan
	for _, s := range spans {
		switch spanStatusCode(s) {
		case http.StatusOK:
			ok200 = s
		case http.StatusInternalServerError:
			err500 = s
		}
	}
	if ok200 == nil || err500 == nil {
		t.Fatalf("missing spans by status code: have ok=%t err=%t", ok200 != nil, err500 != nil)
	}

	// Both spans carry the standard HTTP server attributes and are server-kind.
	for _, s := range []sdktrace.ReadOnlySpan{ok200, err500} {
		if s.SpanKind() != trace.SpanKindServer {
			t.Errorf("span kind = %v, want server", s.SpanKind())
		}
		if got := spanAttrString(s, semconv.HTTPRequestMethodKey); got != http.MethodGet {
			t.Errorf("http.request.method = %q, want %q", got, http.MethodGet)
		}
		if got := spanAttrString(s, semconv.HTTPRouteKey); got != "bytes-download" {
			t.Errorf("http.route = %q, want %q", got, "bytes-download")
		}
	}

	// A 5xx response marks the span as Error; a 2xx response stays Unset.
	if got := err500.Status().Code; got != codes.Error {
		t.Errorf("500 span status = %v, want %v", got, codes.Error)
	}
	if got := ok200.Status().Code; got != codes.Unset {
		t.Errorf("200 span status = %v, want %v", got, codes.Unset)
	}
}

func endedSpansByName(sr *tracetest.SpanRecorder, name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

func spanStatusCode(s sdktrace.ReadOnlySpan) int {
	for _, a := range s.Attributes() {
		if a.Key == semconv.HTTPResponseStatusCodeKey {
			return int(a.Value.AsInt64())
		}
	}
	return 0
}

func spanAttrString(s sdktrace.ReadOnlySpan, key attribute.Key) string {
	for _, a := range s.Attributes() {
		if a.Key == key {
			return a.Value.AsString()
		}
	}
	return ""
}
