// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/transaction/mock"
)

func TestTransactionStoredTransaction(t *testing.T) {
	txHashStr := "0xabcd"
	txHash := common.HexToHash(txHashStr)
	dataStr := "abdd"
	data := common.Hex2Bytes(dataStr)
	created := int64(1616451040)
	recipient := common.HexToAddress("fffe")
	gasPrice := big.NewInt(23)
	gasLimit := uint64(200)
	value := big.NewInt(50)
	nonce := uint64(12)
	description := "test"

	t.Run("found", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return &transaction.StoredTransaction{
						To:          &recipient,
						Created:     created,
						Data:        data,
						GasPrice:    gasPrice,
						GasLimit:    gasLimit,
						Value:       value,
						Nonce:       nonce,
						Description: description,
					}, nil
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/transactions/"+txHashStr, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.TransactionInfo{
				TransactionHash: txHash,
				Created:         time.Unix(created, 0),
				Data:            "0x" + dataStr,
				To:              &recipient,
				GasPrice:        bigint.Wrap(gasPrice),
				GasLimit:        gasLimit,
				Value:           bigint.Wrap(value),
				Nonce:           nonce,
				Description:     description,
			}),
		)
	})

	t.Run("not found", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, transaction.ErrUnknownTransaction
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/transactions/"+txHashStr, http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: debugapi.ErrUnknownTransaction,
				Code:    http.StatusNotFound,
			}))
	})

	t.Run("other errors", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, errors.New("err")
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/transactions/"+txHashStr, http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: debugapi.ErrCantGetTransaction,
				Code:    http.StatusInternalServerError,
			}))
	})
}

func TestTransactionList(t *testing.T) {
	recipient := common.HexToAddress("dfff")
	txHash1 := common.HexToHash("abcd")
	txHash2 := common.HexToHash("efff")
	storedTransactions := map[common.Hash]*transaction.StoredTransaction{
		txHash1: {
			To:          &recipient,
			Created:     1,
			Data:        []byte{1, 2, 3, 4},
			GasPrice:    big.NewInt(12),
			GasLimit:    5345,
			Value:       big.NewInt(4),
			Nonce:       3,
			Description: "test",
		},
		txHash2: {
			To:          &recipient,
			Created:     2,
			Data:        []byte{3, 2, 3, 4},
			GasPrice:    big.NewInt(42),
			GasLimit:    53451,
			Value:       big.NewInt(41),
			Nonce:       32,
			Description: "test2",
		},
	}

	testServer := newTestServer(t, testServerOptions{
		TransactionOpts: []mock.Option{
			mock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
				return []common.Hash{txHash1, txHash2}, nil
			}),
			mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
				return storedTransactions[txHash], nil
			}),
		},
	})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/transactions", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.TransactionPendingList{
			PendingTransactions: []debugapi.TransactionInfo{
				{
					TransactionHash: txHash1,
					Created:         time.Unix(storedTransactions[txHash1].Created, 0),
					Data:            hexutil.Encode(storedTransactions[txHash1].Data),
					To:              storedTransactions[txHash1].To,
					GasPrice:        bigint.Wrap(storedTransactions[txHash1].GasPrice),
					GasLimit:        storedTransactions[txHash1].GasLimit,
					Value:           bigint.Wrap(storedTransactions[txHash1].Value),
					Nonce:           storedTransactions[txHash1].Nonce,
					Description:     storedTransactions[txHash1].Description,
				},
				{
					TransactionHash: txHash2,
					Created:         time.Unix(storedTransactions[txHash2].Created, 0),
					Data:            hexutil.Encode(storedTransactions[txHash2].Data),
					To:              storedTransactions[txHash2].To,
					GasPrice:        bigint.Wrap(storedTransactions[txHash2].GasPrice),
					GasLimit:        storedTransactions[txHash2].GasLimit,
					Value:           bigint.Wrap(storedTransactions[txHash2].Value),
					Nonce:           storedTransactions[txHash2].Nonce,
					Description:     storedTransactions[txHash2].Description,
				},
			},
		}),
	)
}

func TestTransactionListError(t *testing.T) {
	txHash1 := common.HexToHash("abcd")
	t.Run("pending transactions error", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
					return nil, errors.New("err")
				}),
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, nil
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/transactions", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: debugapi.ErrCantGetTransaction,
			}),
		)
	})

	t.Run("pending transactions error", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithPendingTransactionsFunc(func() ([]common.Hash, error) {
					return []common.Hash{txHash1}, nil
				}),
				mock.WithStoredTransactionFunc(func(txHash common.Hash) (*transaction.StoredTransaction, error) {
					return nil, errors.New("error")
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/transactions", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: debugapi.ErrCantGetTransaction,
			}),
		)
	})
}

func TestTransactionResend(t *testing.T) {
	txHash := common.HexToHash("abcd")
	t.Run("ok", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return nil
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/transactions/"+txHash.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(debugapi.TransactionHashResponse{
				TransactionHash: txHash,
			}),
		)
	})

	t.Run("unknown transaction", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return transaction.ErrUnknownTransaction
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/transactions/"+txHash.String(), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: debugapi.ErrUnknownTransaction,
			}),
		)
	})

	t.Run("already imported", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return transaction.ErrAlreadyImported
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/transactions/"+txHash.String(), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: debugapi.ErrAlreadyImported,
			}),
		)
	})

	t.Run("other error", func(t *testing.T) {
		testServer := newTestServer(t, testServerOptions{
			TransactionOpts: []mock.Option{
				mock.WithResendTransactionFunc(func(ctx context.Context, txHash common.Hash) error {
					return errors.New("err")
				}),
			},
		})

		jsonhttptest.Request(t, testServer.Client, http.MethodPost, "/transactions/"+txHash.String(), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: debugapi.ErrCantResendTransaction,
			}),
		)
	})
}
