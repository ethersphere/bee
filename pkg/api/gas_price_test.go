// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"math/big"
	"net/http"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/api"
	contractMock "github.com/ethersphere/bee/v2/pkg/postage/postagecontract/mock"
	"github.com/ethersphere/bee/v2/pkg/sctx"
)

// TestGasPriceHeaderIsBounded checks that Gas-Price values outside the
// accepted range are rejected before they reach a transaction.
func TestGasPriceHeaderIsBounded(t *testing.T) {
	t.Parallel()

	limit := big.NewInt(1_000_000_000_000) // 1000 gwei

	for _, tc := range []struct {
		name       string
		gasPrice   *big.Int
		wantStatus int
	}{
		{"no header", nil, http.StatusCreated},
		{"at limit", limit, http.StatusCreated},
		{"over limit", new(big.Int).Add(limit, big.NewInt(1)), http.StatusBadRequest},
		{"absurd", new(big.Int).Lsh(big.NewInt(1), 200), http.StatusBadRequest},
		{"negative", big.NewInt(-5), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu       sync.Mutex
				called   bool
				observed *big.Int
			)
			contract := contractMock.New(
				contractMock.WithCreateBatchFunc(func(ctx context.Context, _ *big.Int, _ uint8, _ bool, _ string) (common.Hash, []byte, error) {
					mu.Lock()
					defer mu.Unlock()
					called, observed = true, sctx.GetGasPrice(ctx)
					return common.HexToHash("0xabcd"), []byte{0xab}, nil
				}),
			)
			client, _, _, _, _ := newTestServer(t, testServerOptions{PostageContract: contract})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/stamps/1000/24", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.gasPrice != nil {
				req.Header.Set(api.GasPriceHeader, tc.gasPrice.String())
			}
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()

			if res.StatusCode != tc.wantStatus {
				t.Fatalf("got status %d, want %d", res.StatusCode, tc.wantStatus)
			}

			mu.Lock()
			defer mu.Unlock()
			if tc.wantStatus != http.StatusCreated {
				if called {
					t.Fatal("rejected gas price still reached the contract")
				}
				return
			}
			if tc.gasPrice != nil && observed.Cmp(tc.gasPrice) != 0 {
				t.Fatalf("contract saw gas price %s, want %s", observed, tc.gasPrice)
			}
		})
	}
}

// TestGasPriceHeaderIsBoundedOnCancel covers the second place the header is
// parsed, the transaction cancel handler.
func TestGasPriceHeaderIsBoundedOnCancel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		gasPrice *big.Int
	}{
		{"absurd", new(big.Int).Lsh(big.NewInt(1), 200)},
		{"negative", big.NewInt(-5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, _, _, _, _ := newTestServer(t, testServerOptions{})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/transactions/"+common.HexToHash("0x01").String(), nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(api.GasPriceHeader, tc.gasPrice.String())

			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()

			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("got status %d, want %d", res.StatusCode, http.StatusBadRequest)
			}
		})
	}
}
