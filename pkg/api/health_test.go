// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
)

func TestHealth(t *testing.T) {
	t.Parallel()

	t.Run("probe not set", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{})

		// When probe is not set health endpoint should indicate that node is not healthy
		jsonhttptest.Request(t, testServer, http.MethodGet, "/health", http.StatusOK, jsonhttptest.WithExpectedJSONResponse(api.HealthStatusResponse{
			Status:     "nok",
			Version:    bee.Version,
			APIVersion: api.Version,
		}))
	})

	t.Run("health probe status change", func(t *testing.T) {
		t.Parallel()

		probe := api.NewProbe()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			Probe: probe,
		})

		// Current health probe is pending which should indicate that API is not healthy
		jsonhttptest.Request(t, testServer, http.MethodGet, "/health", http.StatusOK, jsonhttptest.WithExpectedJSONResponse(api.HealthStatusResponse{
			Status:     "nok",
			Version:    bee.Version,
			APIVersion: api.Version,
		}))

		// When we set health probe to OK it should indicate that node is healthy
		probe.SetHealthy(api.ProbeStatusOK)
		jsonhttptest.Request(t, testServer, http.MethodGet, "/health", http.StatusOK, jsonhttptest.WithExpectedJSONResponse(api.HealthStatusResponse{
			Status:     "ok",
			Version:    bee.Version,
			APIVersion: api.Version,
		}))

		// When we set health probe to NOK it should indicate that node is not healthy
		probe.SetHealthy(api.ProbeStatusNOK)
		jsonhttptest.Request(t, testServer, http.MethodGet, "/health", http.StatusOK, jsonhttptest.WithExpectedJSONResponse(api.HealthStatusResponse{
			Status:     "nok",
			Version:    bee.Version,
			APIVersion: api.Version,
		}))
	})
}
