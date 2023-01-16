// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	testing2 "github.com/ethersphere/bee/pkg/postage/testing"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"net/http"
	"testing"
)

func TestRedistributionStatus(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		store := statestore.NewStateStore()
		err := store.Put("redistribution_state_", storageincentives.NodeStatus{
			State:  0,
			Round:  2,
			Block:  6,
			Reward: testing2.NewBigInt(),
			Fees:   testing2.NewBigInt(),
		})
		if err != nil {
			t.Errorf("redistribution put state: %v", err)
		}
		srv, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:    true,
			StateStorer: store,
		})
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json; charset=utf-8"),
		)

	})
	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		srv, _, _, _ := newTestServer(t, testServerOptions{})
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusNotFound)
	})

}
