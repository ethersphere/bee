// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/sctx"
)

const retryStatePrefix = "transaction_retry_"

// mempoolBumpPercent is the minimum percent increase required by EIP-1559
// mempools to accept a replacement transaction for the same nonce.
const mempoolBumpPercent = 15

// defaultAttemptsPerTier is the number of broadcast attempts at each fee tier
// before escalating to the next tier.
const defaultAttemptsPerTier = 2

func retryStateKey(nonce uint64) string {
	return fmt.Sprintf("%s%020d", retryStatePrefix, nonce)
}

// RetriedTransaction holds state for a single sendWithRetry session and is persisted under retryStateKey.
type RetriedTransaction struct {
	Nonce         uint64
	NonceAssigned bool
	CurrentHash   common.Hash
	PrevHashes    []common.Hash
}

func (rs *RetriedTransaction) allHashes() []common.Hash {
	n := len(rs.PrevHashes)
	if rs.CurrentHash != (common.Hash{}) {
		n++
	}
	out := make([]common.Hash, 0, n)
	out = append(out, rs.PrevHashes...)
	if rs.CurrentHash != (common.Hash{}) {
		out = append(out, rs.CurrentHash)
	}
	return out
}

// SendWithRetry sends an EIP-1559 transaction using fee-history tiers with automatic
// escalation. Each tier gets attemptsPerTier broadcast rounds with fresh eth_feeHistory
// data. A +15% mempool bump floor is applied to the tip and the fee cap so a replacement is accepted.
func (t *transactionService) SendWithRetry(ctx context.Context, request *TxRequest) (txHash common.Hash, receipt *types.Receipt, err error) {
	if request.GasPrice != nil {
		err = errors.New("send txs with retry requires automatic gas pricing")
		t.retryMetrics.RecordRetryComplete(1, err)
		return common.Hash{}, nil, err
	}
	return t.sendWithRetry(ctx, request, nil, nil, nil, nil)
}

// applyMempoolBump returns amount increased by mempoolBumpPercent.
func applyMempoolBump(amount *big.Int) *big.Int {
	return new(big.Int).Div(
		new(big.Int).Mul(new(big.Int).Set(amount), big.NewInt(int64(100+mempoolBumpPercent))),
		big.NewInt(100),
	)
}

// suggestGasFeeForTier fetches fresh fee history, picks the tip for the given tier,
// and applies the mempool bump floor to the tip and the fee cap.
func (t *transactionService) suggestGasFeeForTier(ctx context.Context, tier feeTier, previousTip, previousFeeCap *big.Int) (gasFeeCap, gasTipCap *big.Int, err error) {
	header, err := t.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("block header: %w", err)
	}
	if header == nil || header.BaseFee == nil {
		return nil, nil, errors.New("latest block header or base fee unavailable")
	}

	fh, err := t.backend.SuggestedFeeAndTipsFromHistory(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("fee history: %w", err)
	}
	if fh == nil {
		return nil, nil, errors.New("fee history: empty response")
	}

	tip := tierTip(tier, fh)

	if previousTip != nil && previousTip.Sign() > 0 {
		bumpedTip := applyMempoolBump(previousTip)
		if tip.Cmp(bumpedTip) < 0 {
			tip = bumpedTip
		}
	}

	gasFeeCap = new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	gasFeeCapWithTip := new(big.Int).Add(new(big.Int).Set(gasFeeCap), tip)

	if previousFeeCap != nil && previousFeeCap.Sign() > 0 {
		bumpedFeeCap := applyMempoolBump(previousFeeCap)
		if gasFeeCapWithTip.Cmp(bumpedFeeCap) < 0 {
			gasFeeCapWithTip = bumpedFeeCap
		}
	}

	if t.maxTxPrice != nil && gasFeeCapWithTip.Cmp(t.maxTxPrice) > 0 {
		return nil, nil, fmt.Errorf("%w: max_fee_per_gas %s exceeds limit %s", ErrTxMaxPriceExceeded, gasFeeCapWithTip, t.maxTxPrice)
	}
	return gasFeeCapWithTip, tip, nil
}

func (t *transactionService) tierRangeForRequest(ctx context.Context) ([]feeTier, error) {
	start := t.startTier
	if override := sctx.GetFeePriority(ctx); override != "" {
		parsed, err := ParseFeeTier(override)
		if err != nil {
			return nil, fmt.Errorf("fee priority: %w", err)
		}
		if parsed > t.endTier {
			t.logger.Warning("fee priority exceeds configured maximum, clamping",
				"requested", parsed.String(),
				"maximum", t.endTier.String())
			parsed = t.endTier
		}
		start = parsed
	}
	return tierRange(start, t.endTier), nil
}

// broadcastTx prepares, signs, and sends a transaction.
// When fixedNonce is nil a new nonce is allocated (first attempt);
// otherwise the supplied nonce is reused (replacement transaction).
func (t *transactionService) broadcastTx(ctx context.Context, request *TxRequest, fixedNonce *uint64, tier feeTier, previousTip, previousFeeCap *big.Int) (*types.Transaction, error) {
	var nonce uint64

	if fixedNonce != nil {
		nonce = *fixedNonce
	} else {
		// The lock must be held until the transaction is broadcasted.
		// Otherwise another transaction can take the same nonce.
		t.lock.Lock()
		defer t.lock.Unlock()

		n, err := t.nextNonce(ctx)
		if err != nil {
			return nil, fmt.Errorf("next nonce: %w", err)
		}
		nonce = n
	}

	gasLimit, err := t.estimateGasLimit(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("estimate gas: %w", err)
	}

	gasFeeCap, gasTipCap, err := t.suggestGasFeeForTier(ctx, tier, previousTip, previousFeeCap)
	if err != nil {
		return nil, fmt.Errorf("suggest gas fee: %w", err)
	}

	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		ChainID:   t.chainID,
		To:        request.To,
		Value:     request.Value,
		Gas:       gasLimit,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Data:      request.Data,
	})

	signedTx, err := t.signer.SignTx(tx, t.chainID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSignTransaction, err)
	}

	t.logger.Debug("retried transaction: broadcast",
		"tx", signedTx.Hash(),
		"nonce", nonce,
		"tier", tier.String(),
		"gas_tip_cap", tx.GasTipCap(),
		"gas_fee_cap", tx.GasFeeCap(),
	)
	if err := t.backend.SendTransaction(ctx, signedTx); err != nil {
		return signedTx, fmt.Errorf("send transaction: %w", err)
	}
	return signedTx, nil
}

// persistReplaceTx persists a newly broadcast tx and updates retry state.
// Previous hashes stay in the store until the session finishes with a result.
func (t *transactionService) persistReplaceTx(signedTx *types.Transaction, rs *RetriedTransaction, description string) error {
	if signedTx == nil {
		return nil
	}

	txHash := signedTx.Hash()
	now := time.Now().Unix()

	if !rs.NonceAssigned {
		rs.Nonce = signedTx.Nonce()
		rs.NonceAssigned = true
	}
	if rs.CurrentHash != (common.Hash{}) {
		rs.PrevHashes = append(rs.PrevHashes, rs.CurrentHash)
	}
	rs.CurrentHash = txHash

	if err := t.store.Put(storedTransactionKey(txHash), StoredTransaction{
		To:          signedTx.To(),
		Data:        signedTx.Data(),
		GasPrice:    signedTx.GasPrice(),
		GasLimit:    signedTx.Gas(),
		GasTipCap:   signedTx.GasTipCap(),
		GasFeeCap:   signedTx.GasFeeCap(),
		Value:       signedTx.Value(),
		Nonce:       signedTx.Nonce(),
		Created:     now,
		Description: description,
	}); err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	if err := t.store.Put(pendingTransactionKey(txHash), struct{}{}); err != nil {
		return fmt.Errorf("store pending transaction: %w", err)
	}

	if err := t.store.Put(retryStateKey(rs.Nonce), rs); err != nil {
		return fmt.Errorf("store retry state: %w", err)
	}
	return nil
}

type retryNonceWatch struct {
	doneC <-chan struct{}
	errC  <-chan error
}

// sendWithRetry is the core retry loop.
func (t *transactionService) sendWithRetry(ctx context.Context, request *TxRequest, rs *RetriedTransaction, nonceWatch *retryNonceWatch, previousTip, previousFeeCap *big.Int) (common.Hash, *types.Receipt, error) {
	tiers, err := t.tierRangeForRequest(ctx)
	if err != nil {
		return common.Hash{}, nil, fmt.Errorf("fee tiers: %w", err)
	}

	if rs == nil {
		rs = &RetriedTransaction{}
	}

	if nonceWatch == nil {
		nonceWatch = &retryNonceWatch{}
	}

	var (
		terminateTxErr error
		nonce          *uint64
		attempts       int
	)
	defer func() { t.finishRetry(rs, attempts, terminateTxErr, nonceWatch) }()

	for _, tier := range tiers {
		for k := 0; k < t.attemptsPerTier; k++ {
			attempts++
			if rs.NonceAssigned {
				nonce = &rs.Nonce
			}

			signedTx, receipt, err := t.attempt(ctx, rs, request, previousTip, previousFeeCap, tier, nonce, nonceWatch)
			if err != nil && (isNonRetryable(err) || isNonceTooLow(err)) {
				terminateTxErr = err
				return common.Hash{}, nil, terminateTxErr
			}

			if receipt != nil {
				t.cleanupRetryResult(rs, receipt.TxHash)

				if receipt.Status == 0 {
					terminateTxErr = ErrTransactionReverted
					return receipt.TxHash, receipt, terminateTxErr
				}
				t.logger.Info("retried transaction: receipt received", "tx", receipt.TxHash, "nonce", rs.Nonce)
				return receipt.TxHash, receipt, nil
			}

			if signedTx != nil {
				previousTip = new(big.Int).Set(signedTx.GasTipCap())
				previousFeeCap = new(big.Int).Set(signedTx.GasFeeCap())
			}
		}
	}

	terminateTxErr = ErrAllAttemptsExhausted
	return rs.CurrentHash, nil, terminateTxErr
}

// finishRetry performs final cleanup when the loop exits without a receipt.
// The nonce watch keeps running until the confirmed nonce moves past the session.
func (t *transactionService) finishRetry(rs *RetriedTransaction, attempt int, terminateTxErr error, nonceWatch *retryNonceWatch) {
	if terminateTxErr != nil {
		t.logger.Error(
			terminateTxErr,
			"retried transaction: finished with error",
			"tx", rs.CurrentHash, "nonce", rs.Nonce, "attempt", attempt)
	}

	switch {
	case errors.Is(terminateTxErr, ErrTransactionCancelled):
		t.deleteRetryTransaction(rs)
	case nonceWatch.doneC != nil:
		t.waitForPendingRetry(rs, nonceWatch)
	}
	t.retryMetrics.RecordRetryComplete(attempt, terminateTxErr)
}

func (t *transactionService) watchRetryNonce(nonce uint64, watch *retryNonceWatch) {
	if watch.doneC != nil {
		return
	}
	doneC, errC := t.monitor.WatchNonce(nonce)
	watch.doneC = doneC
	watch.errC = errC
}

func (t *transactionService) receiptForRetryHashes(ctx context.Context, rs *RetriedTransaction) *types.Receipt {
	hashes := rs.allHashes()
	for i := len(hashes) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return nil
		}
		receipt, err := t.backend.TransactionReceipt(ctx, hashes[i])
		if err == nil && receipt != nil {
			return receipt
		}
	}
	return nil
}

// attempt performs a single broadcast+wait cycle: broadcast, persist state, wait for receipt.
func (t *transactionService) attempt(ctx context.Context, rs *RetriedTransaction, request *TxRequest, previousTip, previousFeeCap *big.Int, tier feeTier, nonce *uint64, nonceWatch *retryNonceWatch) (*types.Transaction, *types.Receipt, error) {
	replaced := true

	signedTx, broadCastErr := t.broadcastTx(ctx, request, nonce, tier, previousTip, previousFeeCap)
	if broadCastErr != nil {
		switch {
		case isNonceTooLow(broadCastErr):
			if receipt := t.receiptForRetryHashes(ctx, rs); receipt != nil {
				return nil, receipt, nil
			}
			return nil, nil, broadCastErr
		case isReplacementUnderpriced(broadCastErr):
			// Base fee dropped between attempts within the same tier,
			// so the bumped tip was not enough for the mempool to accept the replacement.
			t.logger.Debug("retried transaction: replacement underpriced",
				"tx", signedTx.Hash(),
				"nonce", signedTx.Nonce(),
				"gas_tip_cap", signedTx.GasTipCap(),
				"gas_fee_cap", signedTx.GasFeeCap(),
			)
			replaced = false
		case isNonRetryable(broadCastErr):
			return nil, nil, broadCastErr
		case signedTx == nil:
			// Any other error that occurred before the SendTransaction RPC call.
			replaced = false
		}
	}

	if replaced {
		persistErr := t.persistReplaceTx(signedTx, rs, request.Description)
		if rs.NonceAssigned {
			t.watchRetryNonce(rs.Nonce, nonceWatch)
		}
		if persistErr != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrUpdateRetryState, persistErr)
		}
	}

	if nonceWatch.doneC == nil {
		// Rare case: no successful broadcast yet, nothing to monitor. Wait to avoid spamming attempts.
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("wait to broadcast: %w", ctx.Err())
		case <-time.After(t.txRetryDelay):
			return nil, nil, broadCastErr
		}
	}

	return t.waitNonceChanged(ctx, rs, nonceWatch, signedTx, t.txRetryDelay)
}

func (t *transactionService) waitNonceChanged(ctx context.Context, rs *RetriedTransaction, nonceWatch *retryNonceWatch, signedTx *types.Transaction, timeout time.Duration) (*types.Transaction, *types.Receipt, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-nonceWatch.doneC:
		if receipt := t.receiptForRetryHashes(waitCtx, rs); receipt != nil {
			return nil, receipt, nil
		}
		return signedTx, nil, ErrTransactionCancelled
	case err := <-nonceWatch.errC:
		return signedTx, nil, fmt.Errorf("nonce watch: %w", err)
	case <-waitCtx.Done():
		return signedTx, nil, fmt.Errorf("wait for nonce: %w", waitCtx.Err())
	}
}

func (t *transactionService) waitForPendingRetry(rs *RetriedTransaction, nonceWatch *retryNonceWatch) {
	t.wg.Go(func() {
		select {
		case <-nonceWatch.doneC:
			if receipt := t.receiptForRetryHashes(t.ctx, rs); receipt != nil {
				t.logger.Info("pending retried transaction confirmed", "tx", receipt.TxHash, "nonce", rs.Nonce)
				t.cleanupRetryResult(rs, receipt.TxHash)
			} else {
				t.logger.Warning("pending retried transaction cancelled", "tx", rs.CurrentHash, "nonce", rs.Nonce)
				t.deleteRetryTransaction(rs)
			}

		case err := <-nonceWatch.errC:
			if errors.Is(err, ErrTransactionCancelled) {
				t.logger.Warning("pending retried transaction cancelled", "tx", rs.CurrentHash, "nonce", rs.Nonce)
				t.deleteRetryTransaction(rs)
			} else if !errors.Is(err, ErrMonitorClosed) {
				t.logger.Error(err, "waiting for pending retried transaction failed", "tx", rs.CurrentHash, "nonce", rs.Nonce)
			}
		case <-t.ctx.Done():
		}
	})
}

func (t *transactionService) cleanupRetryResult(rs *RetriedTransaction, resultHash common.Hash) {
	_ = t.store.Delete(retryStateKey(rs.Nonce))
	for _, h := range rs.allHashes() {
		_ = t.store.Delete(pendingTransactionKey(h))
		if h != resultHash {
			_ = t.store.Delete(storedTransactionKey(h))
		}
	}
}

func isReplacementUnderpriced(err error) bool {
	return err != nil && containsNormalized(err.Error(), "replacementtransactionunderpriced")
}

func isNonceTooLow(err error) bool {
	return err != nil && containsNormalized(err.Error(), "noncetoolow")
}

// normalizeForMatch lowercases s and strips spaces so that both
// "insufficient funds" and "InsufficientFunds" match the same needle.
func normalizeForMatch(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
}

func containsNormalized(haystack, needle string) bool {
	return strings.Contains(normalizeForMatch(haystack), needle)
}

func isNonRetryable(err error) bool {
	if errors.Is(err, ErrTransactionReverted) ||
		errors.Is(err, ErrTransactionCancelled) ||
		errors.Is(err, ErrSignTransaction) ||
		errors.Is(err, ErrTxMaxPriceExceeded) ||
		errors.Is(err, ErrUpdateRetryState) ||
		errors.Is(err, context.Canceled) {
		return true
	}

	s := normalizeForMatch(err.Error())
	nonRetryable := []string{
		"specifiedgasprice",
		"alreadycommitted",
		"alreadyrevealed",
		"alreadyclaimed",
		"notcommitphase",
		"notrevealphase",
		"notclaimphase",
		"commitroundover",
		"commitroundnotstarted",
		"phaselastblock",
		"outofdepth",
		"outofdepthreveal",
		"outofdepthclaim",
		"notstaked",
		"muststake2rounds",
		"noreveals",
		"nocommitsreceived",
		"executionreverted",
		"insufficientfunds",
	}
	for _, sub := range nonRetryable {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// pendingRetryTransactions returns hashes managed by active retry sessions
// so that waitForAllPendingTx does not double-watch them.
func (t *transactionService) pendingRetryTransactions() (map[uint64]RetriedTransaction, error) {
	out := make(map[uint64]RetriedTransaction)
	err := t.store.Iterate(retryStatePrefix, func(key, val []byte) (stop bool, err error) {
		var state RetriedTransaction
		if err := json.Unmarshal(val, &state); err != nil {
			return false, fmt.Errorf("unmarshal retry state: %w", err)
		}
		var nonce uint64
		if _, err := fmt.Sscanf(string(key), retryStatePrefix+"%d", &nonce); err != nil {
			return false, fmt.Errorf("parse retry nonce: %w", err)
		}
		state.Nonce = nonce
		out[nonce] = state
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate retry state: %w", err)
	}
	return out, nil
}

func (t *transactionService) deleteRetryTransaction(rs *RetriedTransaction) {
	_ = t.store.Delete(retryStateKey(rs.Nonce))
	for _, h := range rs.allHashes() {
		_ = t.store.Delete(pendingTransactionKey(h))
		_ = t.store.Delete(storedTransactionKey(h))
	}
}

func (t *transactionService) resumeRetryTransactions() error {
	entries, err := t.pendingRetryTransactions()
	if err != nil {
		return fmt.Errorf("pending retry transactions: %w", err)
	}

	confirmed, err := t.backend.NonceAt(t.ctx, t.sender, nil)
	if err != nil {
		t.logger.Warning("resume retried transaction: failed to get confirmed nonce, resuming all", "error", err)
	}

	t.logger.Debug("resume retried transaction: scanning persisted retry states", "count", len(entries), "confirmed_nonce", confirmed)

	for nonce, rs := range entries {
		stored, err := t.StoredTransaction(rs.CurrentHash)
		if err != nil {
			t.logger.Error(err, "resume retried transaction: stored tx not found, cleaning up", "tx", rs.CurrentHash, "nonce", nonce)
			t.deleteRetryTransaction(&rs)
			continue
		}
		if confirmed > nonce {
			t.logger.Debug("resume retried transaction: skipping already confirmed transaction", "tx", rs.CurrentHash, "nonce", nonce)
			t.deleteRetryTransaction(&rs)
			continue
		}

		t.logger.Debug("resume retried transaction: resuming", "tx", rs.CurrentHash, "nonce", nonce)

		request := &TxRequest{
			To:          stored.To,
			Data:        stored.Data,
			GasLimit:    stored.GasLimit,
			Value:       stored.Value,
			Description: stored.Description,
		}

		nonceWatch := &retryNonceWatch{}
		t.watchRetryNonce(rs.Nonce, nonceWatch)

		var (
			previousTip    *big.Int
			previousFeeCap *big.Int
		)
		if stored.GasTipCap != nil {
			previousTip = new(big.Int).Set(stored.GasTipCap)
		}
		if stored.GasFeeCap != nil {
			previousFeeCap = new(big.Int).Set(stored.GasFeeCap)
		}
		t.wg.Go(func() {
			_, _, _ = t.sendWithRetry(t.ctx, request, &rs, nonceWatch, previousTip, previousFeeCap)
		})
	}
	return nil
}

func addressForLog(addr *common.Address) string {
	if addr == nil {
		return ""
	}
	return addr.Hex()
}
