package api_test

import (
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	testing2 "github.com/ethersphere/bee/pkg/postage/testing"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"net/http"
	"testing"
)

func TestRedistributionStatus(t *testing.T) {
	t.Parallel()
	addr := test.RandomAddress()
	store := statestore.NewStateStore()
	expectedResponse := storageincentives.NodeStatus{
		State:  "test",
		Round:  2,
		Block:  6,
		Reward: testing2.NewBigInt(),
		Fees:   testing2.NewBigInt(),
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		key := "redistribution_state_" + addr.String()
		err := store.Put(key, expectedResponse)
		if err != nil {
			t.Errorf("redistribution put state: %v", err)
		}
		srv, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:    true,
			StateStorer: store,
			Overlay:     addr,
		})
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json; charset=utf-8"),
			jsonhttptest.WithExpectedJSONResponse(expectedResponse),
		)

	})
	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		srv, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:    true,
			StateStorer: store,
		})
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusNotFound)
	})

}
