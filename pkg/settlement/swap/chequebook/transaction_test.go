// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
)

func TestTransactionSend(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	recipient := common.HexToAddress("0xabcd")
	signedTx := types.NewTransaction(0, recipient, big.NewInt(0), 0, nil, nil)
	txData := common.Hex2Bytes("0xabcdee")
	value := big.NewInt(1)
	suggestedGasPrice := big.NewInt(2)
	estimatedGasLimit := uint64(3)
	nonce := uint64(2)

	request := &chequebook.TxRequest{
		To:    recipient,
		Data:  txData,
		Value: value,
	}

	transactionService, err := chequebook.NewTransactionService(logger,
		backendmock.New(
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				if tx != signedTx {
					t.Fatal("not sending signed transaction")
				}
				return nil
			}),
			backendmock.WithEstimateGasFunc(func(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error) {
				if !bytes.Equal(call.To.Bytes(), recipient.Bytes()) {
					t.Fatalf("estimating with wrong recipient. wanted %x, got %x", recipient, call.To)
				}
				if !bytes.Equal(call.Data, txData) {
					t.Fatal("estimating with wrong data")
				}
				return estimatedGasLimit, nil
			}),
			backendmock.WithSuggestGasPriceFunc(func(ctx context.Context) (*big.Int, error) {
				return suggestedGasPrice, nil
			}),
			backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
				return nonce, nil
			}),
		),
		&signerMock{
			signTx: func(transaction *types.Transaction) (*types.Transaction, error) {
				if !bytes.Equal(transaction.To().Bytes(), recipient.Bytes()) {
					t.Fatalf("signing transaction with wrong recipient. wanted %x, got %x", recipient, transaction.To())
				}
				if !bytes.Equal(transaction.Data(), txData) {
					t.Fatalf("signing transaction with wrong data. wanted %x, got %x", txData, transaction.Data())
				}
				if transaction.Value().Cmp(value) != 0 {
					t.Fatalf("signing transaction with wrong value. wanted %d, got %d", value, transaction.Value())
				}
				if transaction.Gas() != estimatedGasLimit {
					t.Fatalf("signing transaction with wrong gas. wanted %d, got %d", estimatedGasLimit, transaction.Gas())
				}
				if transaction.GasPrice().Cmp(suggestedGasPrice) != 0 {
					t.Fatalf("signing transaction with wrong gasprice. wanted %d, got %d", suggestedGasPrice, transaction.GasPrice())
				}

				if transaction.Nonce() != nonce {
					t.Fatalf("signing transaction with wrong nonce. wanted %d, got %d", nonce, transaction.Nonce())
				}

				return signedTx, nil
			},
		})
	if err != nil {
		t.Fatal(err)
	}

	txHash, err := transactionService.Send(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
		t.Fatal("returning wrong transaction hash")
	}
}

func TestTransactionWaitForReceipt(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	txHash := common.HexToHash("0xabcdee")

	transactionService, err := chequebook.NewTransactionService(logger,
		backendmock.New(
			backendmock.WithTransactionReceiptFunc(func(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
				return &types.Receipt{
					TxHash: txHash,
				}, nil
			}),
		),
		&signerMock{})
	if err != nil {
		t.Fatal(err)
	}

	receipt, err := transactionService.WaitForReceipt(context.Background(), txHash)
	if err != nil {
		t.Fatal(err)
	}

	if receipt.TxHash != txHash {
		t.Fatal("got wrong receipt")
	}
}
