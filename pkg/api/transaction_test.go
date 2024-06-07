// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

func TestTransactionStoredTransaction(t *testing.T) {
	t.Parallel()

	txHashStr := "0xabcd"
	txHash := common.HexToHash(txHashStr)
	dataStr := "abdd"
	data := common.Hex2Bytes(dataStr)
	created := int64(1616451040)
	recipient := common.HexToAddress("fffe")
	gasPrice := big.NewInt(12)
	gasLimit := uint64(200)
	value := big.NewInt(50)
	nonce := uint64(12)
	description := "test"
	gasTipBoost := 10
	gasTipCap := new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(gasTipBoost)+100), gasPrice), big.NewInt(100))
	t.Run("found", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return &transaction.StoredTransaction{
						To:          &recipient,
						Created:     created,
						Data:        data,
						GasPrice:    gasPrice,
						GasLimit:    gasLimit,
						GasFeeCap:   gasPrice,
						GasTipBoost: gasTipBoost,
						GasTipCap:   gasTipCap,
						Value:       value,
						Nonce:       nonce,
						Description: description,
					}, nil
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/transactions/"+txHashStr, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.TransactionInfo{
				TransactionHash: txHash,
				Created:         time.Unix(created, 0),
				Data:            "0x" + dataStr,
				To:              &recipient,
				GasPrice:        bigint.Wrap(gasPrice),
				GasLimit:        gasLimit,
				GasFeeCap:       bigint.Wrap(gasPrice),
				GasTipCap:       bigint.Wrap(gasTipCap),
				GasTipBoost:     gasTipBoost,
				Value:           bigint.Wrap(value),
				Nonce:           nonce,
				Description:     description,
			}),
		)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, transaction.ErrUnknownTransaction
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/transactions/"+txHashStr, http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: api.ErrUnknownTransaction,
				Code:    http.StatusNotFound,
			}))
	})

	t.Run("other errors", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, errors.New("err")
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/transactions/"+txHashStr, http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: api.ErrCantGetTransaction,
				Code:    http.StatusInternalServerError,
			}))
	})
}

func TestTransactionList(t *testing.T) {
	t.Parallel()

	recipient := common.HexToAddress("dfff")
	txHash1 := common.HexToHash("abcd")
	txHash2 := common.HexToHash("efff")
	storedTransactions := map[common.Hash]*transaction.StoredTransaction{
		txHash1: {
			To:          &recipient,
			Data:        []byte{1, 2, 3, 4},
			GasPrice:    big.NewInt(12),
			GasLimit:    5345,
			GasTipBoost: 10,
			GasTipCap:   new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), big.NewInt(12)), big.NewInt(100)),
			GasFeeCap:   big.NewInt(12),
			Value:       big.NewInt(4),
			Nonce:       3,
			Created:     1,
			Description: "test",
		},
		txHash2: {
			To:          &recipient,
			Created:     2,
			Data:        []byte{3, 2, 3, 4},
			GasPrice:    big.NewInt(42),
			GasTipBoost: 10,
			GasTipCap:   new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), big.NewInt(42)), big.NewInt(100)),
			GasFeeCap:   big.NewInt(42),
			GasLimit:    53451,
			Value:       big.NewInt(41),
			Nonce:       32,
			Description: "test2",
		},
	}

	testServer, _, _, _ := newTestServer(t, testServerOptions{
		TransactionOpts: []mock.Option{
			mock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
				return []common.Hash{txHash1, txHash2}, nil
			}),
			mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
				return storedTransactions[txHash], nil
			}),
		},
	})

	jsonhttptest.Request(t, testServer, http.MethodGet, "/transactions", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(api.TransactionPendingList{
			PendingTransactions: []api.TransactionInfo{
				{
					TransactionHash: txHash1,
					To:              storedTransactions[txHash1].To,
					Nonce:           storedTransactions[txHash1].Nonce,
					GasPrice:        bigint.Wrap(storedTransactions[txHash1].GasPrice),
					GasLimit:        storedTransactions[txHash1].GasLimit,
					GasTipCap:       bigint.Wrap(storedTransactions[txHash1].GasTipCap),
					GasTipBoost:     storedTransactions[txHash1].GasTipBoost,
					GasFeeCap:       bigint.Wrap(storedTransactions[txHash1].GasPrice),
					Data:            hexutil.Encode(storedTransactions[txHash1].Data),
					Created:         time.Unix(storedTransactions[txHash1].Created, 0),
					Description:     storedTransactions[txHash1].Description,
					Value:           bigint.Wrap(storedTransactions[txHash1].Value),
				},
				{
					TransactionHash: txHash2,
					To:              storedTransactions[txHash2].To,
					Nonce:           storedTransactions[txHash2].Nonce,
					GasPrice:        bigint.Wrap(storedTransactions[txHash2].GasPrice),
					GasLimit:        storedTransactions[txHash2].GasLimit,
					GasTipCap:       bigint.Wrap(storedTransactions[txHash2].GasTipCap),
					GasTipBoost:     storedTransactions[txHash2].GasTipBoost,
					GasFeeCap:       bigint.Wrap(storedTransactions[txHash2].GasPrice),
					Data:            hexutil.Encode(storedTransactions[txHash2].Data),
					Created:         time.Unix(storedTransactions[txHash2].Created, 0),
					Description:     storedTransactions[txHash2].Description,
					Value:           bigint.Wrap(storedTransactions[txHash2].Value),
				},
			},
		}),
	)
}

func TestTransactionListError(t *testing.T) {
	t.Parallel()

	txHash1 := common.HexToHash("abcd")
	t.Run("pending transactions error", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
					return nil, errors.New("err")
				}),
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, nil
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/transactions", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: api.ErrCantGetTransaction,
			}),
		)
	})

	t.Run("pending transactions error", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
					return []common.Hash{txHash1}, nil
				}),
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, errors.New("error")
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodGet, "/transactions", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: api.ErrCantGetTransaction,
			}),
		)
	})
}

func TestTransactionResend(t *testing.T) {
	t.Parallel()

	txHash := common.HexToHash("abcd")
	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return nil
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodPost, "/transactions/"+txHash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.TransactionHashResponse{
				TransactionHash: txHash,
			}),
		)
	})

	t.Run("unknown transaction", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return transaction.ErrUnknownTransaction
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodPost, "/transactions/"+txHash.String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: api.ErrUnknownTransaction,
			}),
		)
	})

	t.Run("already imported", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return transaction.ErrAlreadyImported
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodPost, "/transactions/"+txHash.String(), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: api.ErrAlreadyImported,
			}),
		)
	})

	t.Run("other error", func(t *testing.T) {
		t.Parallel()

		testServer, _, _, _ := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return errors.New("err")
				}),
			},
		})

		jsonhttptest.Request(t, testServer, http.MethodPost, "/transactions/"+txHash.String(), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: api.ErrCantResendTransaction,
			}),
		)
	})
}
