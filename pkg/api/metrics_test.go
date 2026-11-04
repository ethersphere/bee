// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/prometheus/client_golang/prometheus"
)

func TestToFileSizeBucket(t *testing.T) {
	t.Parallel()

	var want int64 = 300000
	bucket := api.ToFileSizeBucket(want)
	if bucket < want {
		t.Fatalf("bucket should be greater than filesize")
	}

	want = 5000000
	bucket = api.ToFileSizeBucket(want)
	if bucket != want {
		t.Fatalf("bucket should be exactly 5000000")
	}

	overBound := api.FileSizeBucketsKBytes[len(api.FileSizeBucketsKBytes)-1]*1000 + 1
	bucket = api.ToFileSizeBucket(overBound)
	if bucket != api.FileSizeBucketsKBytes[len(api.FileSizeBucketsKBytes)-1]*1000 {
		t.Fatalf("bucket should be the last bucket")
	}
}

func TestMetricsMethod(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		method string
		want   string
	}{
		{method: http.MethodGet, want: http.MethodGet},
		{method: http.MethodHead, want: http.MethodHead},
		{method: http.MethodPost, want: http.MethodPost},
		{method: http.MethodPut, want: http.MethodPut},
		{method: http.MethodPatch, want: http.MethodPatch},
		{method: http.MethodDelete, want: http.MethodDelete},
		{method: http.MethodConnect, want: http.MethodConnect},
		{method: http.MethodOptions, want: http.MethodOptions},
		{method: http.MethodTrace, want: http.MethodTrace},
		{method: "FOO", want: "other"},
		{method: "get", want: "other"},
		{method: "", want: "other"},
	} {
		if got := api.MetricsMethod(tc.method); got != tc.want {
			t.Errorf("method %q: got %q, want %q", tc.method, got, tc.want)
		}
	}
}

// TestResponseCodeMetricsMethodLabel checks that requests with arbitrary HTTP
// methods do not create a new response code time series per method.
func TestResponseCodeMetricsMethodLabel(t *testing.T) {
	t.Parallel()

	client, _, _, _, s := newTestServer(t, testServerOptions{})

	jsonhttptest.Request(t, client, http.MethodGet, "/health", http.StatusOK)
	for i := range 10 {
		jsonhttptest.Request(t, client, fmt.Sprintf("FOO%d", i), "/health", http.StatusOK)
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(s.Metrics()...)
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]float64)
	for _, f := range families {
		if !strings.HasSuffix(f.GetName(), "api_response_code_count") {
			continue
		}
		for _, metric := range f.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "method" {
					got[label.GetValue()] += metric.GetCounter().GetValue()
				}
			}
		}
	}

	want := map[string]float64{http.MethodGet: 1, "other": 10}
	if !maps.Equal(got, want) {
		t.Fatalf("got method label counts %v, want %v", got, want)
	}
}
