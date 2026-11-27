// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
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

func TestResendTransaction(t *testing.T) {
	t.Parallel()

	const (
		nonce       = uint64(7)
		description = "postage"
	)
	data := []byte{1, 2, 3}

	t.Run("market", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)

		resent, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
		if err != nil {
			t.Fatal(err)
		}
		if resent.CurrentHash == env.old || resent.Nonce != nonce || !slices.Equal(resent.PrevHashes, []common.Hash{env.old}) {
			t.Fatalf("chain = %+v", resent)
		}
		sent := env.lastSent()
		if sent.Nonce() != nonce || sent.Gas() != 21_000 || sent.GasFeeCap().Cmp(big.NewInt(200)) != 0 || sent.GasTipCap().Cmp(big.NewInt(20)) != 0 {
			t.Fatalf("sent nonce %d gas %d fee %s tip %s", sent.Nonce(), sent.Gas(), sent.GasFeeCap(), sent.GasTipCap())
		}
		if sent.To() == nil || *sent.To() != admissionRecipient || !slices.Equal(sent.Data(), data) || sent.Value().Cmp(big.NewInt(0)) != 0 {
			t.Fatal("sent transaction content changed")
		}
		stored, err := env.svc.StoredTransaction(resent.CurrentHash)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Description != description {
			t.Fatalf("description = %q", stored.Description)
		}
		if !slices.Contains(env.pending(), resent.CurrentHash) {
			t.Fatal("resent transaction is not pending")
		}
		chain := env.chain(nonce)
		if chain.CurrentHash != resent.CurrentHash || !slices.Equal(chain.PrevHashes, []common.Hash{env.old}) {
			t.Fatalf("stored chain = %+v", chain)
		}
	})

	t.Run("bump from current hash", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)
		env.feeCap, env.tip = big.NewInt(1), big.NewInt(1)

		first, err := env.svc.ResendTransaction(context.Background(), env.old, 20)
		if err != nil {
			t.Fatal(err)
		}
		second, err := env.svc.ResendTransaction(context.Background(), env.old, 20)
		if err != nil {
			t.Fatal(err)
		}
		sent := env.lastSent()
		if sent.GasFeeCap().Cmp(big.NewInt(144)) != 0 || sent.GasTipCap().Cmp(big.NewInt(14)) != 0 {
			t.Fatalf("fee %s tip %s", sent.GasFeeCap(), sent.GasTipCap())
		}
		if !slices.Equal(second.PrevHashes, []common.Hash{env.old, first.CurrentHash}) {
			t.Fatalf("prev = %v", second.PrevHashes)
		}
	})

	t.Run("equal market replaces and watches", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)
		env.feeCap, env.tip = big.NewInt(100), big.NewInt(10)
		if err := env.store.Delete(transaction.PendingTransactionKey(env.old)); err != nil {
			t.Fatal(err)
		}

		before := len(env.watchedHashes())
		resent, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 115, 11)
		pending := env.pending()
		if len(pending) != 1 || pending[0] != resent.CurrentHash {
			t.Fatalf("pending = %v", pending)
		}
		env.waitForWatch(before, resent.CurrentHash)
	})

	t.Run("nonce too low", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)
		env.sendErr = errors.New("nonce too low")

		_, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
		if !errors.Is(err, transaction.ErrNonceUsed) {
			t.Fatalf("got %v", err)
		}
		if _, err := env.svc.StoredTransaction(env.lastSent().Hash()); !errors.Is(err, transaction.ErrUnknownTransaction) {
			t.Fatalf("stored error = %v", err)
		}
		if _, err := env.chainErr(nonce); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("chain error = %v", err)
		}
	})

	for _, msg := range []string{
		"replacement transaction underpriced",
		"ReplacementNotAllowed",
	} {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			env := newResendEnv(t, nonce, description, data, true)
			env.sendErr = errors.New(msg)

			_, err := env.svc.ResendTransaction(context.Background(), env.old, 20)
			if !errors.Is(err, transaction.ErrReplacementUnderpriced) {
				t.Fatalf("got %v", err)
			}
			if _, err := env.svc.StoredTransaction(env.lastSent().Hash()); !errors.Is(err, transaction.ErrUnknownTransaction) {
				t.Fatalf("stored error = %v", err)
			}
			if _, err := env.chainErr(nonce); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("chain error = %v", err)
			}
		})
	}

	for _, msg := range []string{"already known", "already imported"} {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			env := newResendEnv(t, nonce, description, data, true)
			env.sendErr = errors.New(msg)

			_, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
			if !errors.Is(err, transaction.ErrAlreadyImported) {
				t.Fatalf("got %v", err)
			}
			if _, err := env.svc.StoredTransaction(env.old); err != nil {
				t.Fatal(err)
			}
		})
	}

	for _, msg := range []string{"FeeTooLowToCompete", "unknown transaction type"} {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			env := newResendEnv(t, nonce, description, data, true)
			env.sendErr = errors.New(msg)

			_, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
			if err == nil || errors.Is(err, transaction.ErrReplacementUnderpriced) || errors.Is(err, transaction.ErrAlreadyImported) {
				t.Fatalf("got %v", err)
			}
			if _, err := env.svc.StoredTransaction(env.lastSent().Hash()); !errors.Is(err, transaction.ErrUnknownTransaction) {
				t.Fatalf("stored error = %v", err)
			}
		})
	}

	t.Run("lost response", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)
		env.sendErr = context.DeadlineExceeded

		resent, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := env.svc.StoredTransaction(resent.CurrentHash); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(env.pending(), resent.CurrentHash) {
			t.Fatal("resent transaction is not pending")
		}
		if env.chain(nonce).CurrentHash != resent.CurrentHash {
			t.Fatal("chain was not stored")
		}
	})

	t.Run("broadcast uses service context", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := env.svc.ResendTransaction(ctx, env.old, 0); err != nil {
			t.Fatal(err)
		}
		if env.sendCtxErr != nil || !env.sendDeadline {
			t.Fatalf("send context err %v deadline %v", env.sendCtxErr, env.sendDeadline)
		}
	})

	t.Run("hash outside chain", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)
		err := env.store.Put(transaction.ResendTxKey(nonce), &transaction.ResentTransaction{
			Nonce:       nonce,
			CurrentHash: common.HexToHash("0x99"),
		})
		if err != nil {
			t.Fatal(err)
		}

		_, err = env.svc.ResendTransaction(context.Background(), env.old, 0)
		if !errors.Is(err, transaction.ErrNotInResendChain) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("bump without fees", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, description, data, true)
		stored, err := env.svc.StoredTransaction(env.old)
		if err != nil {
			t.Fatal(err)
		}
		stored.GasFeeCap, stored.GasTipCap = nil, nil
		if err := env.store.Put(transaction.StoredTransactionKey(env.old), stored); err != nil {
			t.Fatal(err)
		}

		_, err = env.svc.ResendTransaction(context.Background(), env.old, 20)
		if !errors.Is(err, transaction.ErrMissingFees) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestTransactionReceiptResendChain(t *testing.T) {
	t.Parallel()

	const nonce = uint64(7)
	env := newResendEnv(t, nonce, "postage", []byte{1}, false)
	h1 := env.old
	h2 := common.HexToHash("0x22")
	hCancel := common.HexToHash("0x33")
	env.putStored(h2, nonce, big.NewInt(120), big.NewInt(12))
	env.putStored(hCancel, nonce, big.NewInt(1), big.NewInt(1))
	env.putChain(&transaction.ResentTransaction{Nonce: nonce, CurrentHash: h2, PrevHashes: []common.Hash{h1}})
	env.setReceipts(map[common.Hash]*types.Receipt{
		h2: {TxHash: h2, Status: 1},
	})
	env.setCancelWatch(true)

	t.Run("receipt of another hash", func(t *testing.T) {
		t.Parallel()
		receipt, err := env.svc.TransactionReceipt(context.Background(), h1)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.TxHash != h2 {
			t.Fatalf("receipt = %s", receipt.TxHash)
		}
	})

	t.Run("hash outside the chain", func(t *testing.T) {
		t.Parallel()
		_, err := env.svc.TransactionReceipt(context.Background(), hCancel)
		if !errors.Is(err, ethereum.NotFound) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("wait falls back to the mined hash", func(t *testing.T) {
		t.Parallel()
		receipt, err := env.svc.WaitForReceipt(context.Background(), h1)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.TxHash != h2 {
			t.Fatalf("receipt = %s", receipt.TxHash)
		}
	})

	t.Run("cancellation stays cancelled", func(t *testing.T) {
		t.Parallel()
		_, err := env.svc.WaitForReceipt(context.Background(), hCancel)
		if !errors.Is(err, transaction.ErrTransactionCancelled) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("no chain stays cancelled", func(t *testing.T) {
		t.Parallel()
		alone := newResendEnv(t, nonce, "postage", []byte{1}, false)
		alone.setCancelWatch(true)
		_, err := alone.svc.WaitForReceipt(context.Background(), alone.old)
		if !errors.Is(err, transaction.ErrTransactionCancelled) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("wait for the newer hash falls back to the older", func(t *testing.T) {
		t.Parallel()
		olderFirst := newResendEnv(t, nonce, "postage", []byte{1}, false)
		newer := common.HexToHash("0x44")
		olderFirst.putStored(newer, nonce, big.NewInt(120), big.NewInt(12))
		olderFirst.putChain(&transaction.ResentTransaction{Nonce: nonce, CurrentHash: newer, PrevHashes: []common.Hash{olderFirst.old}})
		olderFirst.setReceipts(map[common.Hash]*types.Receipt{
			olderFirst.old: {TxHash: olderFirst.old, Status: 1},
		})
		olderFirst.setCancelWatch(true)
		receipt, err := olderFirst.svc.WaitForReceipt(context.Background(), newer)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.TxHash != olderFirst.old {
			t.Fatalf("receipt = %s", receipt.TxHash)
		}
	})

	t.Run("receipt lookup error stays a cancellation", func(t *testing.T) {
		t.Parallel()
		broken := newResendEnv(t, nonce, "postage", []byte{1}, false)
		broken.putStored(h2, nonce, big.NewInt(120), big.NewInt(12))
		broken.putChain(&transaction.ResentTransaction{Nonce: nonce, CurrentHash: h2, PrevHashes: []common.Hash{broken.old}})
		rpcErr := errors.New("rpc unavailable")
		broken.setReceiptErr(rpcErr)
		broken.setCancelWatch(true)
		_, err := broken.svc.WaitForReceipt(context.Background(), broken.old)
		if !errors.Is(err, transaction.ErrTransactionCancelled) || !errors.Is(err, rpcErr) {
			t.Fatalf("got %v", err)
		}
	})
}

// TestResentHashMinedAfterNonceReuse covers a resend that is mined after the node
// lost sight of the chain and reused its nonce: the original hash is not cancelled.
func TestResentHashMinedAfterNonceReuse(t *testing.T) {
	t.Parallel()

	env := newResendEnv(t, 1, "postage", []byte{1, 2, 3}, true)
	env.feeCap, env.tip = big.NewInt(1), big.NewInt(1)
	resent, err := env.svc.ResendTransaction(context.Background(), env.old, 20)
	if err != nil {
		t.Fatal(err)
	}

	env.byHash = func(common.Hash) (*types.Transaction, bool, error) {
		return nil, false, ethereum.NotFound
	}
	reused, err := env.svc.Send(context.Background(), admissionRequest(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if env.lastSent().Nonce() != 1 {
		t.Fatalf("nonce = %d", env.lastSent().Nonce())
	}
	if _, err := env.svc.ResendTransaction(context.Background(), env.old, 20); !errors.Is(err, transaction.ErrSuperseded) {
		t.Fatalf("resend got %v", err)
	}

	env.setReceipts(map[common.Hash]*types.Receipt{
		resent.CurrentHash: {TxHash: resent.CurrentHash, Status: types.ReceiptStatusSuccessful},
	})
	env.setCancelWatch(true)
	receipt, err := env.svc.WaitForReceipt(context.Background(), env.old)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.TxHash != resent.CurrentHash {
		t.Fatalf("receipt = %s, want %s", receipt.TxHash, resent.CurrentHash)
	}
	if _, err := env.svc.WaitForReceipt(context.Background(), reused); !errors.Is(err, transaction.ErrTransactionCancelled) {
		t.Fatalf("wait for the reused nonce got %v", err)
	}
}

func TestUnknownHashIsNotLookedUp(t *testing.T) {
	t.Parallel()

	env := newResendEnv(t, 7, "postage", []byte{1}, false)
	unknown := common.HexToHash("0xabc")
	env.receiptErr = errors.New("node was asked")
	env.byHash = func(common.Hash) (*types.Transaction, bool, error) {
		t.Fatal("node was asked for an unknown hash")
		return nil, false, nil
	}

	_, _, err := env.svc.KnownTransaction(context.Background(), unknown)
	if !errors.Is(err, transaction.ErrUnknownTransaction) {
		t.Fatalf("known got %v", err)
	}
	_, err = env.svc.TransactionReceipt(context.Background(), unknown)
	if !errors.Is(err, transaction.ErrUnknownTransaction) {
		t.Fatalf("receipt got %v", err)
	}
	_, err = env.svc.TransactionFee(context.Background(), unknown)
	if !errors.Is(err, transaction.ErrUnknownTransaction) {
		t.Fatalf("fee got %v", err)
	}
}

func TestKnownTransaction(t *testing.T) {
	t.Parallel()

	const nonce = uint64(7)
	env := newResendEnv(t, nonce, "postage", []byte{1}, false)
	h2 := common.HexToHash("0x22")
	outside := common.HexToHash("0x33")
	env.putStored(h2, nonce, big.NewInt(120), big.NewInt(12))
	env.putStored(outside, nonce, big.NewInt(1), big.NewInt(1))
	env.putChain(&transaction.ResentTransaction{Nonce: nonce, CurrentHash: h2, PrevHashes: []common.Hash{env.old}})

	t.Run("pending replacement", func(t *testing.T) {
		var asked []common.Hash
		env.byHash = func(hash common.Hash) (*types.Transaction, bool, error) {
			asked = append(asked, hash)
			if hash == h2 {
				return nil, true, nil
			}
			return nil, false, ethereum.NotFound
		}
		got, pending, err := env.svc.KnownTransaction(context.Background(), env.old)
		if err != nil {
			t.Fatal(err)
		}
		if !pending || got != h2 {
			t.Fatalf("hash %s pending %v", got, pending)
		}
		if !slices.Equal(asked, []common.Hash{h2}) {
			t.Fatalf("asked %v", asked)
		}
	})

	t.Run("mined replacement", func(t *testing.T) {
		var asked []common.Hash
		env.byHash = func(hash common.Hash) (*types.Transaction, bool, error) {
			asked = append(asked, hash)
			if hash == h2 {
				return nil, false, nil
			}
			return nil, false, ethereum.NotFound
		}
		got, pending, err := env.svc.KnownTransaction(context.Background(), env.old)
		if err != nil {
			t.Fatal(err)
		}
		if pending || got != h2 {
			t.Fatalf("hash %s pending %v", got, pending)
		}
		if !slices.Equal(asked, []common.Hash{h2}) {
			t.Fatalf("asked %v", asked)
		}
	})

	t.Run("node knows none", func(t *testing.T) {
		env.byHash = func(common.Hash) (*types.Transaction, bool, error) {
			return nil, false, ethereum.NotFound
		}
		got, pending, err := env.svc.KnownTransaction(context.Background(), env.old)
		if pending || !errors.Is(err, ethereum.NotFound) || got != h2 {
			t.Fatalf("hash %s pending %v err %v", got, pending, err)
		}
	})

	t.Run("node error", func(t *testing.T) {
		rpcErr := errors.New("rpc unavailable")
		var asked []common.Hash
		env.byHash = func(hash common.Hash) (*types.Transaction, bool, error) {
			asked = append(asked, hash)
			return nil, false, rpcErr
		}
		_, _, err := env.svc.KnownTransaction(context.Background(), env.old)
		if !errors.Is(err, rpcErr) {
			t.Fatalf("got %v", err)
		}
		if !slices.Equal(asked, []common.Hash{h2}) {
			t.Fatalf("asked %v", asked)
		}
	})

	t.Run("hash outside the chain", func(t *testing.T) {
		env.byHash = func(hash common.Hash) (*types.Transaction, bool, error) {
			if hash == outside {
				return nil, true, nil
			}
			return nil, false, ethereum.NotFound
		}
		got, pending, err := env.svc.KnownTransaction(context.Background(), outside)
		if err != nil {
			t.Fatal(err)
		}
		if !pending || got != outside {
			t.Fatalf("hash %s pending %v", got, pending)
		}
	})
}

func TestTransactionFeeResendChain(t *testing.T) {
	t.Parallel()

	const nonce = uint64(7)
	env := newResendEnv(t, nonce, "postage", []byte{1}, false)
	h2 := common.HexToHash("0x22")
	priced := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		ChainID:   big.NewInt(5),
		To:        &admissionRecipient,
		Value:     big.NewInt(0),
		Gas:       21_000,
		GasTipCap: big.NewInt(10),
		GasFeeCap: big.NewInt(100),
	})
	env.putStored(h2, nonce, big.NewInt(100), big.NewInt(10))
	env.putChain(&transaction.ResentTransaction{Nonce: nonce, CurrentHash: h2, PrevHashes: []common.Hash{env.old}})
	env.setReceipts(map[common.Hash]*types.Receipt{h2: {TxHash: h2, Status: 1}})
	env.byHash = func(hash common.Hash) (*types.Transaction, bool, error) {
		if hash != h2 {
			return nil, false, ethereum.NotFound
		}
		return priced, false, nil
	}

	fee, err := env.svc.TransactionFee(context.Background(), env.old)
	if err != nil {
		t.Fatal(err)
	}
	if fee.Cmp(priced.Cost()) != 0 {
		t.Fatalf("fee = %s, want %s", fee, priced.Cost())
	}

	pendingOld := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		ChainID:   big.NewInt(5),
		To:        &admissionRecipient,
		Value:     big.NewInt(0),
		Gas:       21_000,
		GasTipCap: big.NewInt(20),
		GasFeeCap: big.NewInt(200),
	})
	env.setReceipts(nil)
	env.byHash = func(hash common.Hash) (*types.Transaction, bool, error) {
		if hash != env.old {
			return nil, false, ethereum.NotFound
		}
		return pendingOld, true, nil
	}

	fee, err = env.svc.TransactionFee(context.Background(), h2)
	if err != nil {
		t.Fatal(err)
	}
	if fee.Cmp(pendingOld.Cost()) != 0 {
		t.Fatalf("fee of pending older hash = %s, want %s", fee, pendingOld.Cost())
	}
}

func TestResendPullsFeesUpToCurrent(t *testing.T) {
	t.Parallel()

	const nonce = uint64(7)

	t.Run("mixed market", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1, 2, 3}, true)
		env.feeCap, env.tip = big.NewInt(120), big.NewInt(9)

		resent, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 120, 11)
		if resent.CurrentHash == env.old || !slices.Equal(resent.PrevHashes, []common.Hash{env.old}) {
			t.Fatalf("chain = %+v", resent)
		}
	})

	t.Run("market above the bump", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1, 2, 3}, true)
		env.feeCap, env.tip = big.NewInt(300), big.NewInt(30)

		if _, err := env.svc.ResendTransaction(context.Background(), env.old, 20); err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 300, 30)
	})

	t.Run("fee priority", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1, 2, 3}, true)
		stored, err := env.svc.StoredTransaction(env.old)
		if err != nil {
			t.Fatal(err)
		}
		stored.FeePriority = transaction.FeePriorityMarket
		if err := env.store.Put(transaction.StoredTransactionKey(env.old), stored); err != nil {
			t.Fatal(err)
		}
		env.historyFee, env.historyTip = big.NewInt(90), big.NewInt(9)

		if _, err := env.svc.ResendTransaction(context.Background(), env.old, 0); err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 115, 11)
	})

	t.Run("small tip rounds up", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1, 2, 3}, true)
		stored, err := env.svc.StoredTransaction(env.old)
		if err != nil {
			t.Fatal(err)
		}
		stored.GasTipCap = big.NewInt(5)
		if err := env.store.Put(transaction.StoredTransactionKey(env.old), stored); err != nil {
			t.Fatal(err)
		}
		env.feeCap, env.tip = big.NewInt(1), big.NewInt(1)

		if _, err := env.svc.ResendTransaction(context.Background(), env.old, 10); err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 110, 6)
	})

	t.Run("missing fees use the market", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1, 2, 3}, true)
		stored, err := env.svc.StoredTransaction(env.old)
		if err != nil {
			t.Fatal(err)
		}
		stored.GasFeeCap, stored.GasTipCap = nil, nil
		if err := env.store.Put(transaction.StoredTransactionKey(env.old), stored); err != nil {
			t.Fatal(err)
		}

		if _, err := env.svc.ResendTransaction(context.Background(), env.old, 0); err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 200, 20)
	})

	t.Run("cancellation does not raise the baseline", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1, 2, 3}, true)
		cancel := common.HexToHash("0x33")
		env.putStored(cancel, nonce, big.NewInt(500), big.NewInt(50))
		env.putChain(&transaction.ResentTransaction{Nonce: nonce, CurrentHash: env.old})
		env.feeCap, env.tip = big.NewInt(1), big.NewInt(1)
		env.sendErr = errors.New("replacement transaction underpriced")

		_, err := env.svc.ResendTransaction(context.Background(), env.old, 0)
		if !errors.Is(err, transaction.ErrReplacementUnderpriced) {
			t.Fatalf("got %v", err)
		}
		assertFees(t, env.lastSent(), 115, 11)
		if _, err := env.svc.StoredTransaction(env.lastSent().Hash()); !errors.Is(err, transaction.ErrUnknownTransaction) {
			t.Fatalf("stored error = %v", err)
		}
	})
}

func TestStaleHashAfterNonceReuse(t *testing.T) {
	t.Parallel()

	const nonce = uint64(1)
	env := newResendEnv(t, nonce, "postage", []byte{1, 2, 3}, true)
	previous := common.HexToHash("0x10")
	older := common.HexToHash("0xaa")
	env.putStored(previous, nonce, big.NewInt(80), big.NewInt(8))
	env.putStored(older, nonce, big.NewInt(70), big.NewInt(7))
	env.putChain(&transaction.ResentTransaction{
		Nonce:       nonce,
		CurrentHash: env.old,
		PrevHashes:  []common.Hash{previous},
		Superseded:  [][]common.Hash{{older}},
	})
	env.byHash = func(common.Hash) (*types.Transaction, bool, error) {
		return nil, false, ethereum.NotFound
	}

	replacement, err := env.svc.Send(context.Background(), admissionRequest(), 0)
	if err != nil {
		t.Fatal(err)
	}
	chain := env.chain(nonce)
	if chain.CurrentHash != replacement || len(chain.PrevHashes) != 0 {
		t.Fatalf("owner = %+v", chain)
	}
	want := [][]common.Hash{{older}, {previous, env.old}}
	if len(chain.Superseded) != len(want) || !slices.Equal(chain.Superseded[0], want[0]) || !slices.Equal(chain.Superseded[1], want[1]) {
		t.Fatalf("superseded = %v, want %v", chain.Superseded, want)
	}

	for _, hash := range []common.Hash{env.old, previous, older} {
		if _, err := env.svc.ResendTransaction(context.Background(), hash, 20); !errors.Is(err, transaction.ErrSuperseded) {
			t.Fatalf("resend %s got %v", hash, err)
		}
	}
	if _, err := env.svc.CancelTransaction(context.Background(), env.old); !errors.Is(err, transaction.ErrSuperseded) {
		t.Fatalf("cancel got %v", err)
	}
	if env.sentCount() != 1 {
		t.Fatalf("broadcast %d transactions", env.sentCount())
	}

	resent, err := env.svc.ResendTransaction(context.Background(), replacement, 20)
	if err != nil {
		t.Fatal(err)
	}
	assertFees(t, env.lastSent(), 240, 24)
	if resent.CurrentHash == replacement || !slices.Equal(resent.PrevHashes, []common.Hash{replacement}) {
		t.Fatalf("chain = %+v", resent)
	}

	env.setReceipts(map[common.Hash]*types.Receipt{
		replacement: {TxHash: replacement, Status: types.ReceiptStatusSuccessful},
	})
	env.setCancelWatch(true)
	if _, err := env.svc.WaitForReceipt(context.Background(), env.old); !errors.Is(err, transaction.ErrTransactionCancelled) {
		t.Fatalf("wait got %v", err)
	}
	if _, err := env.svc.TransactionReceipt(context.Background(), env.old); !errors.Is(err, ethereum.NotFound) {
		t.Fatalf("receipt got %v", err)
	}
}

func TestCancelFees(t *testing.T) {
	t.Parallel()

	const nonce = uint64(7)

	t.Run("fee cap rises with the tip", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1}, false)
		env.putStored(env.old, nonce, big.NewInt(201), big.NewInt(1))
		env.feeCap, env.tip = big.NewInt(1), big.NewInt(1)

		if _, err := env.svc.CancelTransaction(context.Background(), env.old); err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 231, 2)
	})

	t.Run("market above current", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1}, false)
		env.feeCap, env.tip = big.NewInt(300), big.NewInt(30)

		if _, err := env.svc.CancelTransaction(context.Background(), env.old); err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 300, 30)
	})

	t.Run("small tip rounds up", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1}, false)
		env.putStored(env.old, nonce, big.NewInt(100), big.NewInt(5))
		env.feeCap, env.tip = big.NewInt(1), big.NewInt(1)

		if _, err := env.svc.CancelTransaction(context.Background(), env.old); err != nil {
			t.Fatal(err)
		}
		assertFees(t, env.lastSent(), 115, 6)
	})

	t.Run("broadcast uses service context", func(t *testing.T) {
		t.Parallel()
		env := newResendEnv(t, nonce, "postage", []byte{1}, false)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := env.svc.CancelTransaction(ctx, env.old); err != nil {
			t.Fatal(err)
		}
		if env.sendCtxErr != nil || !env.sendDeadline {
			t.Fatalf("send context err %v deadline %v", env.sendCtxErr, env.sendDeadline)
		}
	})

	for _, tc := range []struct {
		msg  string
		want error
	}{
		{"nonce too low", transaction.ErrNonceUsed},
		{"replacement transaction underpriced", transaction.ErrReplacementUnderpriced},
		{"ReplacementNotAllowed", transaction.ErrReplacementUnderpriced},
		{"already known", transaction.ErrAlreadyImported},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			t.Parallel()
			env := newResendEnv(t, nonce, "postage", []byte{1}, false)
			env.sendErr = errors.New(tc.msg)

			hash, err := env.svc.CancelTransaction(context.Background(), env.old)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v", err)
			}
			if hash != (common.Hash{}) {
				t.Fatalf("rejected cancel returned hash %s", hash)
			}
			if len(env.pending()) != 0 {
				t.Fatalf("pending = %v", env.pending())
			}
		})
	}
}

func TestCancelStaysOutsideChain(t *testing.T) {
	t.Parallel()

	const nonce = uint64(7)
	env := newResendEnv(t, nonce, "postage", []byte{1}, false)
	h2 := common.HexToHash("0x22")
	env.putStored(h2, nonce, big.NewInt(120), big.NewInt(12))
	env.putChain(&transaction.ResentTransaction{Nonce: nonce, CurrentHash: h2, PrevHashes: []common.Hash{env.old}})
	env.feeCap, env.tip = big.NewInt(1), big.NewInt(1)

	cancelHash, err := env.svc.CancelTransaction(context.Background(), env.old)
	if err != nil {
		t.Fatal(err)
	}
	assertFees(t, env.lastSent(), 138, 13)
	chain := env.chain(nonce)
	if chain.CurrentHash != h2 || !slices.Equal(chain.PrevHashes, []common.Hash{env.old}) {
		t.Fatalf("chain = %+v", chain)
	}
	if chain.CurrentHash == cancelHash || slices.Contains(chain.PrevHashes, cancelHash) {
		t.Fatal("cancellation entered the chain")
	}
	if _, err := env.svc.ResendTransaction(context.Background(), cancelHash, 20); !errors.Is(err, transaction.ErrCancellation) {
		t.Fatalf("resend got %v", err)
	}
	if _, err := env.svc.CancelTransaction(context.Background(), cancelHash); !errors.Is(err, transaction.ErrCancellation) {
		t.Fatalf("cancel got %v", err)
	}
	if env.sentCount() != 1 {
		t.Fatalf("broadcast %d transactions", env.sentCount())
	}

	env.setReceipts(map[common.Hash]*types.Receipt{
		cancelHash: {TxHash: cancelHash, Status: types.ReceiptStatusSuccessful},
	})
	env.setCancelWatch(true)
	if _, err := env.svc.WaitForReceipt(context.Background(), env.old); !errors.Is(err, transaction.ErrTransactionCancelled) {
		t.Fatalf("wait got %v", err)
	}
	if _, err := env.svc.TransactionReceipt(context.Background(), env.old); !errors.Is(err, ethereum.NotFound) {
		t.Fatalf("receipt got %v", err)
	}

	var asked []common.Hash
	env.byHash = func(hash common.Hash) (*types.Transaction, bool, error) {
		asked = append(asked, hash)
		return nil, false, ethereum.NotFound
	}
	got, _, err := env.svc.KnownTransaction(context.Background(), env.old)
	if !errors.Is(err, ethereum.NotFound) {
		t.Fatalf("pending got %s err %v", got, err)
	}
	if got == cancelHash || slices.Contains(asked, cancelHash) {
		t.Fatalf("pending lookup returned the cancellation, asked %v", asked)
	}
}

func TestSendSucceedsWhenOwnerStoreFails(t *testing.T) {
	t.Parallel()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	base := storemock.NewStateStore()
	testutil.CleanupCloser(t, base)
	store := prefixPutFailStore{StateStorer: base}
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
			backendmock.WithSendTransactionFunc(func(context.Context, *types.Transaction) error {
				return nil
			}),
		),
		signer,
		store,
		big.NewInt(5),
		monitormock.New(monitormock.WithWatchTransactionFunc(func(common.Hash, uint64) (<-chan types.Receipt, <-chan error, error) {
			return make(chan types.Receipt), make(chan error), nil
		})),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, svc)

	hash, err := svc.Send(context.Background(), admissionRequest(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StoredTransaction(hash); err != nil {
		t.Fatal(err)
	}
	pending, err := svc.PendingTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0] != hash {
		t.Fatalf("pending = %v", pending)
	}
	var chain transaction.ResentTransaction
	if err := base.Get(transaction.ResendTxKey(1), &chain); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("chain error = %v", err)
	}
}

type prefixPutFailStore struct {
	storage.StateStorer
}

func (s prefixPutFailStore) Put(key string, obj any) error {
	if strings.HasPrefix(key, "transaction_resend_") {
		return errors.New("disk full")
	}
	return s.StateStorer.Put(key, obj)
}

type resendEnv struct {
	t            *testing.T
	svc          transaction.Service
	store        storage.StateStorer
	old          common.Hash
	created      int64
	feeCap       *big.Int
	tip          *big.Int
	historyFee   *big.Int
	historyTip   *big.Int
	sendErr      error
	sendCtxErr   error
	sendDeadline bool
	sent         []*types.Transaction
	watched      []common.Hash
	receipts     map[common.Hash]*types.Receipt
	receiptErr   error
	byHash       func(common.Hash) (*types.Transaction, bool, error)
	cancelWatch  bool
	mu           sync.Mutex
}

func newResendEnv(t *testing.T, nonce uint64, description string, data []byte, pending bool) *resendEnv {
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
		Nonce:     nonce,
		ChainID:   chainID,
		To:        &admissionRecipient,
		Value:     big.NewInt(0),
		Gas:       21_000,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Data:      data,
	}), chainID)
	if err != nil {
		t.Fatal(err)
	}

	env := &resendEnv{
		t:       t,
		store:   storemock.NewStateStore(),
		old:     signed.Hash(),
		created: 42,
		feeCap:  big.NewInt(200),
		tip:     big.NewInt(20),
	}
	testutil.CleanupCloser(t, env.store)
	env.putStored(signed.Hash(), nonce, feeCap, tip)
	stored, err := func() (*transaction.StoredTransaction, error) {
		var tx transaction.StoredTransaction
		err := env.store.Get(transaction.StoredTransactionKey(signed.Hash()), &tx)
		return &tx, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	stored.Data = data
	stored.Description = description
	stored.Created = env.created
	if err := env.store.Put(transaction.StoredTransactionKey(signed.Hash()), stored); err != nil {
		t.Fatal(err)
	}
	if pending {
		if err := env.store.Put(transaction.PendingTransactionKey(signed.Hash()), struct{}{}); err != nil {
			t.Fatal(err)
		}
	}

	svc, err := transaction.NewService(
		log.Noop,
		mustSender(t, key),
		backendmock.New(
			backendmock.WithPendingNonceAtFunc(func(context.Context, common.Address) (uint64, error) {
				return 1, nil
			}),
			backendmock.WithSuggestedFeeAndTipFunc(func(context.Context, *big.Int, int) (*big.Int, *big.Int, error) {
				env.mu.Lock()
				defer env.mu.Unlock()
				return env.feeCap, env.tip, nil
			}),
			backendmock.WithSuggestFeeHistoryFunc(func(context.Context, int) (*big.Int, *big.Int, error) {
				env.mu.Lock()
				defer env.mu.Unlock()
				if env.historyFee == nil || env.historyTip == nil {
					return nil, nil, backendmock.ErrNotImplemented
				}
				return env.historyFee, env.historyTip, nil
			}),
			backendmock.WithSendTransactionFunc(func(ctx context.Context, tx *types.Transaction) error {
				env.mu.Lock()
				env.sendCtxErr = ctx.Err()
				_, env.sendDeadline = ctx.Deadline()
				env.sent = append(env.sent, tx)
				err := env.sendErr
				env.mu.Unlock()
				return err
			}),
			backendmock.WithTransactionReceiptFunc(func(_ context.Context, hash common.Hash) (*types.Receipt, error) {
				env.mu.Lock()
				defer env.mu.Unlock()
				if env.receiptErr != nil {
					return nil, env.receiptErr
				}
				if receipt, ok := env.receipts[hash]; ok {
					return receipt, nil
				}
				return nil, ethereum.NotFound
			}),
			backendmock.WithTransactionByHashFunc(func(_ context.Context, hash common.Hash) (*types.Transaction, bool, error) {
				if env.byHash != nil {
					return env.byHash(hash)
				}
				// Keep a stored pending mark pending: a NotFound answer deletes it.
				return types.NewTx(&types.DynamicFeeTx{
					GasFeeCap: big.NewInt(1),
					GasTipCap: big.NewInt(1),
					Value:     big.NewInt(0),
				}), true, nil
			}),
		),
		signer,
		env.store,
		chainID,
		monitormock.New(monitormock.WithWatchTransactionFunc(func(hash common.Hash, _ uint64) (<-chan types.Receipt, <-chan error, error) {
			env.mu.Lock()
			env.watched = append(env.watched, hash)
			cancel := env.cancelWatch
			env.mu.Unlock()
			if cancel {
				errC := make(chan error, 1)
				errC <- transaction.ErrTransactionCancelled
				return nil, errC, nil
			}
			return make(chan types.Receipt), make(chan error), nil
		})),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, svc)
	env.svc = svc
	return env
}

func (e *resendEnv) putStored(hash common.Hash, nonce uint64, feeCap, tip *big.Int) {
	e.t.Helper()
	err := e.store.Put(transaction.StoredTransactionKey(hash), transaction.StoredTransaction{
		Nonce:       nonce,
		To:          &admissionRecipient,
		Value:       big.NewInt(0),
		GasLimit:    21_000,
		GasFeeCap:   feeCap,
		GasTipCap:   tip,
		Created:     e.created,
		Description: "postage",
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *resendEnv) putChain(chain *transaction.ResentTransaction) {
	e.t.Helper()
	if err := e.store.Put(transaction.ResendTxKey(chain.Nonce), chain); err != nil {
		e.t.Fatal(err)
	}
}

func (e *resendEnv) chain(nonce uint64) transaction.ResentTransaction {
	e.t.Helper()
	chain, err := e.chainErr(nonce)
	if err != nil {
		e.t.Fatal(err)
	}
	return chain
}

func (e *resendEnv) chainErr(nonce uint64) (transaction.ResentTransaction, error) {
	var chain transaction.ResentTransaction
	err := e.store.Get(transaction.ResendTxKey(nonce), &chain)
	return chain, err
}

func (e *resendEnv) pending() []common.Hash {
	e.t.Helper()
	pending, err := e.svc.PendingTransactions()
	if err != nil {
		e.t.Fatal(err)
	}
	return pending
}

func (e *resendEnv) setReceipts(receipts map[common.Hash]*types.Receipt) {
	e.mu.Lock()
	e.receipts = receipts
	e.mu.Unlock()
}

func (e *resendEnv) setReceiptErr(err error) {
	e.mu.Lock()
	e.receiptErr = err
	e.mu.Unlock()
}

func (e *resendEnv) setCancelWatch(cancel bool) {
	e.mu.Lock()
	e.cancelWatch = cancel
	e.mu.Unlock()
}

func (e *resendEnv) sentCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.sent)
}

func (e *resendEnv) lastSent() *types.Transaction {
	e.t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.sent) == 0 {
		e.t.Fatal("no transaction was broadcast")
	}
	return e.sent[len(e.sent)-1]
}

func (e *resendEnv) waitForWatch(before int, hash common.Hash) {
	e.t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		watched := e.watchedHashes()
		if len(watched) > before && slices.Contains(watched, hash) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("watched = %v", e.watchedHashes())
}

func (e *resendEnv) watchedHashes() []common.Hash {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.watched)
}

func assertFees(t *testing.T, tx *types.Transaction, fee, tip int64) {
	t.Helper()
	if tx.GasFeeCap().Cmp(big.NewInt(fee)) != 0 || tx.GasTipCap().Cmp(big.NewInt(tip)) != 0 {
		t.Fatalf("fee %s tip %s, want %d/%d", tx.GasFeeCap(), tx.GasTipCap(), fee, tip)
	}
	if tx.GasFeeCap().Cmp(tx.GasTipCap()) < 0 {
		t.Fatalf("fee cap %s below tip %s", tx.GasFeeCap(), tx.GasTipCap())
	}
}

func mustSender(t *testing.T, key *ecdsa.PrivateKey) common.Address {
	t.Helper()
	sender, err := crypto.NewDefaultSigner(key).EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	return sender
}
