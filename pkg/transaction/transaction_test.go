// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/ethersphere/bee/pkg/sctx"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/pkg/transaction/monitormock"
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
			monitormock.New(
				monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
					return nil, nil, nil
				}),
			),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer transactionService.Close()

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

		storedTransaction, err := transactionService.StoredTransaction(txHash)
		if err != nil {
			t.Fatal(err)
		}

		if storedTransaction.To == nil || *storedTransaction.To != recipient {
			t.Fatalf("got wrong recipient in stored transaction. wanted %x, got %x", recipient, storedTransaction.To)
		}

		if !bytes.Equal(storedTransaction.Data, request.Data) {
			t.Fatalf("got wrong data in stored transaction. wanted %x, got %x", request.Data, storedTransaction.Data)
		}

		if storedTransaction.Description != request.Description {
			t.Fatalf("got wrong description in stored transaction. wanted %x, got %x", request.Description, storedTransaction.Description)
		}

		if storedTransaction.GasLimit != estimatedGasLimit {
			t.Fatalf("got wrong gas limit in stored transaction. wanted %d, got %d", estimatedGasLimit, storedTransaction.GasLimit)
		}

		if suggestedGasPrice.Cmp(storedTransaction.GasPrice) != 0 {
			t.Fatalf("got wrong gas price in stored transaction. wanted %d, got %d", suggestedGasPrice, storedTransaction.GasPrice)
		}

		if storedTransaction.Nonce != nonce {
			t.Fatalf("got wrong nonce in stored transaction. wanted %d, got %d", nonce, storedTransaction.Nonce)
		}

		pending, err := transactionService.PendingTransactions()
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 1 {
			t.Fatalf("expected one pending transaction, got %d", len(pending))
		}

		if pending[0] != txHash {
			t.Fatalf("got wrong pending transaction. wanted %x, got %x", txHash, pending[0])
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
		defer transactionService.Close()

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
		defer transactionService.Close()

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
			monitormock.WithWatchTransactionFunc(func(txh common.Hash, n uint64) (<-chan types.Receipt, <-chan error, error) {
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
	defer transactionService.Close()

	receipt, err := transactionService.WaitForReceipt(context.Background(), txHash)
	if err != nil {
		t.Fatal(err)
	}

	if receipt.TxHash != txHash {
		t.Fatal("got wrong receipt")
	}
}

func TestTransactionResend(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	recipient := common.HexToAddress("0xbbbddd")
	chainID := big.NewInt(5)
	nonce := uint64(10)
	data := []byte{1, 2, 3, 4}
	gasPrice := big.NewInt(0)
	gasLimit := uint64(100000)
	value := big.NewInt(0)

	store := storemock.NewStateStore()
	defer store.Close()

	signedTx := types.NewTransaction(nonce, recipient, value, gasLimit, gasPrice, data)

	err := store.Put(transaction.StoredTransactionKey(signedTx.Hash()), transaction.StoredTransaction{
		Nonce:    nonce,
		To:       &recipient,
		Data:     data,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
		Value:    value,
	})
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
		),
		signerMockForTransaction(signedTx, recipient, chainID, t),
		store,
		chainID,
		monitormock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transactionService.Close()

	err = transactionService.ResendTransaction(context.Background(), signedTx.Hash())
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransactionCancel(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	recipient := common.HexToAddress("0xbbbddd")
	chainID := big.NewInt(5)
	nonce := uint64(10)
	data := []byte{1, 2, 3, 4}
	gasPrice := big.NewInt(1)
	gasLimit := uint64(100000)
	value := big.NewInt(0)

	store := storemock.NewStateStore()
	defer store.Close()

	signedTx := types.NewTransaction(nonce, recipient, value, gasLimit, gasPrice, data)
	err := store.Put(transaction.StoredTransactionKey(signedTx.Hash()), transaction.StoredTransaction{
		Nonce:    nonce,
		To:       &recipient,
		Data:     data,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
		Value:    value,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		cancelTx := types.NewTransaction(
			nonce,
			recipient,
			big.NewInt(0),
			21000,
			new(big.Int).Add(gasPrice, big.NewInt(1)),
			[]byte{},
		)

		transactionService, err := transaction.NewService(logger,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if tx != cancelTx {
						t.Fatal("not sending signed transaction")
					}
					return nil
				}),
			),
			signerMockForTransaction(cancelTx, recipient, chainID, t),
			store,
			chainID,
			monitormock.New(),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer transactionService.Close()

		cancelTxHash, err := transactionService.CancelTransaction(context.Background(), signedTx.Hash())
		if err != nil {
			t.Fatal(err)
		}

		if cancelTx.Hash() != cancelTxHash {
			t.Fatalf("returned wrong hash. wanted %v, got %v", cancelTx.Hash(), cancelTxHash)
		}
	})

	t.Run("custom gas price", func(t *testing.T) {
		customGasPrice := big.NewInt(5)

		cancelTx := types.NewTransaction(
			nonce,
			recipient,
			big.NewInt(0),
			21000,
			customGasPrice,
			[]byte{},
		)

		transactionService, err := transaction.NewService(logger,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if tx != cancelTx {
						t.Fatal("not sending signed transaction")
					}
					return nil
				}),
			),
			signerMockForTransaction(cancelTx, recipient, chainID, t),
			store,
			chainID,
			monitormock.New(),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer transactionService.Close()

		ctx := sctx.SetGasPrice(context.Background(), customGasPrice)
		cancelTxHash, err := transactionService.CancelTransaction(ctx, signedTx.Hash())
		if err != nil {
			t.Fatal(err)
		}

		if cancelTx.Hash() != cancelTxHash {
			t.Fatalf("returned wrong hash. wanted %v, got %v", cancelTx.Hash(), cancelTxHash)
		}
	})

	t.Run("too low gas price", func(t *testing.T) {
		customGasPrice := big.NewInt(0)

		cancelTx := types.NewTransaction(
			nonce,
			recipient,
			big.NewInt(0),
			21000,
			customGasPrice,
			[]byte{},
		)

		transactionService, err := transaction.NewService(logger,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if tx != cancelTx {
						t.Fatal("not sending signed transaction")
					}
					return nil
				}),
			),
			signerMockForTransaction(cancelTx, recipient, chainID, t),
			store,
			chainID,
			monitormock.New(),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer transactionService.Close()

		ctx := sctx.SetGasPrice(context.Background(), customGasPrice)
		_, err = transactionService.CancelTransaction(ctx, signedTx.Hash())
		if !errors.Is(err, transaction.ErrGasPriceTooLow) {
			t.Fatalf("returned wrong error. wanted %v, got %v", transaction.ErrGasPriceTooLow, err)
		}
	})
}
