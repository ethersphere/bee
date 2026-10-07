// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
)

// TestResponseCodeMetricMethodIsBounded checks that methods other than the
// standard ones are recorded under OTHER.
//
// The collectors are read from the Service directly: the api test server does
// not register them into the /metrics registry, so scraping /metrics would
// show nothing and pass regardless.
func TestResponseCodeMetricMethodIsBounded(t *testing.T) {
	t.Parallel()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	s := api.New(pk.PublicKey, pk.PublicKey, common.Address{}, nil, log.Noop, nil, nil,
		api.FullMode, false, false, nil, nil, inmemstore.New())
	t.Cleanup(func() { _ = s.Close() })
	s.Configure(crypto.NewDefaultSigner(pk), nil, api.Options{}, api.ExtraOptions{}, 1, nil)
	s.Mount()
	s.EnableFullAPI()

	methods := make([]string, 0, 41)
	methods = append(methods, http.MethodGet)
	for i := range 40 {
		methods = append(methods, fmt.Sprintf("BOGUS%d", i))
	}
	for _, m := range methods {
		s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), m, "/health", nil))
	}

	reg := prometheus.NewRegistry()
	for _, c := range s.Metrics() {
		if err := reg.Register(c); err != nil {
			t.Fatal(err)
		}
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	labels := make(map[string]bool)
	for _, fam := range families {
		if !strings.HasSuffix(fam.GetName(), "response_code_count") {
			continue
		}
		for _, m := range fam.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "method" {
					labels[l.GetValue()] = true
				}
			}
		}
	}

	// Without this the test could pass by observing nothing at all.
	if !labels[http.MethodGet] {
		t.Fatal("response_code_count has no GET series; the metric is not being observed")
	}
	for label := range labels {
		if strings.HasPrefix(label, "BOGUS") {
			t.Fatalf("unknown method %q became a metric label", label)
		}
	}
	if !labels["OTHER"] {
		t.Fatal("unknown methods were not recorded under OTHER")
	}
}
