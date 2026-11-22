// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/monitormock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestTxMayBeInMempool(t *testing.T) {
	t.Parallel()

	maybe := []error{
		nil,
		context.DeadlineExceeded,
		context.Canceled,
		errors.New("context deadline exceeded"),
		errors.New("context canceled"),
		errors.New("connection reset by peer"),
		errors.New("EOF"),
		errors.New("unexpected EOF"),
		errors.New("broken pipe"),
		errors.New("i/o timeout"),
		errors.New("Client.Timeout exceeded while awaiting headers"),
		errors.New("Post \"http://rpc\": context deadline exceeded"),
	}
	for _, err := range maybe {
		name := "<nil>"
		if err != nil {
			name = err.Error()
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !transaction.TxMayBeInMempool(err) {
				t.Fatalf("expected the transaction to be possibly in the mempool: %v", err)
			}
		})
	}

	fatal := []error{
		errors.New(""),
		errors.New("nonce too low"),
		errors.New("execution reverted: AlreadyCommitted"),
		errors.New("insufficient funds for gas * price + value"),
		errors.New("already known"),
		errors.New("already imported"),
		errors.New("known transaction"),
		errors.New("connection refused"),
	}
	for _, err := range fatal {
		name := "<nil>"
		if err != nil {
			name = err.Error()
			if name == "" {
				name = "<empty>"
			}
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if transaction.TxMayBeInMempool(err) {
				t.Fatalf("expected a fatal broadcast error: %v", err)
			}
		})
	}
}

func TestSendStoresTransactionWhenItMayBeInMempool(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		context.DeadlineExceeded,
		context.Canceled,
		errors.New("connection reset by peer"),
		errors.New("EOF"),
		errors.New("i/o timeout"),
	} {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()

			var sent *types.Transaction
			svc := newAdmissionService(t, err, func(tx *types.Transaction) { sent = tx }, nil)

			hash, sendErr := svc.Send(context.Background(), admissionRequest(), 0)
			if sendErr != nil {
				t.Fatalf("expected the hash of a transaction that may be in the mempool, got %v", sendErr)
			}
			if sent == nil || hash != sent.Hash() {
				t.Fatalf("got hash %s, sent %v", hash, sent)
			}
			if _, err := svc.StoredTransaction(hash); err != nil {
				t.Fatal(err)
			}
			pending, err := svc.PendingTransactions()
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 || pending[0] != hash {
				t.Fatalf("pending = %v, want [%s]", pending, hash)
			}
		})
	}
}

func TestSendDropsTransactionWhenNodeRejectsIt(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		errors.New("nonce too low"),
		errors.New("execution reverted: AlreadyCommitted"),
		errors.New("insufficient funds for gas * price + value"),
		errors.New("replacement transaction underpriced"),
		errors.New("transaction underpriced"),
		errors.New("already known"),
		errors.New("already imported"),
		errors.New("known transaction"),
	} {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()

			var sent *types.Transaction
			svc := newAdmissionService(t, err, func(tx *types.Transaction) { sent = tx }, nil)

			hash, sendErr := svc.Send(context.Background(), admissionRequest(), 0)
			if sendErr == nil {
				t.Fatal("expected a fatal broadcast error")
			}
			if hash != (common.Hash{}) {
				t.Fatalf("rejected send returned hash %s", hash)
			}
			if sent == nil {
				t.Fatal("transaction was not broadcast")
			}
			if _, storedErr := svc.StoredTransaction(sent.Hash()); !errors.Is(storedErr, transaction.ErrUnknownTransaction) {
				t.Fatalf("stored error = %v", storedErr)
			}
			pending, pendingErr := svc.PendingTransactions()
			if pendingErr != nil {
				t.Fatal(pendingErr)
			}
			if len(pending) != 0 {
				t.Fatalf("pending = %v", pending)
			}
		})
	}
}

func TestResendReportsOnlyPoolRejection(t *testing.T) {
	t.Parallel()

	t.Run("pool rejection", func(t *testing.T) {
		t.Parallel()
		svc, hash := newResendService(t, errors.New("nonce too low"))
		_, err := svc.ResendTransaction(context.Background(), hash, 0)
		if !errors.Is(err, transaction.ErrNonceUsed) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("already imported", func(t *testing.T) {
		t.Parallel()
		svc, hash := newResendService(t, errors.New("already imported"))
		_, err := svc.ResendTransaction(context.Background(), hash, 0)
		if !errors.Is(err, transaction.ErrAlreadyImported) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("ambiguous error", func(t *testing.T) {
		t.Parallel()
		svc, hash := newResendService(t, context.DeadlineExceeded)
		if _, err := svc.ResendTransaction(context.Background(), hash, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.StoredTransaction(hash); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("already known", func(t *testing.T) {
		t.Parallel()
		svc, hash := newResendService(t, errors.New("already known"))
		_, err := svc.ResendTransaction(context.Background(), hash, 0)
		if !errors.Is(err, transaction.ErrAlreadyImported) {
			t.Fatalf("got %v", err)
		}
		if _, err := svc.StoredTransaction(hash); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCancelStoresTransactionWhenItMayBeInMempool(t *testing.T) {
	t.Parallel()

	var sent *types.Transaction
	svc := newAdmissionService(t, context.DeadlineExceeded, func(tx *types.Transaction) { sent = tx }, func(store storage.StateStorer) {
		err := store.Put(transaction.StoredTransactionKey(common.HexToHash("0x11")), transaction.StoredTransaction{
			Nonce:       3,
			To:          &admissionRecipient,
			Value:       big.NewInt(0),
			GasLimit:    50_000,
			GasFeeCap:   big.NewInt(50),
			GasTipCap:   big.NewInt(5),
			Data:        []byte{9},
			Description: "original",
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	hash, err := svc.CancelTransaction(context.Background(), common.HexToHash("0x11"))
	if err != nil {
		t.Fatal(err)
	}
	if sent == nil || hash != sent.Hash() {
		t.Fatalf("got hash %s, sent %v", hash, sent)
	}
	stored, err := svc.StoredTransaction(hash)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Nonce != 3 {
		t.Fatalf("nonce = %d", stored.Nonce)
	}
}

func TestCancelDropsTransactionWhenNodeRejectsIt(t *testing.T) {
	t.Parallel()

	svc := newAdmissionService(t, errors.New("nonce too low"), nil, func(store storage.StateStorer) {
		err := store.Put(transaction.StoredTransactionKey(common.HexToHash("0x22")), transaction.StoredTransaction{
			Nonce:     4,
			To:        &admissionRecipient,
			Value:     big.NewInt(0),
			GasLimit:  50_000,
			GasFeeCap: big.NewInt(50),
			GasTipCap: big.NewInt(5),
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	hash, err := svc.CancelTransaction(context.Background(), common.HexToHash("0x22"))
	if err == nil {
		t.Fatal("expected the pool rejection")
	}
	if hash != (common.Hash{}) {
		t.Fatalf("rejected cancel returned hash %s", hash)
	}
	pending, err := svc.PendingTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v", pending)
	}
}

var admissionRecipient = common.HexToAddress("0xabcd")

func admissionRequest() *transaction.TxRequest {
	return &transaction.TxRequest{
		To:       &admissionRecipient,
		Data:     []byte{1, 2, 3},
		Value:    big.NewInt(0),
		GasLimit: 21_000,
	}
}

func newAdmissionService(t *testing.T, sendErr error, onSend func(*types.Transaction), prepare func(storage.StateStorer)) transaction.Service {
	t.Helper()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	return newSignedService(t, key, sendErr, onSend, prepare)
}

func newResendService(t *testing.T, sendErr error) (transaction.Service, common.Hash) {
	t.Helper()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	chainID := big.NewInt(5)
	feeCap := big.NewInt(100)
	tip := big.NewInt(10)
	signed, err := signer.SignTx(types.NewTx(&types.DynamicFeeTx{
		Nonce:     7,
		ChainID:   chainID,
		To:        &admissionRecipient,
		Value:     big.NewInt(0),
		Gas:       21_000,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Data:      []byte{1, 2, 3},
	}), chainID)
	if err != nil {
		t.Fatal(err)
	}

	svc := newSignedService(t, key, sendErr, nil, func(store storage.StateStorer) {
		err := store.Put(transaction.StoredTransactionKey(signed.Hash()), transaction.StoredTransaction{
			Nonce:     signed.Nonce(),
			To:        signed.To(),
			Value:     signed.Value(),
			GasLimit:  signed.Gas(),
			GasPrice:  signed.GasPrice(),
			GasFeeCap: feeCap,
			GasTipCap: tip,
			Data:      signed.Data(),
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	return svc, signed.Hash()
}

func newSignedService(t *testing.T, key *ecdsa.PrivateKey, sendErr error, onSend func(*types.Transaction), prepare func(storage.StateStorer)) transaction.Service {
	t.Helper()

	store := storemock.NewStateStore()
	testutil.CleanupCloser(t, store)
	if prepare != nil {
		prepare(store)
	}

	chainID := big.NewInt(5)
	signer := crypto.NewDefaultSigner(key)
	sender, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	svc, err := transaction.NewService(
		log.Noop,
		sender,
		backendmock.New(
			backendmock.WithPendingNonceAtFunc(func(context.Context, common.Address) (uint64, error) {
				return 1, nil
			}),
			backendmock.WithSuggestedFeeAndTipFunc(func(context.Context, *big.Int, int) (*big.Int, *big.Int, error) {
				return big.NewInt(100), big.NewInt(10), nil
			}),
			backendmock.WithSendTransactionFunc(func(_ context.Context, tx *types.Transaction) error {
				if onSend != nil {
					onSend(tx)
				}
				return sendErr
			}),
		),
		signer,
		store,
		chainID,
		monitormock.New(
			monitormock.WithWatchTransactionFunc(func(common.Hash, uint64) (<-chan types.Receipt, <-chan error, error) {
				errC := make(chan error, 1)
				errC <- transaction.ErrTransactionCancelled
				return nil, errC, nil
			}),
		),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, svc)
	return svc
}
