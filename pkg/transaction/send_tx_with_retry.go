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

// retryState holds in-memory state for a single sendWithRetry session.
type retryState struct {
	nonce         uint64
	nonceAssigned bool
	lastTxHash    common.Hash
}

// SendWithRetry sends an EIP-1559 transaction using fee-history tiers with automatic
// escalation. Each tier gets attemptsPerTier broadcast rounds with fresh eth_feeHistory
// data. A +15% mempool bump floor is applied to ensure replacement transactions are accepted.
func (t *transactionService) SendWithRetry(ctx context.Context, request *TxRequest) (txHash common.Hash, receipt *types.Receipt, err error) {
	if request.GasPrice != nil {
		err = errors.New("send txs with retry requires automatic gas pricing")
		t.retryMetrics.RecordRetryComplete(1, err)
		return common.Hash{}, nil, err
	}
	return t.sendWithRetry(ctx, request, nil)
}

// applyMempoolBump returns tip bumped by mempoolBumpPercent.
func applyMempoolBump(tip *big.Int) *big.Int {
	return new(big.Int).Div(
		new(big.Int).Mul(new(big.Int).Set(tip), big.NewInt(int64(100+mempoolBumpPercent))),
		big.NewInt(100),
	)
}

// suggestGasFeeForTier fetches fresh fee history, picks the tip for the given tier,
// applies the mempool bump floor relative to previousTip, and computes gasFeeCap.
func (t *transactionService) suggestGasFeeForTier(ctx context.Context, tier feeTier, previousTip *big.Int) (gasFeeCap, gasTipCap *big.Int, err error) {
	header, err := t.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, err
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

	t.logger.Debug("suggest gas fees for retry",
		"tier", tier.String(),
		"base_fee", header.BaseFee,
		"previous_tip", previousTip,
		"selected_tip", tip,
		"gas_fee_cap", gasFeeCapWithTip,
		"max_tx_price", t.maxTxPrice)

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
func (t *transactionService) broadcastTx(ctx context.Context, request *TxRequest, fixedNonce *uint64, tier feeTier, previousTip *big.Int) (*types.Transaction, error) {
	var nonce uint64

	if fixedNonce != nil {
		nonce = *fixedNonce
	} else {
		t.lock.Lock()
		defer t.lock.Unlock()

		n, err := t.nextNonce(ctx)
		if err != nil {
			return nil, err
		}
		nonce = n
	}

	gasLimit, err := t.estimateGasLimit(ctx, request)
	if err != nil {
		return nil, err
	}

	gasFeeCap, gasTipCap, err := t.suggestGasFeeForTier(ctx, tier, previousTip)
	if err != nil {
		return nil, err
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

	t.logger.Info("send with retry: broadcast",
		"tier", tier.String(),
		"tx", signedTx.Hash(),
		"nonce", nonce,
		"to", addressForLog(request.To),
		"gas_limit", tx.Gas(),
		"gas_fee_cap", tx.GasFeeCap(),
		"gas_tip_cap", tx.GasTipCap(),
		"value", tx.Value(),
		"data_len", len(tx.Data()),
		"description", request.Description,
	)
	err = t.backend.SendTransaction(ctx, signedTx)
	return signedTx, err
}

// persistReplaceTx atomically persists a new stored+pending tx, updates retry_key, and removes the previous stored+pending entries.
func (t *transactionService) persistReplaceTx(signedTx *types.Transaction, rs *retryState, description string) error {
	if signedTx == nil {
		return nil
	}

	txHash := signedTx.Hash()
	now := time.Now().Unix()

	// Safe ordering: write new → update pointer → delete old.
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
		return err
	}

	if err := t.store.Put(pendingTransactionKey(txHash), struct{}{}); err != nil {
		return err
	}

	if !rs.nonceAssigned {
		rs.nonce = signedTx.Nonce()
		rs.nonceAssigned = true
	}

	retryKey := retryStateKey(rs.nonce)
	if err := t.store.Put(retryKey, txHash); err != nil {
		return err
	}

	oldHash := rs.lastTxHash
	rs.lastTxHash = txHash

	if oldHash != (common.Hash{}) {
		_ = t.store.Delete(pendingTransactionKey(oldHash))
		_ = t.store.Delete(storedTransactionKey(oldHash))
	}

	return nil
}

// attemptResult is the outcome of a single broadcast+wait cycle.
type attemptResult struct {
	signedTx *types.Transaction // successfully broadcast tx (nil if broadcast failed before send)
	receipt  *types.Receipt     // non-nil when receipt was received within the wait window
	err      error              // non-nil on any error; check isNonRetryable to decide whether to stop
}

// sendWithRetry is the core retry loop. When resuming, pass a pre-populated retryState.
func (t *transactionService) sendWithRetry(ctx context.Context, request *TxRequest, rs *retryState) (common.Hash, *types.Receipt, error) {
	if rs == nil {
		rs = &retryState{}
	}

	tiers, err := t.tierRangeForRequest(ctx)
	if err != nil {
		return common.Hash{}, nil, err
	}

	t.logger.Debug("send with retry: started",
		"description", request.Description,
		"to", addressForLog(request.To),
		"start_tier", tiers[0].String(),
		"end_tier", t.endTier.String(),
		"attempts_per_tier", t.attemptsPerTier,
		"retry_delay", t.txRetryDelay,
		"nonce_assigned", rs.nonceAssigned)

	var (
		previousTip    *big.Int
		terminateTxErr error
		nonce          *uint64
	)
	attempt := 1
	defer func() { t.finishRetry(rs, request, attempt, terminateTxErr) }()

	for _, tier := range tiers {
		for k := 0; k < t.attemptsPerTier; k++ {
			if rs.nonceAssigned {
				nonce = &rs.nonce
			}

			res := t.attempt(ctx, rs, request, previousTip, tier, nonce)
			if res.err != nil && (isNonRetryable(res.err) || isNonceTooLow(res.err)) {
				terminateTxErr = res.err
				return common.Hash{}, nil, terminateTxErr
			}

			if res.receipt != nil {
				t.logger.Debug("send with retry: receipt received",
					"tx_hash", rs.lastTxHash,
					"status", res.receipt.Status,
					"gas_used", res.receipt.GasUsed,
					"block_number", res.receipt.BlockNumber,
					"nonce", rs.nonce,
					"description", request.Description)

				if res.receipt.Status == 0 {
					terminateTxErr = ErrTransactionReverted
					return rs.lastTxHash, res.receipt, terminateTxErr
				}
				return rs.lastTxHash, res.receipt, nil
			}

			if res.signedTx != nil {
				previousTip = res.signedTx.GasTipCap()
			}
			attempt++
		}
	}

	terminateTxErr = ErrAllAttemptsExhausted
	return rs.lastTxHash, nil, terminateTxErr
}

// finishRetry performs final cleanup
func (t *transactionService) finishRetry(rs *retryState, request *TxRequest, attempt int, err error) {
	if err != nil {
		t.logger.Error(err,
			"send with retry: finished with error",
			"attempt", attempt,
			"tx_hash", rs.lastTxHash,
			"nonce", rs.nonce,
			"to", addressForLog(request.To),
			"description", request.Description)
	}

	// remove the retry_key
	if rs.nonceAssigned {
		_ = t.store.Delete(retryStateKey(rs.nonce))
	}

	monitorLast := errors.Is(err, ErrTxMaxPriceExceeded) || errors.Is(err, ErrAllAttemptsExhausted)
	if monitorLast {
		// ErrTxMaxPriceExceeded and ErrAllAttemptsExhausted mean retry must be finished even without a receipt.
		// If at least one broadcast succeeded, the tx may still be pending in the mempool and get mined; keep monitoring in the background.
		if rs.lastTxHash != (common.Hash{}) {
			t.waitForPendingTx(rs.lastTxHash)
		}
	} else {
		if rs.lastTxHash != (common.Hash{}) {
			_ = t.store.Delete(pendingTransactionKey(rs.lastTxHash))
		}
	}

	t.retryMetrics.RecordRetryComplete(attempt, err)
}

// attempt performs a single broadcast+wait cycle: broadcast, persist state, wait for receipt.
func (t *transactionService) attempt(ctx context.Context, rs *retryState, request *TxRequest, previousTip *big.Int, tier feeTier, nonce *uint64) attemptResult {
	replaced := true

	signedTx, broadCastErr := t.broadcastTx(ctx, request, nonce, tier, previousTip)
	if broadCastErr != nil {
		switch {
		case isNonceTooLow(broadCastErr):
			// Between retry attempts the transaction was likely mined.
			// Fetch its receipt once and stop retrying regardless of the outcome.
			if rs.lastTxHash != (common.Hash{}) {
				if rec, recErr := t.backend.TransactionReceipt(ctx, rs.lastTxHash); recErr == nil && rec != nil {
					return attemptResult{receipt: rec, err: recErr}
				}
			}
			return attemptResult{err: broadCastErr}
		case isReplacementUnderpriced(broadCastErr):
			// Base fee dropped between attempts within the same tier,
			// so the bumped tip was not enough for the mempool to accept the replacement.
			replaced = false
		case isNonRetryable(broadCastErr):
			return attemptResult{err: broadCastErr}
		case signedTx == nil:
			// Any other error that occurred before the SendTransaction RPC call.
			replaced = false
		}
	}

	if replaced {
		// Persist state only when the transaction was replaced (new tx hash). Otherwise keep monitoring the same pending tx.
		if err := t.persistReplaceTx(signedTx, rs, request.Description); err != nil {
			return attemptResult{err: fmt.Errorf("%w: %w", ErrUpdateRetryState, err)}
		}
	}

	if rs.lastTxHash == (common.Hash{}) {
		// Rare case: no successful broadcast yet, nothing to monitor. Wait to avoid spamming attempts.
		select {
		case <-ctx.Done():
			return attemptResult{err: ctx.Err()}
		case <-time.After(t.txRetryDelay):
			return attemptResult{err: broadCastErr}
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, t.txRetryDelay)
	rec, waitErr := t.WaitForReceipt(waitCtx, rs.lastTxHash)
	cancel()
	if waitErr != nil {
		return attemptResult{signedTx: signedTx, err: waitErr}
	}
	return attemptResult{receipt: rec}
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
func (t *transactionService) pendingRetryTransactions() (map[common.Hash]string, error) {
	out := make(map[common.Hash]string)
	err := t.store.Iterate(retryStatePrefix, func(key, val []byte) (stop bool, err error) {
		var txHash common.Hash
		if err := json.Unmarshal(val, &txHash); err != nil {
			return false, err
		}
		out[txHash] = string(key)
		return false, nil
	})
	return out, err
}

func (t *transactionService) resumeRetryTransactions() error {
	entries, err := t.pendingRetryTransactions()
	if err != nil {
		return err
	}

	confirmed, err := t.backend.NonceAt(t.ctx, t.sender, nil)
	if err != nil {
		t.logger.Warning("resume send with retry: failed to get confirmed nonce, resuming all", "error", err)
	}

	t.logger.Debug("resume send with retry: scanning persisted retry states", "count", len(entries), "confirmed_nonce", confirmed)

	for txHash, key := range entries {
		stored, sErr := t.StoredTransaction(txHash)
		if sErr != nil {
			t.logger.Warning("resume send with retry: stored tx not found, cleaning up", "tx_hash", txHash, "error", sErr)
			_ = t.store.Delete(key)
			continue
		}

		if confirmed > stored.Nonce {
			t.logger.Debug("resume send with retry: skipping already confirmed transaction", "nonce", stored.Nonce)
			_ = t.store.Delete(key)
			_ = t.store.Delete(pendingTransactionKey(txHash))
			_ = t.store.Delete(storedTransactionKey(txHash))
			continue
		}

		t.logger.Debug("resume send with retry: resuming", "nonce", stored.Nonce, "tx_hash", txHash)

		request := &TxRequest{
			To:          stored.To,
			Data:        stored.Data,
			GasLimit:    stored.GasLimit,
			Value:       stored.Value,
			Description: stored.Description,
		}

		rs := &retryState{
			nonce:         stored.Nonce,
			nonceAssigned: true,
			lastTxHash:    txHash,
		}

		t.wg.Go(func() {
			if _, _, err := t.sendWithRetry(t.ctx, request, rs); err != nil {
				t.logger.Error(err, "resumed transaction send with finished with error", "nonce", stored.Nonce)
			}
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
