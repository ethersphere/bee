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
	"strings"
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

// ResendResult is what a caller sees after a resend.
// Superseded chains stay in the stored record.
type ResendResult struct {
	Nonce       uint64
	CurrentHash common.Hash
	PrevHashes  []common.Hash // earlier hashes, oldest first
}

func (r *ResentTransaction) result() *ResendResult {
	return &ResendResult{
		Nonce:       r.Nonce,
		CurrentHash: r.CurrentHash,
		PrevHashes:  slices.Clone(r.PrevHashes),
	}
}

// ResentTransaction is the resend chain of one nonce. Every hash in it is the same
// transaction with different fees. It is persisted under resendTxKey.
type ResentTransaction struct {
	Nonce       uint64
	CurrentHash common.Hash   // last broadcast hash
	PrevHashes  []common.Hash // earlier hashes, oldest first
	// Superseded holds the chains of earlier owners of the nonce, each oldest first.
	// They can no longer be resent, but one of them may still be mined.
	Superseded [][]common.Hash
}

func resendTxKey(nonce uint64) string {
	return fmt.Sprintf("%s%020d", resendTxPrefix, nonce)
}

// chainHashes returns the hashes of the current chain, oldest first.
// Superseded chains are not included.
func (r *ResentTransaction) chainHashes() []common.Hash {
	out := make([]common.Hash, 0, len(r.PrevHashes)+1)
	out = append(out, r.PrevHashes...)
	return append(out, r.CurrentHash)
}

// inChain reports whether txHash belongs to the current chain.
func (r *ResentTransaction) inChain(txHash common.Hash) bool {
	return slices.Contains(r.chainHashes(), txHash)
}

// promote makes txHash the current hash and keeps the previous current hash.
func (r *ResentTransaction) promote(txHash common.Hash) {
	r.PrevHashes = append(r.PrevHashes, r.CurrentHash)
	r.CurrentHash = txHash
}

// supersededChain returns the superseded chain that contains txHash.
func (r *ResentTransaction) supersededChain(txHash common.Hash) []common.Hash {
	for _, hashes := range r.Superseded {
		if slices.Contains(hashes, txHash) {
			return hashes
		}
	}
	return nil
}

// outsideChainError reports why txHash cannot be resent or cancelled.
// A superseded hash lost the nonce to a newer transaction. A cancellation hash
// is the zero-value transaction that spends the nonce. Anything else stored for
// the nonce is not the transaction the nonce is being resent as.
func outsideChainError(chain *ResentTransaction, stored *StoredTransaction, txHash common.Hash) error {
	if chain.supersededChain(txHash) != nil {
		return ErrSuperseded
	}
	if strings.HasSuffix(stored.Description, cancellationDescriptionSuffix) {
		return ErrCancellation
	}
	return ErrNotInResendChain
}

// getResendChain loads the owner record of the nonce.
// It returns storage.ErrNotFound when the nonce has no record.
func (t *transactionService) getResendChain(nonce uint64) (*ResentTransaction, error) {
	var chain ResentTransaction
	err := t.store.Get(resendTxKey(nonce), &chain)
	if err != nil {
		return nil, fmt.Errorf("load resend chain: %w", err)
	}
	chain.Nonce = nonce
	return &chain, nil
}

// knownHashes returns txHash and, when txHash belongs to a resend chain, every other
// hash of that chain, newest first. The chain may be a superseded one.
// txHash must be a transaction stored by this service.
func (t *transactionService) knownHashes(txHash common.Hash) ([]common.Hash, error) {
	stored, err := t.StoredTransaction(txHash)
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
	hashes := []common.Hash{txHash}
	if chain.inChain(txHash) {
		hashes = chain.chainHashes()
	} else if superseded := chain.supersededChain(txHash); superseded != nil {
		hashes = slices.Clone(superseded)
	}
	slices.Reverse(hashes)
	return hashes, nil
}

// resendFees prices a replacement the node will accept.
// With no bump the fees are max(market, current raised by MinReplacementBumpPercent).
// With a bump they are max(market, current × (1 + bump/100)).
// A stored transaction without fees cannot be raised: a bump is an error,
// and no bump uses the market.
func (t *transactionService) calculateResendFees(ctx context.Context, current *StoredTransaction, bumpPercent int) (gasFeeCap, gasTipCap *big.Int, err error) {
	if bumpPercent > 0 && (current.GasFeeCap == nil || current.GasTipCap == nil) {
		return nil, nil, ErrMissingFees
	}
	var marketFee, marketTip *big.Int
	if current.FeePriority != 0 {
		marketFee, marketTip, err = t.backend.SuggestFeeHistory(ctx, current.FeePriority)
	} else {
		marketFee, marketTip, err = t.backend.SuggestedFeeAndTip(ctx, sctx.GetGasPrice(ctx), current.GasTipBoost)
	}
	if err != nil {
		return nil, nil, err
	}
	if current.GasFeeCap == nil || current.GasTipCap == nil {
		return maxFee(marketFee, current.GasFeeCap), maxFee(marketTip, current.GasTipCap), nil
	}
	percent := bumpPercent
	if percent <= 0 {
		percent = MinReplacementBumpPercent
	}
	return maxFee(marketFee, raiseByPercent(current.GasFeeCap, percent)),
		maxFee(marketTip, raiseByPercent(current.GasTipCap, percent)), nil
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

// ResendTransaction broadcasts a previously sent transaction again with the same nonce
// and content and new fees. txHash may be any hash of the resend chain of its nonce.
// A zero bumpPercent uses max(market, current fees raised by MinReplacementBumpPercent). A positive bumpPercent uses
// max(market, current fees × (1 + bumpPercent/100)).
func (t *transactionService) ResendTransaction(ctx context.Context, txHash common.Hash, bumpPercent int) (*ResendResult, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	stored, err := t.StoredTransaction(txHash)
	if err != nil {
		return nil, err
	}

	chain, err := t.getResendChain(stored.Nonce)
	current := stored
	switch {
	case errors.Is(err, storage.ErrNotFound):
		// The nonce has no owner record.
	case err != nil:
		return nil, err
	case !chain.inChain(txHash):
		return nil, outsideChainError(chain, stored, txHash)
	case chain.CurrentHash != txHash:
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
	if err := rejectedBroadcast(err); err != nil {
		return nil, err
	}
	if err != nil {
		t.logger.Warning("transaction broadcast returned an error; the transaction may be in the mempool",
			"tx", signedTx.Hash(),
			"nonce", signedTx.Nonce(),
			"error", err,
		)
	}

	newHash := signedTx.Hash()
	if err := t.store.Put(storedTransactionKey(newHash), StoredTransaction{
		To:          signedTx.To(),
		Data:        signedTx.Data(),
		GasPrice:    signedTx.GasPrice(),
		GasLimit:    signedTx.Gas(),
		GasTipBoost: current.GasTipBoost,
		GasTipCap:   signedTx.GasTipCap(),
		GasFeeCap:   signedTx.GasFeeCap(),
		Value:       signedTx.Value(),
		Nonce:       signedTx.Nonce(),
		Created:     time.Now().Unix(),
		Description: current.Description,
		FeePriority: current.FeePriority,
	}); err != nil {
		return nil, err
	}
	if chain == nil {
		chain = &ResentTransaction{
			Nonce:       current.Nonce,
			CurrentHash: newHash,
			PrevHashes:  []common.Hash{txHash},
		}
	} else {
		chain.promote(newHash)
	}
	if err := t.store.Put(resendTxKey(chain.Nonce), chain); err != nil {
		return nil, fmt.Errorf("store resend chain: %w", err)
	}

	if err := t.store.Put(pendingTransactionKey(newHash), struct{}{}); err != nil {
		return nil, fmt.Errorf("store pending transaction: %w", err)
	}
	t.waitForPendingTx(newHash)
	return chain.result(), nil
}

// KnownTransaction returns the newest hash of txHash's chain that the node has
// answered for. The boolean is true when that any of hashes of txHash's chain is still in the mempool.
// When the node knows none of them, the newest hash is returned with ethereum.NotFound.
func (t *transactionService) KnownTransaction(ctx context.Context, txHash common.Hash) (common.Hash, bool, error) {
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
// A hash this service did not store is returned as ErrUnknownTransaction.
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
