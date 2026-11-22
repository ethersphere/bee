// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

const (
	resendTxPrefix = "transaction_resend_"
	// resendBroadcastTimeout bounds the broadcast, which runs under the service lock.
	resendBroadcastTimeout = 30 * time.Second
)

// ResentTransaction is the resend chain of one nonce. Every hash in it is the same
// transaction with different fees. It is persisted under resendTxKey.
type ResentTransaction struct {
	Nonce       uint64
	CurrentHash common.Hash   // last broadcast hash
	PrevHashes  []common.Hash // earlier hashes, oldest first
}

func resendTxKey(nonce uint64) string {
	return fmt.Sprintf("%s%020d", resendTxPrefix, nonce)
}

// allHashes returns every hash of the chain, oldest first.
func (r *ResentTransaction) allHashes() []common.Hash {
	out := make([]common.Hash, 0, len(r.PrevHashes)+1)
	out = append(out, r.PrevHashes...)
	return append(out, r.CurrentHash)
}

func (r *ResentTransaction) contains(txHash common.Hash) bool {
	return slices.Contains(r.allHashes(), txHash)
}

// promote makes txHash the current hash of the chain. A hash that is already in
// the chain is moved, not duplicated.
func (r *ResentTransaction) promote(txHash common.Hash) {
	if r.CurrentHash == txHash {
		return
	}
	prev := make([]common.Hash, 0, len(r.PrevHashes)+1)
	for _, h := range r.PrevHashes {
		if h != txHash {
			prev = append(prev, h)
		}
	}
	r.PrevHashes = append(prev, r.CurrentHash)
	r.CurrentHash = txHash
}

// getResendChain loads the owner record of the nonce.
// It returns storage.ErrNotFound when the nonce has no record.
func (t *transactionService) getResendChain(nonce uint64) (*ResentTransaction, error) {
	var chain ResentTransaction
	err := t.store.Get(resendTxKey(nonce), &chain)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("load resend chain: %w", err)
	}
	chain.Nonce = nonce
	return &chain, nil
}

// knownHashes returns txHash and, when txHash belongs to a resend chain, every other
// hash of that chain, newest first.
func (t *transactionService) knownHashes(txHash common.Hash) ([]common.Hash, error) {
	stored, err := t.StoredTransaction(txHash)
	if errors.Is(err, ErrUnknownTransaction) {
		return []common.Hash{txHash}, nil
	}
	if err != nil {
		return nil, err
	}
	chain, err := t.getResendChain(stored.Nonce)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return []common.Hash{txHash}, nil
		}
		return nil, err
	}
	if !chain.contains(txHash) {
		return []common.Hash{txHash}, nil
	}
	hashes := chain.allHashes()
	slices.Reverse(hashes)
	return hashes, nil
}

// resendFees prices the resend. A zero bumpPercent uses the market price the way
// Send does, pulled up to the current hash when the market fell below it.
// Otherwise the fees of the current hash are raised by bumpPercent.
func (t *transactionService) calculateResendFees(ctx context.Context, current *StoredTransaction, bumpPercent int) (gasFeeCap, gasTipCap *big.Int, err error) {
	if bumpPercent <= 0 {
		if current.FeePriority != 0 {
			gasFeeCap, gasTipCap, err = t.backend.SuggestFeeHistory(ctx, current.FeePriority)
		} else {
			gasFeeCap, gasTipCap, err = t.backend.SuggestedFeeAndTip(ctx, sctx.GetGasPrice(ctx), current.GasTipBoost)
		}
		if err != nil {
			return nil, nil, err
		}
		return maxFee(gasFeeCap, current.GasFeeCap), maxFee(gasTipCap, current.GasTipCap), nil
	}
	bump := func(v *big.Int) *big.Int {
		return new(big.Int).Div(new(big.Int).Mul(new(big.Int).Set(v), big.NewInt(int64(100+bumpPercent))), big.NewInt(100))
	}
	return bump(current.GasFeeCap), bump(current.GasTipCap), nil
}

// maxFee returns the larger of the two fees. A nil fee is missing and does not compete.
func maxFee(a, b *big.Int) *big.Int {
	if a == nil && b == nil {
		return nil
	}
	if a == nil {
		return new(big.Int).Set(b)
	}
	if b == nil || a.Cmp(b) >= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

// raiseByPercent raises v by percent. When the percentage rounds away, v grows by 1 wei,
// which is the smallest replacement a node accepts.
func raiseByPercent(v *big.Int, percent int) *big.Int {
	if v == nil {
		return nil
	}
	raised := new(big.Int).Div(new(big.Int).Mul(new(big.Int).Set(v), big.NewInt(int64(100+percent))), big.NewInt(100))
	if raised.Cmp(v) <= 0 {
		return new(big.Int).Add(v, big.NewInt(1))
	}
	return raised
}

// ensurePending marks txHash pending and watches it, unless it is pending already.
// filterPendingTransactions drops the mark of a hash the node did not know, so a hash
// that is broadcast again may have lost it.
func (t *transactionService) ensurePending(txHash common.Hash) error {
	err := t.store.Get(pendingTransactionKey(txHash), &struct{}{})
	if err == nil {
		return nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("load pending transaction: %w", err)
	}
	if err := t.store.Put(pendingTransactionKey(txHash), struct{}{}); err != nil {
		return fmt.Errorf("store pending transaction: %w", err)
	}
	t.waitForPendingTx(txHash)
	return nil
}

// ResendTransaction broadcasts a previously sent transaction again with the same nonce
// and content and new fees. txHash may be any hash of the resend chain of its nonce.
// A zero bumpPercent prices the transaction at the market, but not below the fees of
// the current hash: a fallen market is pulled up to those fees. Otherwise the fees of
// the current hash of the chain are raised by bumpPercent.
// A node answer about the transaction is returned as an error. When the transaction
// may be in the mempool, it is stored, watched and added to the chain.
func (t *transactionService) ResendTransaction(ctx context.Context, txHash common.Hash, bumpPercent int) (*ResentTransaction, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	stored, err := t.StoredTransaction(txHash)
	if err != nil {
		return nil, err
	}

	chain, err := t.getResendChain(stored.Nonce)
	current := stored
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	} else if !chain.contains(txHash) {
		return nil, ErrNotInResendChain
	} else if chain.CurrentHash != txHash {
		current, err = t.StoredTransaction(chain.CurrentHash)
		if err != nil {
			return nil, fmt.Errorf("current transaction %s: %w", chain.CurrentHash, err)
		}
	}

	gasFeeCap, gasTipCap, err := t.calculateResendFees(ctx, current, bumpPercent)
	if err != nil {
		return nil, err
	}

	signedTx, err := t.signer.SignTx(types.NewTx(&types.DynamicFeeTx{
		Nonce:     current.Nonce,
		ChainID:   t.chainID,
		To:        current.To,
		Value:     current.Value,
		Gas:       current.GasLimit,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Data:      current.Data,
	}), t.chainID)
	if err != nil {
		return nil, err
	}

	sendCtx, cancel := context.WithTimeout(t.ctx, resendBroadcastTimeout)
	err = t.backend.SendTransaction(sendCtx, signedTx)
	cancel()
	switch {
	case isAlreadyKnown(err):
		return nil, ErrAlreadyImported
	case isReplacementUnderpriced(err):
		return nil, fmt.Errorf("%w: %w", ErrReplacementUnderpriced, err)
	case isNonceTooLow(err):
		return nil, fmt.Errorf("%w: %w", ErrNonceUsed, err)
	case !txMayBeInMempool(err):
		return nil, err
	case err != nil:
		t.logger.Warning("transaction broadcast returned an error; the transaction may be in the mempool",
			"tx", signedTx.Hash(),
			"nonce", signedTx.Nonce(),
			"error", err,
		)
	}

	newHash := signedTx.Hash()
	if chain == nil {
		if newHash != txHash {
			if err := t.storeResend(signedTx, current); err != nil {
				return nil, err
			}
			chain = &ResentTransaction{
				Nonce:       current.Nonce,
				CurrentHash: newHash,
				PrevHashes:  []common.Hash{txHash},
			}
			if err := t.store.Put(resendTxKey(chain.Nonce), chain); err != nil {
				return nil, fmt.Errorf("store resend chain: %w", err)
			}
		} else {
			chain = &ResentTransaction{Nonce: current.Nonce, CurrentHash: txHash}
		}
	} else {
		if !chain.contains(newHash) {
			if err := t.storeResend(signedTx, current); err != nil {
				return nil, err
			}
		}
		if newHash != chain.CurrentHash {
			chain.promote(newHash)
			if err := t.store.Put(resendTxKey(chain.Nonce), chain); err != nil {
				return nil, fmt.Errorf("store resend chain: %w", err)
			}
		}
	}

	if err := t.ensurePending(newHash); err != nil {
		return nil, err
	}
	return chain, nil
}

func (t *transactionService) storeResend(signedTx *types.Transaction, current *StoredTransaction) error {
	err := t.store.Put(storedTransactionKey(signedTx.Hash()), StoredTransaction{
		To:          signedTx.To(),
		Data:        signedTx.Data(),
		GasPrice:    signedTx.GasPrice(),
		GasLimit:    signedTx.Gas(),
		GasTipBoost: current.GasTipBoost,
		GasTipCap:   signedTx.GasTipCap(),
		GasFeeCap:   signedTx.GasFeeCap(),
		FeePriority: current.FeePriority,
		Value:       signedTx.Value(),
		Nonce:       signedTx.Nonce(),
		Created:     time.Now().Unix(),
		Description: current.Description,
	})
	if err != nil {
		return fmt.Errorf("store resent transaction: %w", err)
	}
	return nil
}

// PendingTransactionByHash returns the newest hash of txHash's resend chain that the
// node has answered for. The boolean is true when that hash is still in the mempool.
// When the node knows none of the hashes, the newest hash of the chain is returned
// with ethereum.NotFound.
func (t *transactionService) PendingTransactionByHash(ctx context.Context, txHash common.Hash) (common.Hash, bool, error) {
	hashes, err := t.knownHashes(txHash)
	if err != nil {
		return common.Hash{}, false, err
	}
	for _, h := range hashes {
		_, isPending, err := t.backend.TransactionByHash(ctx, h)
		switch {
		case err == nil && isPending:
			return h, true, nil
		case err == nil:
			return h, false, nil
		case errors.Is(err, ethereum.NotFound):
		default:
			return common.Hash{}, false, err
		}
	}
	return hashes[0], false, ethereum.NotFound
}

// TransactionReceipt returns the receipt of the transaction. When txHash belongs to a
// resend chain, the receipt of whichever hash of the chain was mined is returned.
// It returns ethereum.NotFound when none of them is mined.
func (t *transactionService) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	hashes, err := t.knownHashes(txHash)
	if err != nil {
		return nil, err
	}
	for _, h := range hashes {
		receipt, err := t.backend.TransactionReceipt(ctx, h)
		switch {
		case err == nil && receipt != nil:
			return receipt, nil
		case err == nil, errors.Is(err, ethereum.NotFound):
			continue
		default:
			return nil, err
		}
	}
	return nil, ethereum.NotFound
}
