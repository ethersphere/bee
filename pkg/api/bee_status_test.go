// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"crypto/ecdsa"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
)

type testBeeStatus struct {
	code int32
	name string
}

func (t testBeeStatus) StatusCode() int32    { return t.code }
func (t testBeeStatus) StatusString() string { return t.name }

func TestSetBeeStatus(t *testing.T) {
	t.Parallel()

	var unset *api.Service
	if unset.BeeStatusCode() != 0 || unset.BeeStatusString() != "unknown" {
		t.Fatal("nil service must report unknown status")
	}

	s := api.New(
		ecdsa.PublicKey{},
		ecdsa.PublicKey{},
		common.Address{},
		nil,
		log.Noop,
		nil,
		nil,
		api.FullMode,
		false,
		false,
		nil,
		nil,
		nil,
	)

	if s.BeeStatusString() != "unknown" {
		t.Fatalf("got %q, want unknown", s.BeeStatusString())
	}

	s.SetBeeStatus(testBeeStatus{code: 4, name: "opening_localstore"})
	if s.BeeStatusCode() != 4 {
		t.Fatalf("got code %d, want 4", s.BeeStatusCode())
	}
	if s.BeeStatusString() != "opening_localstore" {
		t.Fatalf("got %q, want opening_localstore", s.BeeStatusString())
	}
}

func TestBeeStatusEndpoints(t *testing.T) {
	t.Parallel()

	status := testBeeStatus{code: 4, name: "opening_localstore"}
	probe := api.NewProbe()
	probe.SetHealthy(api.ProbeStatusOK)

	t.Run("available endpoints", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			BeeStatus: status,
			Probe:     probe,
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/node", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.NodeResponse{
				BeeMode:           api.FullMode.String(),
				ChequebookEnabled: true,
				SwapEnabled:       true,
				BeeStatus:         "opening_localstore",
			}),
		)
		jsonhttptest.Request(t, testServer, http.MethodGet, "/health", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.HealthStatusResponse{
				Status:     "ok",
				Version:    bee.Version,
				APIVersion: api.Version,
				BeeStatus:  "opening_localstore",
			}),
		)
	})

	t.Run("status and reservestate before full api", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			BeeStatus:       status,
			FullAPIDisabled: true,
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/status", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.StatusSnapshotResponse{
				Proximity: 256,
				BeeMode:   api.FullMode.String(),
				BeeStatus: "opening_localstore",
			}),
		)
		jsonhttptest.Request(t, testServer, http.MethodGet, "/reservestate", http.StatusOK)
	})

	t.Run("unavailable message", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			BeeStatus:       status,
			FullAPIDisabled: true,
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/bytes/00", http.StatusServiceUnavailable,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusServiceUnavailable,
				Message: "node is not ready: opening_localstore",
			}),
		)
	})
}
