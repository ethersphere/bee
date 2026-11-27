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
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/monitormock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/google/go-cmp/cmp"
)

var (
	minimumTip = big.NewInt(1_500_000_000)
	baseFee    = big.NewInt(3_000_000_000)
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

func checkStoredTransaction(t *testing.T, transactionService transaction.Service, txHash common.Hash, request *transaction.TxRequest, recipient common.Address, gasLimit uint64, gasPrice *big.Int, nonce uint64) {
	t.Helper()

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

	if storedTransaction.GasLimit != gasLimit {
		t.Fatalf("got wrong gas limit in stored transaction. wanted %d, got %d", gasLimit, storedTransaction.GasLimit)
	}

	if gasPrice.Cmp(storedTransaction.GasPrice) != 0 {
		t.Fatalf("got wrong gas price in stored transaction. wanted %d, got %d", gasPrice, storedTransaction.GasPrice)
	}

	if storedTransaction.Nonce != nonce {
		t.Fatalf("got wrong nonce in stored transaction. wanted %d, got %d", nonce, storedTransaction.Nonce)
	}
}

func TestTransactionSend(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xabcd")
	txData := common.Hex2Bytes("0xabcdee")
	value := big.NewInt(1)
	suggestedGasTip := minimumTip
	estimatedGasLimit := uint64(30000)
	gasLimit := estimatedGasLimit + estimatedGasLimit*transaction.GasBufferPercent/100 // added 33% buffer
	nonce := uint64(2)
	chainID := big.NewInt(5)
	gasFeeCap := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), suggestedGasTip)

	t.Run("send", func(t *testing.T) {
		t.Parallel()

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       gasLimit,
			GasFeeCap: gasFeeCap,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					if !bytes.Equal(msg.To.Bytes(), recipient.Bytes()) {
						t.Fatalf("estimating with wrong recipient. wanted %x, got %x", recipient, msg.To)
					}
					if !bytes.Equal(msg.Data, txData) {
						t.Fatal("estimating with wrong data")
					}
					return estimatedGasLimit, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce - 1, nil
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
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
		}

		checkStoredTransaction(t, transactionService, txHash, request, recipient, gasLimit, gasFeeCap, nonce)

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

		// When estimation fails, use MinEstimatedGasLimit without buffer
		gasLimitFallback := estimatedGasLimit

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       gasLimitFallback,
			GasFeeCap: gasFeeCap,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					return 0, errors.New("estimate failure")
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce - 1, nil
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
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
		}

		checkStoredTransaction(t, transactionService, txHash, request, recipient, gasLimitFallback, gasFeeCap, nonce)

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

		multiplier := big.NewInt(int64(transaction.DefaultTipBoostPercent) + 100)
		suggestedGasTipWithBoost := new(big.Int).Div(new(big.Int).Mul(suggestedGasTip, multiplier), big.NewInt(100))
		gasFeeCapWithBoost := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), suggestedGasTipWithBoost)

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       gasLimit,
			GasFeeCap: gasFeeCapWithBoost,
			GasTipCap: suggestedGasTipWithBoost,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					if !bytes.Equal(msg.To.Bytes(), recipient.Bytes()) {
						t.Fatalf("estimating with wrong recipient. wanted %x, got %x", recipient, msg.To)
					}
					if !bytes.Equal(msg.Data, txData) {
						t.Fatal("estimating with wrong data")
					}
					return estimatedGasLimit, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce - 1, nil
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
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCapWithBoost, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
		}

		checkStoredTransaction(t, transactionService, txHash, request, recipient, gasLimit, gasFeeCapWithBoost, nonce)

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
			Gas:       gasLimit,
			GasTipCap: suggestedGasTip,
			GasFeeCap: gasFeeCap,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					if !bytes.Equal(msg.To.Bytes(), recipient.Bytes()) {
						t.Fatalf("estimating with wrong recipient. wanted %x, got %x", recipient, msg.To)
					}
					if !bytes.Equal(msg.Data, txData) {
						t.Fatal("estimating with wrong data")
					}
					return estimatedGasLimit, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
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
			Gas:       gasLimit,
			GasTipCap: suggestedGasTip,
			GasFeeCap: gasFeeCap,
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
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nextNonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})

		txHash, err := transactionService.Send(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(txHash.Bytes(), signedTx.Hash().Bytes()) {
			t.Fatal("returning wrong transaction hash")
		}
	})

	t.Run("send higher tip than fee", func(t *testing.T) {
		t.Parallel()

		customGasFeeCap := big.NewInt(1000) // smaller than tip
		nextNonce := nonce + 5
		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nextNonce,
			To:        &recipient,
			Value:     value,
			Gas:       gasLimit,
			GasTipCap: customGasFeeCap,
			GasFeeCap: customGasFeeCap,
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
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nextNonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return customGasFeeCap, customGasFeeCap, nil
		})

		txHash, err := transactionService.Send(context.Background(), request, transaction.WithGasPrice(customGasFeeCap))
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

		if storedTransaction.GasTipCap.Cmp(customGasFeeCap) != 0 {
			t.Fatalf("got wrong gas tip in stored transaction. wanted %d, got %d", customGasFeeCap, storedTransaction.GasTipCap)
		}
	})

	t.Run("send with contract fallback", func(t *testing.T) {
		t.Parallel()

		// When estimation fails for contract call (has data), use FallbackGasLimit (900k)
		contractData := []byte{0xab, 0xcd, 0xef} // Explicit non-empty data for contract call
		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       transaction.FallbackGasLimit,
			GasFeeCap: gasFeeCap,
			GasTipCap: suggestedGasTip,
			Data:      contractData,
		})
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  contractData,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					return 0, errors.New("estimation failed")
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
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

		if storedTransaction.GasLimit != transaction.FallbackGasLimit {
			t.Fatalf("expected fallback gas limit %d, got %d", transaction.FallbackGasLimit, storedTransaction.GasLimit)
		}
	})

	t.Run("send with simple transfer fallback", func(t *testing.T) {
		t.Parallel()

		// When estimation fails for simple transfer (no data), use MinGasLimit (21k)
		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       transaction.MinGasLimit,
			GasFeeCap: gasFeeCap,
			GasTipCap: suggestedGasTip,
			Data:      nil,
		})
		request := &transaction.TxRequest{
			To:    &recipient,
			Data:  nil,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					return 0, errors.New("estimation failed")
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
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

		if storedTransaction.GasLimit != transaction.MinGasLimit {
			t.Fatalf("expected min gas limit %d, got %d", transaction.MinGasLimit, storedTransaction.GasLimit)
		}
	})

	t.Run("send with max gas limit cap", func(t *testing.T) {
		t.Parallel()

		// When estimation returns value that exceeds MaxGasLimit, cap it
		highEstimate := uint64(15_000_000) // Above MaxGasLimit of 10M
		expectedGasLimit := uint64(transaction.MaxGasLimit)

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       expectedGasLimit,
			GasFeeCap: gasFeeCap,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					return highEstimate, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
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

		if storedTransaction.GasLimit != transaction.MaxGasLimit {
			t.Fatalf("expected max gas limit %d, got %d", transaction.MaxGasLimit, storedTransaction.GasLimit)
		}
	})

	t.Run("send with provided gas limit", func(t *testing.T) {
		t.Parallel()

		// When GasLimit is explicitly provided, use it with bounds validation
		providedGasLimit := uint64(100_000)

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       providedGasLimit,
			GasFeeCap: gasFeeCap,
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
				// EstimateGas should not be called when GasLimit is provided
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					t.Fatal("EstimateGas should not be called when GasLimit is provided")
					return 0, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request, transaction.WithGasLimit(providedGasLimit))
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

		if storedTransaction.GasLimit != providedGasLimit {
			t.Fatalf("expected provided gas limit %d, got %d", providedGasLimit, storedTransaction.GasLimit)
		}
	})

	t.Run("send with MinEstimatedGasLimit enforced after buffer", func(t *testing.T) {
		t.Parallel()

		// When estimated gas + buffer is below MinEstimatedGasLimit, enforce the minimum
		lowEstimate := uint64(50_000)
		minGas := uint64(100_000)
		// lowEstimate + 33% = 66,500, which is < minGas, so minGas should be used

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       minGas,
			GasFeeCap: gasFeeCap,
			GasTipCap: suggestedGasTip,
			Data:      txData,
		})
		request := &transaction.TxRequest{
			To:                   &recipient,
			Data:                 txData,
			Value:                value,
			MinEstimatedGasLimit: minGas,
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
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
					return lowEstimate, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request)
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

		if storedTransaction.GasLimit != minGas {
			t.Fatalf("expected MinEstimatedGasLimit %d, got %d", minGas, storedTransaction.GasLimit)
		}
	})

	t.Run("send with provided gas limit below minimum", func(t *testing.T) {
		t.Parallel()

		// When provided GasLimit is below MinGasLimit, enforce MinGasLimit
		lowGasLimit := uint64(10_000) // Below MinGasLimit of 21k

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       transaction.MinGasLimit,
			GasFeeCap: gasFeeCap,
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
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request, transaction.WithGasLimit(lowGasLimit))
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

		if storedTransaction.GasLimit != transaction.MinGasLimit {
			t.Fatalf("expected min gas limit enforced %d, got %d", transaction.MinGasLimit, storedTransaction.GasLimit)
		}
	})

	t.Run("send with provided gas limit above maximum", func(t *testing.T) {
		t.Parallel()

		// When provided GasLimit is above MaxGasLimit, cap at MaxGasLimit
		highGasLimit := uint64(15_000_000) // Above MaxGasLimit of 10M

		signedTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     value,
			Gas:       transaction.MaxGasLimit,
			GasFeeCap: gasFeeCap,
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
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return nonce, nil
				}),
			),
			signerMockForTransaction(t, signedTx, sender, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFeeCap, suggestedGasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		txHash, err := transactionService.Send(context.Background(), request, transaction.WithGasLimit(highGasLimit))
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

		if storedTransaction.GasLimit != transaction.MaxGasLimit {
			t.Fatalf("expected max gas limit enforced %d, got %d", transaction.MaxGasLimit, storedTransaction.GasLimit)
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
		0,
		0,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
		return nil, nil, errors.New("fees not used")
	})
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
	gasTip := big.NewInt(100)
	gasFee := big.NewInt(1100)
	gasLimit := uint64(100000)
	value := big.NewInt(0)
	gasFeeCap := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), minimumTip)

	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)

	signedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		To:        &recipient,
		Value:     value,
		Gas:       gasLimit,
		GasTipCap: gasTip,
		GasFeeCap: gasFeeCap,
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
		),
		signerMockForTransaction(t, signedTx, recipient, chainID),
		store,
		chainID,
		monitormock.New(),
		0,
		0,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
		return gasFeeCap, gasTip, nil
	})
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

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		fee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), minimumTip)
		gasTipCap := new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), minimumTip), big.NewInt(100))
		gasFeeCap := new(big.Int).Add(fee, gasTipCap)

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
			),
			signerMockForTransaction(t, cancelTx, recipient, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return fee, minimumTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		cancelTxHash, err := transactionService.CancelTransaction(context.Background(), signedTx.Hash(), nil)
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
		gasTipCap := new(big.Int).Div(new(big.Int).Mul(big.NewInt(int64(10)+100), gasTip), big.NewInt(100))
		gasFeeCap := new(big.Int).Add(gasFee, gasTipCap)

		cancelTx := types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			To:        &recipient,
			Value:     big.NewInt(0),
			Gas:       21000,
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTipCap,
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
			),
			signerMockForTransaction(t, cancelTx, recipient, chainID),
			store,
			chainID,
			monitormock.New(),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return gasFee, gasTip, nil
		})
		testutil.CleanupCloser(t, transactionService)

		cancelTxHash, err := transactionService.CancelTransaction(context.Background(), signedTx.Hash(), customGasPrice)
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

func (e *rpcAPIError) ErrorCode() int { return e.code }
func (e *rpcAPIError) Error() string  { return e.msg }
func (e *rpcAPIError) ErrorData() any { return e.err }

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
		0,
		0,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
		return nil, nil, errors.New("fees not used")
	})
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

// TestTransactionService_UnwrapABIError_ShortRevertData is a regression test
// for SLICE-03: an RPC provider that returns empty ("0x") or malformed revert
// data yields a buffer shorter than the 4-byte error selector. The ABI-error
// selector loop must not slice buf[:4] on such a buffer, which would panic.
func TestTransactionService_UnwrapABIError_ShortRevertData(t *testing.T) {
	t.Parallel()

	var (
		sender      = common.HexToAddress("0xddff")
		recipient   = common.HexToAddress("0xbbbddd")
		chainID     = big.NewInt(5)
		txData      = common.Hex2Bytes("0xabcdee")
		value       = big.NewInt(1)
		contractABI = abiutil.MustParseABI(`[{"inputs":[{"internalType":"uint256","name":"available","type":"uint256"},{"internalType":"uint256","name":"required","type":"uint256"}],"name":"InsufficientBalance","type":"error"}]`)
	)

	signedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID: chainID,
		To:      &recipient,
		Value:   value,
		Gas:     21000,
		Data:    txData,
	})
	request := &transaction.TxRequest{To: &recipient, Data: txData, Value: value}

	for _, tc := range []struct {
		name string
		data string // revert data returned by the RPC provider
	}{
		{name: "empty revert data", data: "0x"},
		{name: "shorter than selector", data: "0xabcd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			rpcAPIErr := &rpcAPIError{code: 3, msg: "execution reverted", err: tc.data}

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
				0,
				0,
				0,
			)
			if err != nil {
				t.Fatal(err)
			}
			transaction.SetSuggestFees(transactionService, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
				return nil, nil, errors.New("fees not used")
			})
			testutil.CleanupCloser(t, transactionService)

			originErr := errors.New("origin error")
			// Must not panic on the short buffer, and must still wrap the origin.
			wrappedErr := transactionService.UnwrapABIError(ctx, request, originErr, contractABI.Errors)
			if !errors.Is(wrappedErr, originErr) {
				t.Fatalf("origin error not wrapped, got %v", wrappedErr)
			}
		})
	}
}

func TestTransactionSendAndWait(t *testing.T) {
	t.Parallel()

	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xabcd")
	chainID := big.NewInt(5)

	// newService estimates every call at estimatedGas, records the gas limit
	// of every sent transaction and delivers receipts[nonce] to whoever
	// watches a transaction: both the caller of WaitForReceipt and the
	// pending-transaction watcher that Send starts in the background.
	newService := func(t *testing.T, estimatedGas uint64, receipts []types.Receipt) (transaction.Service, *[]uint64) {
		t.Helper()
		var sentGas []uint64
		store := storemock.NewStateStore()
		testutil.CleanupCloser(t, store)

		service, err := transaction.NewService(log.Noop, sender,
			backendmock.New(
				backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
					sentGas = append(sentGas, tx.Gas())
					return nil
				}),
				backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
					return estimatedGas, nil
				}),
				backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
					return uint64(len(sentGas)), nil
				}),
			),
			signermock.New(
				signermock.WithSignTxFunc(func(tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
					return tx, nil
				}),
				signermock.WithEthereumAddressFunc(func() (common.Address, error) {
					return sender, nil
				}),
			),
			store,
			chainID,
			monitormock.New(
				monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
					if nonce >= uint64(len(receipts)) {
						return nil, nil, fmt.Errorf("no receipt prepared for nonce %d", nonce)
					}
					receipt := receipts[nonce]
					receipt.TxHash = txHash
					receiptC := make(chan types.Receipt, 1)
					receiptC <- receipt
					return receiptC, nil, nil
				}),
			),
			0,
			0,
			0,
		)
		if err != nil {
			t.Fatal(err)
		}
		transaction.SetSuggestFees(service, func(context.Context, transaction.TxOptions) (*big.Int, *big.Int, error) {
			return baseFee, minimumTip, nil
		})
		testutil.CleanupCloser(t, service)
		return service, &sentGas
	}

	const estimatedGas = uint64(300_000)
	bufferedGas := estimatedGas + estimatedGas*transaction.GasBufferPercent/100
	request := func() *transaction.TxRequest {
		return &transaction.TxRequest{To: &recipient, Data: []byte{1, 2, 3}, Value: big.NewInt(0), Description: "call"}
	}

	t.Run("retries once with a doubled limit when the estimated limit ran out", func(t *testing.T) {
		t.Parallel()
		service, sentGas := newService(t, estimatedGas, []types.Receipt{
			{Status: types.ReceiptStatusFailed, GasUsed: bufferedGas},
			{Status: types.ReceiptStatusSuccessful},
		})

		receipt, err := service.SendAndWait(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			t.Fatal("expected the receipt of the successful retry")
		}
		if want := []uint64{bufferedGas, 2 * bufferedGas}; !equalGas(*sentGas, want) {
			t.Fatalf("sent gas limits: got %v, want %v", *sentGas, want)
		}
	})

	t.Run("does not retry a revert that left gas unused", func(t *testing.T) {
		t.Parallel()
		service, sentGas := newService(t, estimatedGas, []types.Receipt{
			{Status: types.ReceiptStatusFailed, GasUsed: bufferedGas - 1},
		})

		_, err := service.SendAndWait(context.Background(), request())
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("got %v, want %v", err, transaction.ErrTransactionReverted)
		}
		if len(*sentGas) != 1 {
			t.Fatalf("got %d sends, want 1", len(*sentGas))
		}
	})

	t.Run("gives up after one retry", func(t *testing.T) {
		t.Parallel()
		service, sentGas := newService(t, estimatedGas, []types.Receipt{
			{Status: types.ReceiptStatusFailed, GasUsed: bufferedGas},
			{Status: types.ReceiptStatusFailed, GasUsed: 2 * bufferedGas},
		})

		_, err := service.SendAndWait(context.Background(), request())
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("got %v, want %v", err, transaction.ErrTransactionReverted)
		}
		if len(*sentGas) != 2 {
			t.Fatalf("got %d sends, want 2", len(*sentGas))
		}
	})

	t.Run("does not retry a gas limit chosen by the caller", func(t *testing.T) {
		t.Parallel()
		service, sentGas := newService(t, estimatedGas, []types.Receipt{
			{Status: types.ReceiptStatusFailed, GasUsed: bufferedGas},
		})

		_, err := service.SendAndWait(context.Background(), request(), transaction.WithGasLimit(bufferedGas))
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("got %v, want %v", err, transaction.ErrTransactionReverted)
		}
		if want := []uint64{bufferedGas}; !equalGas(*sentGas, want) {
			t.Fatalf("sent gas limits: got %v, want %v", *sentGas, want)
		}
	})

	t.Run("caps the retry limit", func(t *testing.T) {
		t.Parallel()
		const bigEstimate = uint64(6_000_000)
		bigBuffered := bigEstimate + bigEstimate*transaction.GasBufferPercent/100
		service, sentGas := newService(t, bigEstimate, []types.Receipt{
			{Status: types.ReceiptStatusFailed, GasUsed: bigBuffered},
			{Status: types.ReceiptStatusSuccessful},
		})

		if _, err := service.SendAndWait(context.Background(), request()); err != nil {
			t.Fatal(err)
		}
		if want := []uint64{bigBuffered, transaction.MaxGasLimit}; !equalGas(*sentGas, want) {
			t.Fatalf("sent gas limits: got %v, want %v", *sentGas, want)
		}
	})

	t.Run("does not retry when the limit is already at the maximum", func(t *testing.T) {
		t.Parallel()
		service, sentGas := newService(t, 8_000_000, []types.Receipt{
			{Status: types.ReceiptStatusFailed, GasUsed: transaction.MaxGasLimit},
			{Status: types.ReceiptStatusSuccessful},
		})

		_, err := service.SendAndWait(context.Background(), request())
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("got error %v, want %v", err, transaction.ErrTransactionReverted)
		}
		if want := []uint64{transaction.MaxGasLimit}; !equalGas(*sentGas, want) {
			t.Fatalf("sent gas limits: got %v, want %v", *sentGas, want)
		}
	})
}

func equalGas(got, want []uint64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSendUsesFeeHistoryForTier(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	sender := common.HexToAddress("0xddff")
	recipient := common.HexToAddress("0xabcd")
	txData := common.Hex2Bytes("0xabcdee")
	value := big.NewInt(1)
	estimatedGasLimit := uint64(30000)
	gasLimit := estimatedGasLimit + estimatedGasLimit*transaction.GasBufferPercent/100
	nonce := uint64(2)
	chainID := big.NewInt(5)
	historyTip := big.NewInt(55)
	historyFeeCap := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), historyTip)

	signedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		To:        &recipient,
		Value:     value,
		Gas:       gasLimit,
		GasFeeCap: historyFeeCap,
		GasTipCap: historyTip,
		Data:      txData,
	})
	var historyCalled bool
	svc, err := transaction.NewService(logger, sender,
		backendmock.New(
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				if tx != signedTx {
					t.Fatal("not sending signed transaction")
				}
				return nil
			}),
			backendmock.WithEstimateGasFunc(func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error) {
				return estimatedGasLimit, nil
			}),
			backendmock.WithPendingNonceAtFunc(func(ctx context.Context, account common.Address) (uint64, error) {
				return nonce - 1, nil
			}),
		),
		signerMockForTransaction(t, signedTx, sender, chainID),
		storemock.NewStateStore(),
		chainID,
		monitormock.New(
			monitormock.WithWatchTransactionFunc(func(txHash common.Hash, nonce uint64) (<-chan types.Receipt, <-chan error, error) {
				return nil, nil, nil
			}),
		),
		0,
		0,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction.SetSuggestFees(svc, func(_ context.Context, txOpts transaction.TxOptions) (*big.Int, *big.Int, error) {
		if txOpts.Fee.Priority != transaction.FeePriorityLow {
			t.Fatalf("got priority %d, want low", txOpts.Fee.Priority)
		}
		historyCalled = true
		return historyFeeCap, historyTip, nil
	})
	testutil.CleanupCloser(t, svc)

	txHash, err := svc.Send(context.Background(), &transaction.TxRequest{
		To:    &recipient,
		Data:  txData,
		Value: value,
	}, transaction.WithFeePriority(transaction.FeePriorityLow), transaction.WithTipBoost(transaction.DefaultTipBoostPercent))
	if err != nil {
		t.Fatal(err)
	}
	if txHash != signedTx.Hash() {
		t.Fatalf("got hash %s, want %s", txHash, signedTx.Hash())
	}
	if !historyCalled {
		t.Fatal("fee history was not queried")
	}

	stored, err := svc.StoredTransaction(txHash)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GasTipCap.Cmp(historyTip) != 0 {
		t.Fatalf("stored tip %s, want %s", stored.GasTipCap, historyTip)
	}
	if stored.FeePriority != transaction.FeePriorityLow {
		t.Fatalf("stored fee priority %d, want low", stored.FeePriority)
	}
	if stored.GasTipBoost != 0 {
		t.Fatalf("stored tip boost %d, want none for a fee priority", stored.GasTipBoost)
	}
}

func newFeeService(t *testing.T, b transaction.Backend, minimumGasTipCap, blocks uint64) transaction.Service {
	t.Helper()
	svc, err := transaction.NewService(
		log.Noop,
		common.Address{},
		b,
		signermock.New(signermock.WithEthereumAddressFunc(func() (common.Address, error) {
			return common.Address{}, nil
		})),
		storemock.NewStateStore(),
		big.NewInt(1),
		monitormock.New(),
		0,
		minimumGasTipCap,
		blocks,
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestSuggestedFeeAndTip(t *testing.T) {
	t.Parallel()

	var (
		ctx              = context.Background()
		minimumGasTipCap = uint64(10)
		baseFee          = big.NewInt(100)
	)

	testCases := []struct {
		name              string
		gasPrice          *big.Int
		boostPercent      int
		mockSuggestGasTip *big.Int
		mockSuggestGasErr error
		mockHeader        *types.Header
		mockHeaderErr     error
		wantGasFeeCap     *big.Int
		wantGasTipCap     *big.Int
		wantErr           error
	}{
		{
			name:          "with gas price",
			gasPrice:      big.NewInt(1000),
			wantGasFeeCap: big.NewInt(1000),
			wantGasTipCap: big.NewInt(1000),
		},
		{
			name:          "with gas price and base fee",
			gasPrice:      big.NewInt(1000),
			mockHeader:    &types.Header{BaseFee: baseFee},
			wantGasFeeCap: big.NewInt(1000),
			wantGasTipCap: big.NewInt(900),
		},
		{
			name:              "suggest tip error",
			mockSuggestGasErr: errors.New("suggest tip error"),
			wantErr:           errors.New("failed to suggest gas tip cap: suggest tip error"),
		},
		{
			name:              "header error",
			mockSuggestGasTip: big.NewInt(20),
			mockHeaderErr:     errors.New("header error"),
			wantErr:           errors.New("failed to get latest block header: header error"),
		},
		{
			name:              "no base fee",
			mockSuggestGasTip: big.NewInt(20),
			mockHeader:        &types.Header{},
			wantErr:           transaction.ErrEIP1559NotSupported,
		},
		{
			name:              "suggested tip > minimum",
			mockSuggestGasTip: big.NewInt(20),
			mockHeader:        &types.Header{BaseFee: baseFee},
			wantGasFeeCap:     big.NewInt(220), // 2*100 + 20
			wantGasTipCap:     big.NewInt(20),
		},
		{
			name:              "suggested tip < minimum",
			mockSuggestGasTip: big.NewInt(5),
			mockHeader:        &types.Header{BaseFee: baseFee},
			wantGasFeeCap:     big.NewInt(210), // 2*100 + 10
			wantGasTipCap:     big.NewInt(10),
		},
		{
			name:              "with boost",
			boostPercent:      10,
			mockSuggestGasTip: big.NewInt(20),
			mockHeader:        &types.Header{BaseFee: baseFee},
			wantGasFeeCap:     big.NewInt(222), // 2*100 + 22
			wantGasTipCap:     big.NewInt(22),  // 20 * 1.1
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := newFeeService(t, backendmock.New(
				backendmock.WithSuggestGasTipCapFunc(func(ctx context.Context) (*big.Int, error) {
					return tc.mockSuggestGasTip, tc.mockSuggestGasErr
				}),
				backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
					return tc.mockHeader, tc.mockHeaderErr
				}),
			), minimumGasTipCap, transaction.DefaultFeeHistoryBlockCount)

			gasFeeCap, gasTipCap, err := backend.SuggestFees(ctx, transaction.WithGasPrice(tc.gasPrice), transaction.WithTipBoost(tc.boostPercent))

			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if err.Error() != tc.wantErr.Error() {
					t.Fatalf("unexpected error. want %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.wantGasFeeCap.String(), gasFeeCap.String()); diff != "" {
				t.Errorf("gasFeeCap mismatch (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tc.wantGasTipCap.String(), gasTipCap.String()); diff != "" {
				t.Errorf("gasTipCap mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// Medians of the two reward rows are low=15, market=55, aggressive=95.
// The last base fee is the next block, so the fee cap is 2*1000 plus the tip.
// A minimum above a history tip raises the tip and the cap; a minimum below it leaves the tip.
func TestSuggestFeeHistory(t *testing.T) {
	t.Parallel()

	history := &ethereum.FeeHistory{
		BaseFee: []*big.Int{big.NewInt(1), big.NewInt(1000)},
		Reward: [][]*big.Int{
			{big.NewInt(10), big.NewInt(50), big.NewInt(90)},
			{big.NewInt(20), big.NewInt(60), big.NewInt(100)},
		},
	}

	for _, tc := range []struct {
		name    string
		minimum uint64
		tier    int
		tip     string
		feeCap  string
	}{
		{name: "no minimum/low", tier: transaction.FeePriorityLow, tip: "15", feeCap: "2015"},
		{name: "no minimum/market", tier: transaction.FeePriorityMarket, tip: "55", feeCap: "2055"},
		{name: "no minimum/aggressive", tier: transaction.FeePriorityAggressive, tip: "95", feeCap: "2095"},
		{name: "minimum above low", minimum: 40, tier: transaction.FeePriorityLow, tip: "40", feeCap: "2040"},
		{name: "minimum below market", minimum: 40, tier: transaction.FeePriorityMarket, tip: "55", feeCap: "2055"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := newFeeService(t, backendmock.New(
				backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
					return history, nil
				}),
				backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
					t.Fatal("fee history already carries the next base fee")
					return nil, nil
				}),
			), tc.minimum, transaction.DefaultFeeHistoryBlockCount)

			feeCap, tip, err := backend.SuggestFees(context.Background(), transaction.WithFeePriority(tc.tier))
			if err != nil {
				t.Fatal(err)
			}
			if tip.String() != tc.tip || feeCap.String() != tc.feeCap {
				t.Fatalf("got tip %s fee cap %s, want %s and %s", tip, feeCap, tc.tip, tc.feeCap)
			}
		})
	}

	t.Run("invalid tier", func(t *testing.T) {
		t.Parallel()

		backend := newFeeService(t, backendmock.New(
			backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
				t.Fatal("invalid tier must be rejected before eth_feeHistory")
				return nil, nil
			}),
		), 0, transaction.DefaultFeeHistoryBlockCount)
		for _, tier := range []int{-1, transaction.FeePriorityAggressive + 1} {
			if _, _, err := backend.SuggestFees(context.Background(), transaction.WithFeePriority(tier)); !errors.Is(err, transaction.ErrUnknownFeePriority) {
				t.Fatalf("tier %d: got %v, want %v", tier, err, transaction.ErrUnknownFeePriority)
			}
		}
	})
}

// eth_feeHistory reports zero base fees on a network without EIP-1559, and a node
// may not implement the method at all. Both return ErrEIP1559NotSupported when the
// latest header has no base fee, and a zero base fee on an EIP-1559 header is used as is.
func TestSuggestFeeHistoryEIP1559(t *testing.T) {
	t.Parallel()

	legacy := &ethereum.FeeHistory{
		BaseFee:      []*big.Int{big.NewInt(0), big.NewInt(0)},
		GasUsedRatio: []float64{0.5},
		Reward:       [][]*big.Int{{big.NewInt(10), big.NewInt(50), big.NewInt(90)}},
	}

	for _, tc := range []struct {
		name       string
		historyErr error
		header     *types.Header
		wantErr    error
		wantFeeCap string
	}{
		{name: "zero base fee without header base fee", header: &types.Header{}, wantErr: transaction.ErrEIP1559NotSupported},
		{name: "zero base fee without header", wantErr: transaction.ErrEIP1559NotSupported},
		{name: "method unsupported without header base fee", historyErr: errors.New("the method eth_feeHistory does not exist"), header: &types.Header{}, wantErr: transaction.ErrEIP1559NotSupported},
		{name: "zero base fee with header base fee", header: &types.Header{BaseFee: big.NewInt(0)}, wantFeeCap: "50"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := newFeeService(t, backendmock.New(
				backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
					if tc.historyErr != nil {
						return nil, tc.historyErr
					}
					return legacy, nil
				}),
				backendmock.WithSuggestGasTipCapFunc(func(context.Context) (*big.Int, error) {
					return big.NewInt(100), nil
				}),
				backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
					return tc.header, nil
				}),
			), 0, transaction.DefaultFeeHistoryBlockCount)

			feeCap, _, err := backend.SuggestFees(context.Background(), transaction.WithFeePriority(transaction.FeePriorityMarket))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if feeCap.String() != tc.wantFeeCap {
				t.Fatalf("got fee cap %s, want %s", feeCap, tc.wantFeeCap)
			}
		})
	}
}

// A failed or unusable eth_feeHistory falls back to the default tip suggestion.
// The boost is 15, DefaultTipBoostPercent, or 80 for low, market, and aggressive.
// An unusable history uses the same fallback. A canceled context does not fall back.
func TestSuggestFeeHistoryFallback(t *testing.T) {
	t.Parallel()

	suggestedTip := big.NewInt(100)
	baseFee := big.NewInt(1000)
	empty := &ethereum.FeeHistory{
		BaseFee:      []*big.Int{big.NewInt(1), new(big.Int).Set(baseFee)},
		GasUsedRatio: []float64{0, 0},
		Reward: [][]*big.Int{
			{big.NewInt(10), big.NewInt(50), big.NewInt(90)},
			{big.NewInt(20), big.NewInt(60), big.NewInt(100)},
		},
	}

	for _, tc := range []struct {
		name  string
		tier  int
		boost int
		empty bool
	}{
		{name: "rpc/low", tier: transaction.FeePriorityLow, boost: 15},
		{name: "rpc/market", tier: transaction.FeePriorityMarket, boost: transaction.DefaultTipBoostPercent},
		{name: "rpc/aggressive", tier: transaction.FeePriorityAggressive, boost: 80},
		{name: "empty blocks/market", tier: transaction.FeePriorityMarket, boost: transaction.DefaultTipBoostPercent, empty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var historyCalls, tipCalls int
			backend := newFeeService(t, backendmock.New(
				backendmock.WithFeeHistoryFunc(func(context.Context, uint64, *big.Int, []float64) (*ethereum.FeeHistory, error) {
					historyCalls++
					if tc.empty {
						return empty, nil
					}
					return nil, errors.New("rpc down")
				}),
				backendmock.WithSuggestGasTipCapFunc(func(context.Context) (*big.Int, error) {
					tipCalls++
					return suggestedTip, nil
				}),
				backendmock.WithHeaderbyNumberFunc(func(context.Context, *big.Int) (*types.Header, error) {
					return &types.Header{BaseFee: baseFee}, nil
				}),
			), 0, transaction.DefaultFeeHistoryBlockCount)

			feeCap, tip, err := backend.SuggestFees(context.Background(), transaction.WithFeePriority(tc.tier))
			if err != nil {
				t.Fatal(err)
			}
			wantTip := new(big.Int).Mul(new(big.Int).Set(suggestedTip), big.NewInt(int64(100+tc.boost)))
			wantTip.Div(wantTip, big.NewInt(100))
			wantFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), wantTip)
			if tip.Cmp(wantTip) != 0 || feeCap.Cmp(wantFee) != 0 {
				t.Fatalf("got tip %s fee cap %s, want %s and %s", tip, feeCap, wantTip, wantFee)
			}
			if historyCalls != 1 || tipCalls != 1 {
				t.Fatalf("history calls %d tip calls %d, want fallback after one history request", historyCalls, tipCalls)
			}
		})
	}

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var tipCalls int
		backend := newFeeService(t, backendmock.New(
			backendmock.WithFeeHistoryFunc(func(ctx context.Context, _ uint64, _ *big.Int, _ []float64) (*ethereum.FeeHistory, error) {
				return nil, ctx.Err()
			}),
			backendmock.WithSuggestGasTipCapFunc(func(context.Context) (*big.Int, error) {
				tipCalls++
				return suggestedTip, nil
			}),
		), 0, transaction.DefaultFeeHistoryBlockCount)

		_, _, err := backend.SuggestFees(ctx, transaction.WithFeePriority(transaction.FeePriorityMarket))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context canceled", err)
		}
		if tipCalls != 0 {
			t.Fatal("canceled context used the fee suggestion fallback")
		}
	})
}
