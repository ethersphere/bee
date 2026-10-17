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

const (
	retryStatePrefix = "transaction_retry_"
	rpcCallTimeout   = 15 * time.Second
)

// mempoolBumpPercent is the minimum percent increase required by EIP-1559
// mempools to accept a replacement transaction for the same nonce.
const mempoolBumpPercent = 15

// defaultAttemptsPerTier is the number of broadcast attempts at each fee tier
// before escalating to the next tier.
const defaultAttemptsPerTier = 2

// RetryOverrides controls per-call behaviour overrides for SendWithRetry.
// Fields are optional; nil means "use default behaviour".
type RetryOverrides struct {
	// IgnoreMaxPrice is called when maxTxPrice would block a broadcast.
	// It receives the gasFeeCap (max fee per gas, wei) that would be used
	// for this attempt. If it returns true, the price cap is bypassed.
	IgnoreMaxPrice func(gasFeeCap *big.Int) bool

	// RetryDelay, if set, rewrites the configured delay between attempts.
	RetryDelay func(time.Duration) time.Duration
}

// RetryOption configures per-call overrides for SendWithRetry.
type RetryOption func(*RetryOverrides)

// WithIgnoreMaxPrice returns a RetryOption that installs a predicate called
// whenever the configured maxTxPrice would block a broadcast.  The predicate
// receives the gasFeeCap (max fee per gas, wei) that would be used.  When fn
// returns true the price cap is bypassed for that attempt.
func WithIgnoreMaxPrice(fn func(gasFeeCap *big.Int) bool) RetryOption {
	return func(o *RetryOverrides) { o.IgnoreMaxPrice = fn }
}

// WithRetryDelay returns a RetryOption that rewrites the configured retry delay.
func WithRetryDelay(fn func(time.Duration) time.Duration) RetryOption {
	return func(o *RetryOverrides) { o.RetryDelay = fn }
}

func applyRetryOptions(opts []RetryOption) *RetryOverrides {
	if len(opts) == 0 {
		return nil
	}
	var o RetryOverrides
	for _, fn := range opts {
		fn(&o)
	}
	return &o
}

// TransactionRetryState is persisted so transactions with retry can resume after a node restart.
type TransactionRetryState struct {
	Nonce         uint64          `json:"nonce"`
	NonceAssigned bool            `json:"nonce_assigned"`
	LastTxHash    common.Hash     `json:"last_tx_hash"`
	AllTxHashes   []common.Hash   `json:"all_tx_hashes"`
	GasLimit      uint64          `json:"gas_limit"`
	To            *common.Address `json:"to,omitempty"`
	Data          []byte          `json:"data,omitempty"`
	Value         *big.Int        `json:"value,omitempty"`
	Description   string          `json:"description,omitempty"`
}

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
// data. A +15% mempool bump floor is applied to ensure replacement transactions are accepted.
// Optional RetryOption values can override per-call retry behaviour (e.g. bypass price cap).
func (t *transactionService) SendWithRetry(ctx context.Context, request *TxRequest, opts ...RetryOption) (txHash common.Hash, receipt *types.Receipt, err error) {
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
func (t *transactionService) suggestGasFeeForTier(ctx context.Context, tier feeTier, previousTip, previousFeeCap *big.Int, overrides *RetryOverrides) (gasFeeCap, gasTipCap *big.Int, err error) {
	callCtx, cancel := context.WithTimeout(ctx, rpcCallTimeout)
	header, err := t.backend.HeaderByNumber(callCtx, nil)
	cancel()
	if err != nil {
		return nil, nil, fmt.Errorf("block header: %w", err)
	}
	if header == nil || header.BaseFee == nil {
		return nil, nil, errors.New("latest block header or base fee unavailable")
	}

	callCtx, cancel = context.WithTimeout(ctx, rpcCallTimeout)
	fh, err := t.backend.SuggestedFeeAndTipsFromHistory(callCtx, nil)
	cancel()
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

	canOverride := func(feeCap *big.Int) bool {
		return overrides != nil && overrides.IgnoreMaxPrice != nil && overrides.IgnoreMaxPrice(feeCap)
	}

	if t.maxTxPrice != nil && gasFeeCapWithTip.Cmp(t.maxTxPrice) > 0 {
		if !canOverride(gasFeeCapWithTip) {
			return nil, nil, fmt.Errorf("%w: max_fee_per_gas %s exceeds limit %s", ErrTxMaxPriceExceeded, gasFeeCapWithTip, t.maxTxPrice)
		}

		t.logger.Info("max price override: bypassing limit", "escalated_gas_fee_cap", gasFeeCapWithTip, "max_tx_price", t.maxTxPrice)
	}

	return gasFeeCapWithTip, tip, nil
}

// 1. Only the start tier is overridden, and only upward.
// 2. The start tier cannot be higher than the end tier.
// 3. The end tier stays under the node operator's control.
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
func (t *transactionService) broadcastTx(ctx context.Context, request *TxRequest, fixedNonce *uint64, tier feeTier, previousTip *big.Int, overrides *RetryOverrides) (*types.Transaction, error) {
	var nonce uint64

	if fixedNonce != nil {
		nonce = *fixedNonce
	} else {
		// The lock must be held until the transaction is broadcasted.
		// Otherwise another transaction can take the same nonce.
		t.lock.Lock()
		defer t.lock.Unlock()

		callCtx, cancel := context.WithTimeout(ctx, rpcCallTimeout)
		var err error
		nonce, err = t.nextNonce(callCtx)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("next nonce: %w", err)
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, rpcCallTimeout)
	gasLimit, err := t.estimateGasLimit(callCtx, request)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("estimate gas: %w", err)
	}

	gasFeeCap, gasTipCap, err := t.suggestGasFeeForTier(ctx, tier, previousTip, previousFeeCap, overrides)
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
	callCtx, cancel = context.WithTimeout(ctx, rpcCallTimeout)
	err = t.backend.SendTransaction(callCtx, signedTx)
	cancel()
	if err != nil {
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

// sendWithRetry runs the broadcast loop.
// Handles the following cases:
//  1. This is the first attempt to send the transaction. No data is stored yet.
//  2. This is a resume of a retried transaction: at least one send succeeded and its data was stored on disk.
//     In that case we use data to restore session.
func (t *transactionService) sendWithRetry(
	ctx context.Context,
	request *TxRequest,
	rs *RetriedTransaction,
	nonceWatch *retryNonceWatch, previousTip, previousFeeCap *big.Int,
	overrides *RetryOverrides) (common.Hash, *types.Receipt, error) {
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

	txHash, receipt, attempts, terminateTxErr := t.retryLoop(ctx, request, rs, nonceWatch, previousTip, previousFeeCap, tiers)
	t.finishRetry(rs, attempts, terminateTxErr, nonceWatch, txHash)
	return txHash, receipt, terminateTxErr
}

// retryLoop broadcasts and waits tier by tier until a receipt, a terminal error, or exhaustion.
func (t *transactionService) retryLoop(
	ctx context.Context, request *TxRequest,
	rs *RetriedTransaction, nonceWatch *retryNonceWatch,
	previousTip, previousFeeCap *big.Int, tiers []feeTier) (common.Hash, *types.Receipt, int, error) {

	var (
		nonce    *uint64
		attempts int // track total number of attempts for metrics
	)
	for _, tier := range tiers {
		for k := 0; k < t.attemptsPerTier; k++ {
			attempts++
			if rs.NonceAssigned {
				nonce = &rs.Nonce
			}

			signedTx, receipt, err := t.attempt(ctx, rs, request, previousTip, previousFeeCap, tier, nonce, nonceWatch, overrides)
			if err != nil && (isNonRetryable(err) || isNonceTooLow(err)) {
				return common.Hash{}, nil, attempts, err
			}

			if receipt != nil {
				t.logger.Info("retried transaction: receipt received", "tx", receipt.TxHash, "nonce", rs.Nonce)

				if receipt.Status == 0 {
					return receipt.TxHash, receipt, attempts, ErrTransactionReverted
				}
				return receipt.TxHash, receipt, attempts, nil
			}

			if signedTx != nil {
				previousTip = new(big.Int).Set(signedTx.GasTipCap())
				previousFeeCap = new(big.Int).Set(signedTx.GasFeeCap())
			}
		}
	}
	return rs.CurrentHash, nil, attempts, ErrAllAttemptsExhausted
}

// finishRetry ends the retry session for one transaction.
//  1. Receipt received: delete every stored transaction except the one the receipt
//     belongs to. Otherwise, one retried send leaves N records on disk.
//  2. No receipt and the error is not fatal: keep watching the transaction in the background.
//  3. No receipt and the transaction was canceled: its nonce was mined, but no receipt
//     was found for the known hashes. Delete every record of the transaction from the store.
func (t *transactionService) finishRetry(rs *RetriedTransaction, attempt int, terminateTxErr error, nonceWatch *retryNonceWatch, txHash common.Hash) {
	if terminateTxErr != nil {
		t.logger.Error(
			terminateTxErr,
			"retried transaction: finished with error",
			"tx", rs.CurrentHash, "nonce", rs.Nonce, "attempt", attempt)
	}

	switch {
	case terminateTxErr == nil || errors.Is(terminateTxErr, ErrTransactionReverted):
		t.clearRetrySession(rs, txHash)
	case errors.Is(terminateTxErr, ErrTransactionCancelled):
		t.clearRetrySession(rs, common.Hash{})
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

// receiptForRetryHashes looks up a receipt for each known transaction hash, newest first.
// It is done to cover corner case:
//  1. A transaction with hash A was included in a block.
//  2. The mempool has not learned that yet and accepts a replacement with hash B.
//  3. The nonce watch learns that the nonce was mined.
//  4. A lookup of only the latest hash returns no receipt, so the transaction would be marked cancelled even though it was executed.
func (t *transactionService) receiptForRetryHashes(ctx context.Context, rs *RetriedTransaction) *types.Receipt {
	hashes := rs.allHashes()
	for i := len(hashes) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return nil
		}
		callCtx, cancel := context.WithTimeout(ctx, rpcCallTimeout)
		receipt, _ := t.backend.TransactionReceipt(callCtx, hashes[i])
		cancel()
		if receipt != nil {
			return receipt
		}
	}
	return nil
}

// attempt performs a single broadcast+wait cycle: broadcast, persist state, wait for receipt.
func (t *transactionService) attempt(ctx context.Context, rs *RetriedTransaction, request *TxRequest, previousTip, previousFeeCap *big.Int, tier feeTier, nonce *uint64, nonceWatch *retryNonceWatch, overrides *RetryOverrides) (*types.Transaction, *types.Receipt, error) {
	replaced := true

	signedTx, broadCastErr := t.broadcastTx(ctx, request, nonce, tier, previousTip, previousFeeCap, overrides)
	if broadCastErr != nil {
		switch {
		case isNonceTooLow(broadCastErr):
			// Nonce was already used
			if receipt := t.receiptForRetryHashes(ctx, rs); receipt != nil {
				// Our transaction was mined between attempts, receipt found by one of known hashes.
				return nil, receipt, nil
			}
			// Another tx used same nonce and was mined.
			return nil, nil, broadCastErr
		case isReplacementUnderpriced(broadCastErr):
			// Bumped gas fee cap and gas tip cap wasn't enough to make mempool replace transaction
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
			// Any other error occurred, but it is not fatal. Transaction from previous attempt still in mempool.
			replaced = false
		}
	}

	if replaced {
		// save successful attempt on disk
		persistErr := t.persistReplaceTx(signedTx, rs, request.Description)

		// monitor nonce in any case
		t.watchRetryNonce(signedTx.Nonce(), nonceWatch)

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

// waitNonceChanged waits up to the timeout for the monitor to signal that the nonce was mined.
func (t *transactionService) waitNonceChanged(ctx context.Context, rs *RetriedTransaction, nonceWatch *retryNonceWatch, signedTx *types.Transaction, timeout time.Duration) (*types.Transaction, *types.Receipt, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-nonceWatch.doneC:
		if receipt := t.receiptForRetryHashes(waitCtx, rs); receipt != nil {
			return nil, receipt, nil
		}
		// Another tx used same nonce and was mined.
		return signedTx, nil, ErrTransactionCancelled
	case err := <-nonceWatch.errC:
		return signedTx, nil, fmt.Errorf("nonce watch: %w", err)
	case <-waitCtx.Done():
		return signedTx, nil, fmt.Errorf("wait for nonce: %w", waitCtx.Err())
	}
}

// waitForPendingRetry watches the transaction in the background with no time limit.
// It is used when the retry continue is not possible anymore and at least one broadcast to the network succeeded.
func (t *transactionService) waitForPendingRetry(rs *RetriedTransaction, nonceWatch *retryNonceWatch) {
	t.wg.Go(func() {
		select {
		case <-nonceWatch.doneC:
			if receipt := t.receiptForRetryHashes(t.ctx, rs); receipt != nil {
				t.logger.Info("pending retried transaction confirmed", "tx", receipt.TxHash, "nonce", rs.Nonce)
				t.clearRetrySession(rs, receipt.TxHash)
			} else {
				t.logger.Warning("pending retried transaction cancelled", "tx", rs.CurrentHash, "nonce", rs.Nonce)
				t.clearRetrySession(rs, common.Hash{})
			}

		case err := <-nonceWatch.errC:
			if errors.Is(err, ErrTransactionCancelled) {
				t.logger.Warning("pending retried transaction cancelled", "tx", rs.CurrentHash, "nonce", rs.Nonce)
				t.clearRetrySession(rs, common.Hash{})
			} else if !errors.Is(err, ErrMonitorClosed) {
				t.logger.Error(err, "waiting for pending retried transaction failed", "tx", rs.CurrentHash, "nonce", rs.Nonce)
			}
		case <-t.ctx.Done():
		}
	})
}

// clearRetrySession removes the retry session and every pending key.
// The stored transaction is kept only for resultHash. A zero hash keeps none.
func (t *transactionService) clearRetrySession(rs *RetriedTransaction, resultHash common.Hash) {
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

func (t *transactionService) resumeRetryTransactions() error {
	entries, err := t.pendingRetryTransactions()
	if err != nil {
		return fmt.Errorf("pending retry transactions: %w", err)
	}

	callCtx, cancel := context.WithTimeout(t.ctx, rpcCallTimeout)
	confirmed, err := t.backend.NonceAt(callCtx, t.sender, nil)
	cancel()
	if err != nil {
		t.logger.Warning("resume retried transaction: failed to get confirmed nonce, resuming all", "error", err)
	}

	t.logger.Debug("resume retried transaction: scanning persisted retry states", "count", len(entries), "confirmed_nonce", confirmed)

	for nonce, rs := range entries {
		if confirmed > nonce {
			t.logger.Debug("resume retried transaction: skipping already confirmed transaction", "nonce", nonce)
			if receipt := t.receiptForRetryHashes(t.ctx, &rs); receipt != nil {
				t.clearRetrySession(&rs, receipt.TxHash)
			} else {
				t.clearRetrySession(&rs, common.Hash{})
			}
			continue
		}

		stored, err := t.StoredTransaction(rs.CurrentHash)
		if err != nil {
			t.logger.Error(err, "resume retried transaction: stored tx not found, cleaning up", "tx", rs.CurrentHash, "nonce", nonce)
			t.clearRetrySession(&rs, common.Hash{})
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

		// A transaction from this session is already in the network, and its hashes are
		// excluded from the normal pending watch. The resumed broadcast may fail before
		// it starts its own watch, so subscribe before that attempt.
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
			_, _, _ = t.sendWithRetry(t.ctx, request, &rs, nonceWatch, previousTip, previousFeeCap, nil)
		})
	}
	return nil
}
