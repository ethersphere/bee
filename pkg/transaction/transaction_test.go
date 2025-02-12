// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	signermock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/monitormock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func signerMockForTransaction(t *testing.T, signedTx *types.Transaction, sender common.Address, signerChainID *big.Int) crypto.Signer {
	t.Helper()
	return signermock.New(
		signermock.WithSignTxFunc(func(transaction *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
			if transaction.Type() != 2 {
				t.Fatalf("wrong transaction type. wanted 2, got %d", transaction.Type())
			}
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

			return signedTx, nil
		}),
		signermock.WithEthereumAddressFunc(func() (common.Address, error) {
			return sender, nil
		}),
	)
}

func TestTransactionSend(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xabcd")
	txData := common.Hex2Bytes("0xabcdee")
	value := big.NewInt(1)
	suggestedGasPrice := big.NewInt(1000)
	suggestedGasTip := big.NewInt(100)
	defaultGasFee := big.NewInt(0).Add(suggestedGasPrice, suggestedGasTip)
	estimatedGasLimit := uint64(3)
	nonce := uint64(2)
	chainID := big.NewInt(5)

	t.Run("send", func(t *testing.T) {
		t.Parallel()

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       estimatedGasLimit,
			GasFeeCap: defaultGasFee,
			GasTipCap: suggestedGasTip,
			Data:      txData,
		})
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  txData,
			Value: value,
		}
		store := storemock.NewStateStore()

		transactionService, err := transaction.NewService(logger, sender,
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
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return suggestedGasTip, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
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
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request, 0)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
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

		if defaultGasFee.Cmp(storedTransaction.GasPrice) != 0 {
			t.Fatalf("got wrong gas price in stored transaction. wanted %d, got %d", defaultGasFee, storedTransaction.GasPrice)
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

	t.Run("send with estimate error", func(t *testing.T) {
		t.Parallel()

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       estimatedGasLimit,
			GasFeeCap: defaultGasFee,
			GasTipCap: suggestedGasTip,
			Data:      txData,
		})
		request := &transaction.TxRequest{
			To:                   &recipient,
			Data:                 txData,
			Value:                value,
			MinEstimatedGasLimit: estimatedGasLimit,
		}
		store := storemock.NewStateStore()

		transactionService, err := transaction.NewService(logger, sender,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if tx != signedTx {
						t.Fatal("not sending signed transaction")
					}
					return nil
				}),
				backendmock.WithEstimateGasFunc(func(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error) {
					return 0, errors.New("estimate failure")
				}),
				backendmock.WithSuggestGasPriceFunc(func(ctx context.Context) (*big.Int, error) {
					return suggestedGasPrice, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce - 1, nil
				}),
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return suggestedGasTip, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
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
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request, 0)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
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

		if defaultGasFee.Cmp(storedTransaction.GasPrice) != 0 {
			t.Fatalf("got wrong gas price in stored transaction. wanted %d, got %d", defaultGasFee, storedTransaction.GasPrice)
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

	t.Run("sendWithBoost", func(t *testing.T) {
		t.Parallel()

		tip := big.NewInt(0).Div(new(big.Int).Mul(suggestedGasTip, big.NewInt(15)), big.NewInt(10))
		fee := big.NewInt(0).Div(new(big.Int).Mul(suggestedGasPrice, big.NewInt(15)), big.NewInt(10))
		fee = fee.Add(fee, tip)
		// tip is the same as suggestedGasPrice and boost is 50%
		// so final gas price will be 2.5x suggestedGasPrice

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       estimatedGasLimit,
			GasFeeCap: fee,
			GasTipCap: tip,
			Data:      txData,
		})
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  txData,
			Value: value,
		}
		store := storemock.NewStateStore()

		transactionService, err := transaction.NewService(logger, sender,
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
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return suggestedGasTip, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
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
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request, 50)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
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

		if fee.Cmp(storedTransaction.GasPrice) != 0 {
			t.Fatalf("got wrong gas price in stored transaction. wanted %d, got %d", fee, storedTransaction.GasPrice)
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
		t.Parallel()

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       estimatedGasLimit,
			GasTipCap: suggestedGasTip,
			GasFeeCap: defaultGasFee,
			Data:      txData,
		})
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  txData,
			Value: value,
		}
		store := storemock.NewStateStore()

		transactionService, err := transaction.NewService(logger, sender,
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
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return suggestedGasTip, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
		)
		if err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request, 0)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
		}
	})

	t.Run("send_skipped_nonce", func(t *testing.T) {
		t.Parallel()

		nextNonce := nonce + 5
		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nextNonce,
			To:        &recipient,
			Value:     value,
			Gas:       estimatedGasLimit,
			GasTipCap: suggestedGasTip,
			GasFeeCap: defaultGasFee,
			Data:      txData,
		})
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  txData,
			Value: value,
		}
		store := storemock.NewStateStore()

		transactionService, err := transaction.NewService(logger, sender,
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
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return suggestedGasTip, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
		)
		if err != nil {
			t.Fatal(err)
		}

		txHash, err := transactionService.Send(context.Background(), request, 0)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
		}
	})
}

func TestTransactionWaitForReceipt(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	sender := common.HexToAddress("0xddff")
	txHash := common.HexToHash("0xabcdee")
	chainID := big.NewInt(5)
	nonce := uint64(10)

	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	err := store.Put(transaction.StoredTransactionKey(txHash), transaction.StoredTransaction{
		Nonce: nonce,
	})
	if err != nil {
		t.Fatal(err)
	}

	transactionService, err := transaction.NewService(logger, sender,
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
	testutil.CleanupCloser(t, transactionService)

	receipt, err := transactionService.WaitForReceipt(context.Background(), txHash)
	if err != nil {
		t.Fatal(err)
	}

	if receipt.TxHash != txHash {
		t.Fatal("got wrong receipt")
	}
}

func TestTransactionResend(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xbbbddd")
	chainID := big.NewInt(5)
	nonce := uint64(10)
	data := []byte{1, 2, 3, 4}
	gasPrice := big.NewInt(1000)
	gasTip := big.NewInt(100)
	gasFee := big.NewInt(1100)
	gasLimit := uint64(100000)
	value := big.NewInt(0)

	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	signedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		To:        &recipient,
		Value:     value,
		Gas:       gasLimit,
		GasTipCap: gasTip,
		GasFeeCap: gasFee,
		Data:      data,
	})

	err := store.Put(transaction.StoredTransactionKey(signedTx.Hash()), transaction.StoredTransaction{
		Nonce:    nonce,
		To:       &recipient,
		Data:     data,
		GasPrice: gasFee,
		GasLimit: gasLimit,
		Value:    value,
	})
	if err != nil {
		t.Fatal(err)
	}

	transactionService, err := transaction.NewService(logger, sender,
		backendmock.New(
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				if tx != signedTx {
					t.Fatal("not sending signed transaction")
				}
				return nil
			}),
			backendmock.WithSuggestGasPriceFunc(func(ctx context.Context) (*big.Int, error) {
				return gasPrice, nil
			}),
			backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
				return gasTip, nil
			}),
		),
		signerMockForTransaction(t, signedTx, recipient, chainID),
		store,
		chainID,
		monitormock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, transactionService)

	err = transactionService.ResendTransaction(context.Background(), signedTx.Hash())
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransactionCancel(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xbbbddd")
	chainID := big.NewInt(5)
	nonce := uint64(10)
	data := []byte{1, 2, 3, 4}
	gasPrice := big.NewInt(1000)
	gasTip := big.NewInt(100)
	gasFee := big.NewInt(1100)
	gasLimit := uint64(100000)
	value := big.NewInt(0)

	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	signedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		To:        &recipient,
		Value:     value,
		Gas:       gasLimit,
		GasFeeCap: gasFee,
		GasTipCap: gasTip,
		Data:      data,
	})
	err := store.Put(transaction.StoredTransactionKey(signedTx.Hash()), transaction.StoredTransaction{
		Nonce:     nonce,
		To:        &recipient,
		Data:      data,
		GasPrice:  gasFee,
		GasLimit:  gasLimit,
		GasFeeCap: gasFee,
		GasTipCap: gasTip,
		Value:     value,
	})
	if err != nil {
		t.Fatal(err)
	}

	gasTipCap := new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), gasTip), big.NewInt(100))
	gasFeeCap := new(big.Int).Add(gasFee, gasTipCap)

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		cancelTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     big.NewInt(0),
			Gas:       21000,
			GasTipCap: gasTipCap,
			GasFeeCap: gasFeeCap,
			Data:      []byte{},
		})

		transactionService, err := transaction.NewService(logger, sender,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if tx != cancelTx {
						t.Fatal("not sending signed transaction")
					}
					return nil
				}),
				backendmock.WithSuggestGasPriceFunc(func(ctx context.Context) (*big.Int, error) {
					return gasPrice, nil
				}),
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return gasTip, nil
				}),
			),
			signerMockForTransaction(t, cancelTx, recipient, chainID),
			store,
			chainID,
			monitormock.New(),
		)
		if err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, transactionService)

		cancelTxHash, err := transactionService.CancelTransaction(context.Background(), signedTx.Hash())
		if err != nil {
			t.Fatal(err)
		}

		if cancelTx.Hash() != cancelTxHash {
			t.Fatalf("returned wrong hash. wanted %v, got %v", cancelTx.Hash(), cancelTxHash)
		}
	})

	t.Run("custom gas price", func(t *testing.T) {
		t.Parallel()

		customGasPrice := big.NewInt(5)

		cancelTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     big.NewInt(0),
			Gas:       21000,
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTip,
			Data:      []byte{},
		})

		transactionService, err := transaction.NewService(logger, sender,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					if tx != cancelTx {
						t.Fatal("not sending signed transaction")
					}
					return nil
				}),
				backendmock.WithSuggestGasPriceFunc(func(ctx context.Context) (*big.Int, error) {
					return gasPrice, nil
				}),
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return gasTip, nil
				}),
			),
			signerMockForTransaction(t, cancelTx, recipient, chainID),
			store,
			chainID,
			monitormock.New(),
		)
		if err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, transactionService)

		ctx := sctx.SetGasPrice(context.Background(), customGasPrice)
		cancelTxHash, err := transactionService.CancelTransaction(ctx, signedTx.Hash())
		if err != nil {
			t.Fatal(err)
		}

		if cancelTx.Hash() != cancelTxHash {
			t.Fatalf("returned wrong hash. wanted %v, got %v", cancelTx.Hash(), cancelTxHash)
		}
	})
}

// rpcAPIError is a copy of engine.EngineAPIError from go-ethereum pkg.
type rpcAPIError struct {
	code int
	msg  string
	err  string
}

func (e *rpcAPIError) ErrorCode() int         { return e.code }
func (e *rpcAPIError) Error() string          { return e.msg }
func (e *rpcAPIError) ErrorData() interface{} { return e.err }

var _ rpc.DataError = (*rpcAPIError)(nil)

func TestTransactionService_UnwrapABIError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var (
		sender    = common.HexToAddress("0xddff")
		recipient = common.HexToAddress("0xbbbddd")
		chainID   = big.NewInt(5)
		nonce     = uint64(10)
		gasTip    = big.NewInt(100)
		gasFee    = big.NewInt(1100)
		txData    = common.Hex2Bytes("0xabcdee")
		value     = big.NewInt(1)

		// This is the ABI of the following contract: https://sepolia.etherscan.io/address/0xd29d9e385f19d888557cd609006bb1934cb5d1e2#code
		contractABI = abiutil.MustParseABI(`[{"inputs":[{"internalType":"uint256","name":"available","type":"uint256"},{"internalType":"uint256","name":"required","type":"uint256"}],"name":"InsufficientBalance","type":"error"},{"inputs":[{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],"name":"transfer","outputs":[],"stateMutability":"nonpayable","type":"function"}]`)
		rpcAPIErr   = &rpcAPIError{
			code: 3,
			msg:  "execution reverted",
			err:  "0xcf4791810000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000006f", // This is the ABI encoded error form the following failed transaction: https://sepolia.etherscan.io/tx/0x74a2577db1c325c41e38977aa1eb32ab03dfa17cc1fa0649e84f3d8c0f0882ee
		}
	)

	gasTipCap := new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), gasTip), big.NewInt(100))
	gasFeeCap := new(big.Int).Add(gasFee, gasTipCap)

	signedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		To:        &recipient,
		Value:     value,
		Gas:       21000,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Data:      txData,
	})
	request := &transaction.TxRequest{
		To:    &recipient,
		Data:  txData,
		Value: value,
	}

	transactionService, err := transaction.NewService(log.Noop, sender,
		backendmock.New(
			backendmock.WithCallContractFunc(func(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
				return nil, rpcAPIErr
			}),
		),
		signerMockForTransaction(t, signedTx, recipient, chainID),
		storemock.NewStateStore(),
		chainID,
		monitormock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, transactionService)

	originErr := errors.New("origin error")
	wrappedErr := transactionService.UnwrapABIError(ctx, request, originErr, contractABI.Errors)
	if !errors.Is(wrappedErr, originErr) {
		t.Fatal("origin error not wrapped")
	}
	if !strings.Contains(wrappedErr.Error(), rpcAPIErr.Error()) {
		t.Fatal("wrapped error without rpc api main error")
	}
	if !strings.Contains(wrappedErr.Error(), "InsufficientBalance(available=0,required=111)") {
		t.Fatal("wrapped error without rpc api error data")
	}
}
