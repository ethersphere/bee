// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	erc20mock "github.com/ethersphere/bee/pkg/settlement/swap/erc20/mock"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
)

func TestWallet(t *testing.T) {

	t.Run("Okay", func(t *testing.T) {

		erc20SmallUnit, ethSmallUnit := new(big.Int), new(big.Int)
		erc20SmallUnit.SetString(chequebook.Erc20SmallUnitStr, 10)
		ethSmallUnit.SetString(chequebook.EthSmallUnitStr, 10)

		srv := newTestServer(t, testServerOptions{
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return new(big.Int).Mul(erc20SmallUnit, big.NewInt(10)), nil
				}),
			},
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return new(big.Int).Mul(ethSmallUnit, big.NewInt(20)), nil
				}),
			},
			ChainID: 1,
		})

		jsonhttptest.Request(t, srv.Client, http.MethodGet, "/wallet", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.WalletResponse{
				BZZ:     "10",
				XDai:    "20",
				ChainID: 1,
			}),
		)
	})

	t.Run("500 - erc20 error", func(t *testing.T) {
		srv := newTestServer(t, testServerOptions{
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return new(big.Int), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv.Client, http.MethodGet, "/wallet", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to acquire erc20 balance",
				Code:    500,
			}))
	})

	t.Run("500 - chain backend error", func(t *testing.T) {
		srv := newTestServer(t, testServerOptions{
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return new(big.Int), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv.Client, http.MethodGet, "/wallet", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to acquire balance from the chain backend",
				Code:    500,
			}))
	})
}
