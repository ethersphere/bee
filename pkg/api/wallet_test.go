// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

func TestWallet(t *testing.T) {
	t.Parallel()

	t.Run("Okay", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return big.NewInt(10000000000000000), nil
				}),
			},
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(2000000000000000000), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodGet, "/wallet", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.WalletResponse{
				BZZ:         bigint.Wrap(big.NewInt(10000000000000000)),
				NativeToken: bigint.Wrap(big.NewInt(2000000000000000000)),
				ChainID:     1,
			}),
		)
	})

	t.Run("500 - erc20 error", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return new(big.Int), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodGet, "/wallet", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to acquire erc20 balance",
				Code:    500,
			}))
	})

	t.Run("500 - chain backend error", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return new(big.Int), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodGet, "/wallet", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to acquire balance from the chain backend",
				Code:    500,
			}))
	})

	t.Run("swap disabled", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			SwapDisabled: true,
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return big.NewInt(10000000000000000), nil
				}),
			},
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(2000000000000000000), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodGet, "/wallet", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.WalletResponse{
				BZZ:         bigint.Wrap(big.NewInt(10000000000000000)),
				NativeToken: bigint.Wrap(big.NewInt(2000000000000000000)),
				ChainID:     1,
			}),
		)
	})

	t.Run("erc20 service unavailable", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			SwapDisabled:    true,
			Erc20ServiceNil: true,
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(2000000000000000000), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodGet, "/wallet", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.WalletResponse{
				BZZ:         bigint.Wrap(nil),
				NativeToken: bigint.Wrap(big.NewInt(2000000000000000000)),
				ChainID:     1,
			}),
		)
	})

	t.Run("chequebook disabled", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			ChequebookDisabled: true,
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return big.NewInt(10000000000000000), nil
				}),
			},
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(2000000000000000000), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodGet, "/wallet", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.WalletResponse{
				BZZ:         bigint.Wrap(big.NewInt(10000000000000000)),
				NativeToken: bigint.Wrap(big.NewInt(2000000000000000000)),
				ChainID:     1,
			}),
		)
	})
}

func TestWalletWithdraw(t *testing.T) {
	t.Parallel()

	t.Run("address not whitelisted", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BZZ?address=0xaf&amount=99999999", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "provided address not whitelisted",
				Code:    400,
			}))
	})

	t.Run("invalid coin type", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BTC?address=0xaf&amount=99999999", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "only BZZ or NativeToken options are accepted",
				Code:    400,
			}))
	})

	t.Run("BZZ erc20 balance error", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BZZ?address=0xaf&amount=99999999", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to get balance",
				Code:    500,
			}))
	})

	t.Run("BZZ erc20 balance insufficient", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return big.NewInt(88888888), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BZZ?address=0xaf&amount=99999999", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "not enough balance",
				Code:    400,
			}))
	})

	t.Run("BZZ erc20 transfer error", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return big.NewInt(100000000), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BZZ?address=0xaf&amount=99999999", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to transfer amount",
				Code:    500,
			}))
	})

	t.Run("BZZ erc20 transfer ok", func(t *testing.T) {
		t.Parallel()

		txHash := common.HexToHash("0x00f")

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return big.NewInt(100000000), nil
				}),
				erc20mock.WithTransferFunc(func(ctx context.Context, address common.Address, value *big.Int, opts ...transaction.Option) (common.Hash, error) {
					if address != common.HexToAddress("0xaf") {
						t.Fatalf("want addr 0xaf, got %s", address)
					}
					if value.Cmp(big.NewInt(99999999)) != 0 {
						t.Fatalf("want value 99999999, got %s", value)
					}
					if transaction.Apply(opts...).GasLimit != 0 {
						t.Fatalf("omitted gas limit want 0, got %d", transaction.Apply(opts...).GasLimit)
					}
					return txHash, nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BZZ?address=0xaf&amount=99999999", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.WalletTxResponse{
				TransactionHash: txHash,
			}))
	})

	t.Run("BZZ gas limit", func(t *testing.T) {
		t.Parallel()

		txHash := common.HexToHash("0x00f")

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
					return big.NewInt(100000000), nil
				}),
				erc20mock.WithTransferFunc(func(ctx context.Context, address common.Address, value *big.Int, opts ...transaction.Option) (common.Hash, error) {
					if transaction.Apply(opts...).GasLimit != 400000 {
						t.Fatalf("want gas limit 400000, got %d", transaction.Apply(opts...).GasLimit)
					}
					return txHash, nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BZZ?address=0xaf&amount=99999999", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.GasLimitHeader, "400000"),
			jsonhttptest.WithExpectedJSONResponse(api.WalletTxResponse{
				TransactionHash: txHash,
			}))
	})

	t.Run("BZZ invalid gas limit", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			Erc20Opts: []erc20mock.Option{
				erc20mock.WithTransferFunc(func(ctx context.Context, address common.Address, value *big.Int, opts ...transaction.Option) (common.Hash, error) {
					t.Fatal("invalid gas limit reached the transfer")
					return common.Hash{}, nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/BZZ?address=0xaf&amount=99999999", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.GasLimitHeader, "nope"),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid header params",
				Code:    http.StatusBadRequest,
				Reasons: []jsonhttp.Reason{{
					Field: api.GasLimitHeader,
					Error: "invalid syntax",
				}},
			}))
	})

	t.Run("native balance error", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/NativeToken?address=0xaf&amount=99999999", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to acquire balance from the chain backend",
				Code:    500,
			}))
	})

	t.Run("native insufficient balance", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(99999990), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/NativeToken?address=0xaf&amount=99999999", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "not enough balance",
				Code:    400,
			}))
	})

	t.Run("native backend send error", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(100000000), nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/NativeToken?address=0xaf&amount=99999999", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unable to transfer",
				Code:    500,
			}))
	})

	t.Run("native ok", func(t *testing.T) {
		t.Parallel()

		txHash := common.HexToHash("0x00f")

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(100000000), nil
				}),
			},
			TransactionOpts: []transactionmock.Option{
				transactionmock.WithSendFunc(func(ctx context.Context, tx *transaction.TxRequest, opts ...transaction.Option) (common.Hash, error) {
					if tx.Value.Cmp(big.NewInt(99999999)) != 0 {
						t.Fatalf("bad value, want 99999999, got %s", tx.Value)
					}
					if tx.To.Cmp(common.HexToAddress("0xaf")) != 0 {
						t.Fatalf("bad address, want 0xaf, got %s", tx.To)
					}
					if transaction.Apply(opts...).GasLimit != 300000 {
						t.Fatalf("omitted gas limit want 300000, got %d", transaction.Apply(opts...).GasLimit)
					}
					return txHash, nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/NativeToken?address=0xaf&amount=99999999", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.WalletTxResponse{
				TransactionHash: txHash,
			}))
	})

	t.Run("native gas limit", func(t *testing.T) {
		t.Parallel()

		txHash := common.HexToHash("0x00f")

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(100000000), nil
				}),
			},
			TransactionOpts: []transactionmock.Option{
				transactionmock.WithSendFunc(func(ctx context.Context, tx *transaction.TxRequest, opts ...transaction.Option) (common.Hash, error) {
					if transaction.Apply(opts...).GasLimit != 400000 {
						t.Fatalf("want gas limit 400000, got %d", transaction.Apply(opts...).GasLimit)
					}
					return txHash, nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/NativeToken?address=0xaf&amount=99999999", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.GasLimitHeader, "400000"),
			jsonhttptest.WithExpectedJSONResponse(api.WalletTxResponse{
				TransactionHash: txHash,
			}))
	})

	t.Run("native invalid gas limit", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _, _ := newTestServer(t, testServerOptions{
			WhitelistedAddr: "0xaf",
			TransactionOpts: []transactionmock.Option{
				transactionmock.WithSendFunc(func(ctx context.Context, tx *transaction.TxRequest, opts ...transaction.Option) (common.Hash, error) {
					t.Fatal("invalid gas limit reached the transfer")
					return common.Hash{}, nil
				}),
			},
		})

		jsonhttptest.Request(t, srv, http.MethodPost, "/wallet/withdraw/NativeToken?address=0xaf&amount=99999999", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.GasLimitHeader, "nope"),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid header params",
				Code:    http.StatusBadRequest,
				Reasons: []jsonhttp.Reason{{
					Field: api.GasLimitHeader,
					Error: "invalid syntax",
				}},
			}))
	})
}
