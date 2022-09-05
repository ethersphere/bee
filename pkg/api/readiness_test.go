package api_test

import (
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestReadiness(t *testing.T) {
	t.Parallel()

	t.Run("probe not set", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{})

		// When probe is not set readiness endpoint should indicate that API is not ready
		jsonhttptest.Request(t, testServer, http.MethodGet, "/readiness", http.StatusBadRequest)
	})

	t.Run("readiness probe status change", func(t *testing.T) {
		t.Parallel()

		probe := api.NewProbe()
		testServer, _, _, _ := newTestServer(t, testServerOptions{
			Probe: probe,
		})

		// Current readiness probe is pending which should indicate that API is not ready
		jsonhttptest.Request(t, testServer, http.MethodGet, "/readiness", http.StatusBadRequest)

		// When we set readiness probe to OK it should indicate that API is ready
		probe.SetReady(api.ProbeStatusOK)
		jsonhttptest.Request(t, testServer, http.MethodGet, "/readiness", http.StatusOK)

		// When we set readiness probe to NOK it should indicate that API is not ready
		probe.SetReady(api.ProbeStatusNOK)
		jsonhttptest.Request(t, testServer, http.MethodGet, "/readiness", http.StatusBadRequest)
	})
}
