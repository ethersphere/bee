// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/ethersphere/bee/pkg/transaction/mock"
	"math/big"
	"net/http"
	"testing"
)

func TestRedistributionStatus(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		store := statestore.NewStateStore()
		err := store.Put("redistribution_state", storageincentives.Status{
			Phase: storageincentives.PhaseType(1),
			Round: 1,
			Block: 12,
		})
		if err != nil {
			t.Errorf("redistribution put state: %v", err)
		}
		srv, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:    true,
			StateStorer: store,
			TransactionOpts: []mock.Option{
				mock.WithTransactionFeeFunc(func(ctx context.Context, txHash common.Hash) (*big.Int, error) {
					return big.NewInt(1000), nil
				}),
			},
		})
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusOK,
			jsonhttptest.WithRequestHeader("Content-Type", "application/json; charset=utf-8"),
		)
	})
}
