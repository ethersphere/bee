// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	signermock "github.com/ethersphere/bee/pkg/crypto/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/monitormock"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
)

func nonceKey(sender common.Address) string {
	return fmt.Sprintf("transaction_nonce_%x", sender)
}

func signerMockForTransaction(signedTx *types.Transaction, sender common.Address, signerChainID *big.Int, t *testing.T) crypto.Signer {
	return signermock.New(
		signermock.WithSignTxFunc(func(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
			if signedTx.To() == nil {
				if transaction.To() != nil {
					t.Fatalf("signing transaction with recipient. wanted nil, got %x", transaction.To())
				}
			} else {
				if transaction.To() == nil || *transaction.To() != *signedTx.To() {
					t.Fatalf("signing transactiono with wrong recipient. wanted %x, got %x", signedTx.To(), transaction.To())
				}
			}
			if !bytes.Equal(transaction.Data(), signedTx.Data()) {
				t.Fatalf("signing transaction with wrong data. wanted %x, got %x", signedTx.Data(), transaction.Data())
			}
			if transaction.Value().Cmp(signedTx.Value()) != 0 {
				t.Fatalf("signing transaction with wrong value. wanted %d, got %d", signedTx.Value(), transaction.Value())
			}
			if chainID.Cmp(signerChainID) != 0 {
				t.Fatalf("signing transaction with wrong chainID. wanted %d, got %d", signerChainID, transaction.ChainId())
			}
			if transaction.Gas() != signedTx.Gas() {
				t.Fatalf("signing transaction with wrong gas. wanted %d, got %d", signedTx.Gas(), transaction.Gas())
			}
			if transaction.GasPrice().Cmp(signedTx.GasPrice()) != 0 {
				t.Fatalf("signing transaction with wrong gasprice. wanted %d, got %d", signedTx.GasPrice(), transaction.GasPrice())
			}

			if transaction.Nonce() != signedTx.Nonce() {
				t.Fatalf("signing transaction with wrong nonce. wanted %d, got %d", signedTx.Nonce(), transaction.Nonce())
			}

			return signedTx, nil
		}),
		signermock.WithEthereumAddressFunc(func() (common.Address, error) {
			return sender, nil
		}),
	)
}

func TestTransactionSend(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xabcd")
	txData := common.Hex2Bytes("0xabcdee")
	value := big.NewInt(1)
	suggestedGasPrice := big.NewInt(2)
	estimatedGasLimit := uint64(3)
	nonce := uint64(2)
	chainID := big.NewInt(5)

	t.Run("send", func(t *testing.T) {
		signedTx := types.NewTransaction(nonce, recipient, value, estimatedGasLimit, suggestedGasPrice, txData)
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  txData,
			Value: value,
		}
		store := storemock.NewStateStore()
		err := store.Put(nonceKey(sender), nonce)
		if err != nil {
			t.Fatal(err)
		}

		transactionService, err := transaction.NewService(logger,
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
					return nonce - 1, nil
				}),
			),
			signerMockForTransaction(signedTx, sender, chainID, t),
			store,
			chainID,
			monitormock.New(),
		)
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

		var storedNonce uint64
		err = store.Get(nonceKey(sender), &storedNonce)
		if err != nil {
			t.Fatal(err)
		}
		if storedNonce != nonce+1 {
			t.Fatalf("nonce not stored correctly: want %d, got %d", nonce+1, storedNonce)
		}
	})

	t.Run("send_no_nonce", func(t *testing.T) {
		signedTx := types.NewTransaction(nonce, recipient, value, estimatedGasLimit, suggestedGasPrice, txData)
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  txData,
			Value: value,
		}
		store := storemock.NewStateStore()

		transactionService, err := transaction.NewService(logger,
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
			signerMockForTransaction(signedTx, sender, chainID, t),
			store,
			chainID,
			monitormock.New(),
		)
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

		var storedNonce uint64
		err = store.Get(nonceKey(sender), &storedNonce)
		if err != nil {
			t.Fatal(err)
		}
		if storedNonce != nonce+1 {
			t.Fatalf("did not store nonce correctly. wanted %d, got %d", nonce+1, storedNonce)
		}
	})

	t.Run("send_skipped_nonce", func(t *testing.T) {
		nextNonce := nonce + 5
		signedTx := types.NewTransaction(nextNonce, recipient, value, estimatedGasLimit, suggestedGasPrice, txData)
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  txData,
			Value: value,
		}
		store := storemock.NewStateStore()
		err := store.Put(nonceKey(sender), nonce)
		if err != nil {
			t.Fatal(err)
		}

		transactionService, err := transaction.NewService(logger,
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
					return nextNonce, nil
				}),
			),
			signerMockForTransaction(signedTx, sender, chainID, t),
			store,
			chainID,
			monitormock.New(),
		)
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

		var storedNonce uint64
		err = store.Get(nonceKey(sender), &storedNonce)
		if err != nil {
			t.Fatal(err)
		}
		if storedNonce != nextNonce+1 {
			t.Fatalf("did not store nonce correctly. wanted %d, got %d", nextNonce+1, storedNonce)
		}
	})

	t.Run("deploy", func(t *testing.T) {
		signedTx := types.NewContractCreation(nonce, value, estimatedGasLimit, suggestedGasPrice, txData)
		request := &transaction.TxRequest{
			To:    nil,
			Data:  txData,
			Value: value,
		}

		transactionService, err := transaction.NewService(logger,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if tx != signedTx {
						t.Fatal("not sending signed transaction")
					}
					return nil
				}),
				backendmock.WithEstimateGasFunc(func(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error) {
					if call.To != nil {
						t.Fatalf("estimating with recipient. wanted nil, got %x", call.To)
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
			signerMockForTransaction(signedTx, sender, chainID, t),
			storemock.NewStateStore(),
			chainID,
			monitormock.New(),
		)
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
	})
}

func TestTransactionWaitForReceipt(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	txHash := common.HexToHash("0xabcdee")
	chainID := big.NewInt(5)
	nonce := uint64(10)

	store := storemock.NewStateStore()
	defer store.Close()

	err := store.Put(transaction.StoredTransactionKey(txHash), transaction.StoredTransaction{
		Nonce: nonce,
	})
	if err != nil {
		t.Fatal(err)
	}

	transactionService, err := transaction.NewService(logger,
		backendmock.New(
			backendmock.WithTransactionReceiptFunc(func(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
				return &types.Receipt{
					TxHash: txHash,
				}, nil
			}),
		),
		signermock.New(),
		store,
		chainID,
		monitormock.New(
			monitormock.WithWatchTransactionFunc(func(txh common.Hash, n uint64) (chan types.Receipt, chan error, error) {
				if nonce != n {
					return nil, nil, fmt.Errorf("nonce mismatch. wanted %d, got %d", nonce, n)
				}
				if txHash != txh {
					return nil, nil, fmt.Errorf("hash mismatch. wanted %x, got %x", txHash, txh)
				}
				receiptC := make(chan types.Receipt, 1)
				receiptC <- types.Receipt{
					TxHash: txHash,
				}
				return receiptC, nil, nil
			}),
		),
	)
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
