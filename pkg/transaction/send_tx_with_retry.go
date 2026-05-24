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
)

const retryStatePrefix = "transaction_retry_"

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
	NextAttempt   int             `json:"next_attempt"`
	LastTxHash    common.Hash     `json:"last_tx_hash"`
	AllTxHashes   []common.Hash   `json:"all_tx_hashes"`
	GasLimit      uint64          `json:"gas_limit"`
	To            *common.Address `json:"to,omitempty"`
	Data          []byte          `json:"data,omitempty"`
	Value         *big.Int        `json:"value,omitempty"`
	Description   string          `json:"description,omitempty"`

	// PreviousTip is the maxPriorityFeePerGas used in the last successful broadcast.
	// Each retry escalates from this value by (100+GasIncreasePercent)/100.
	PreviousTip *big.Int `json:"previous_tip,omitempty"`
}

func retryStateKey(nonce uint64) string {
	return fmt.Sprintf("%s%020d", retryStatePrefix, nonce)
}

// SendWithRetry sends an EIP-1559 transaction using one eth_feeHistory snapshot for the initial tip,
// then increases gas tip by gas_increase_percent after each unsuccessful wait, up to max_retries.
// Optional RetryOption values can override per-call retry behaviour (e.g. bypass price cap).
func (t *transactionService) SendWithRetry(ctx context.Context, request *TxRequest, opts ...RetryOption) (txHash common.Hash, receipt *types.Receipt, err error) {
	if request.GasPrice != nil {
		err = errors.New("send txs with retry requires automatic gas pricing")
		t.recordRetryComplete(0, err)
		return common.Hash{}, nil, err
	}
	return t.retry(ctx, "", request, applyRetryOptions(opts))
}

// escalateGasTip returns tip * (100+increasePct)/100 — a single escalation step.
func escalateGasTip(tip *big.Int, increasePct int) *big.Int {
	if tip == nil {
		return nil
	}
	return new(big.Int).Div(new(big.Int).Mul(new(big.Int).Set(tip), big.NewInt(int64(100+increasePct))), big.NewInt(100))
}

// suggestGasFeeGasTipCapWithHistory returns maxFeePerGas (gasFeeCap) and maxPriorityFeePerGas (gasTipCap)
// for transactions with retry. It reads the latest block base fee and picks a priority fee, then sets
// gasFeeCap = 2*baseFee + tip (same formula as wrapped.SuggestedFeeAndTip).
//
// Priority fee selection:
//   - First attempt (prevGasTipCap nil or zero): one eth_feeHistory snapshot via
//     SuggestedFeeAndTipsFromHistory; MarketTip is used as gasTipCap.
//   - Later attempts: gasTipCap = prevGasTipCap * (100 + txRetryGasIncreasePercent) / 100.
//
// When maxTxPrice is set and 2*baseFee + escalated tip exceeds it, the function broadcasts with the
// un-escalated previous tip (2*baseFee + prevGasTipCap) instead. If that fee cap still exceeds
// maxTxPrice, it returns ErrTxMaxPriceExceeded.
//
// When overrides.IgnoreMaxPrice is set and returns true, the maxTxPrice cap is bypassed.
func (t *transactionService) suggestGasFeeGasTipCapWithHistory(ctx context.Context, prevGasTipCap *big.Int, overrides *RetryOverrides) (gasFeeCap, gasTipCap *big.Int, err error) {
	header, err := t.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, prevGasTipCap, err
	}
	if header == nil || header.BaseFee == nil {
		return nil, prevGasTipCap, fmt.Errorf("latest block header or base fee unavailable")
	}

	var escalatedGasTip *big.Int
	if prevGasTipCap == nil || prevGasTipCap.Sign() == 0 {
		fh, err := t.backend.SuggestedFeeAndTipsFromHistory(ctx, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("fee history: %w", err)
		}
		if fh == nil {
			return nil, nil, errors.New("fee history: missing base fee")
		}
		escalatedGasTip = fh.MarketTip
		prevGasTipCap = fh.MarketTip
	} else {
		escalatedGasTip = escalateGasTip(prevGasTipCap, t.txRetryGasIncreasePercent)
	}

	gasFeeCap = new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	gasFeeCapWithEscalatedTip := new(big.Int).Add(new(big.Int).Set(gasFeeCap), escalatedGasTip)
	gasFeeCapWithPreviousTip := new(big.Int).Add(new(big.Int).Set(gasFeeCap), prevGasTipCap)

	t.logger.V(1).Register().Debug("suggest gas fees for retry",
		"base_fee", header.BaseFee,
		"previous_tip", prevGasTipCap,
		"escalated_tip", escalatedGasTip,
		"gas_fee_cap_with_escalated_tip", gasFeeCapWithEscalatedTip,
		"gas_fee_cap_with_previous_tip", gasFeeCapWithPreviousTip,
		"max_tx_price", t.maxTxPrice)

	canOverride := func(feeCap *big.Int) bool {
		return overrides != nil && overrides.IgnoreMaxPrice != nil && overrides.IgnoreMaxPrice(feeCap)
	}

	if t.maxTxPrice != nil && gasFeeCapWithEscalatedTip.Cmp(t.maxTxPrice) > 0 {
		if canOverride(gasFeeCapWithEscalatedTip) {
			t.logger.Info("max price override: bypassing limit",
				"escalated_gas_fee_cap", gasFeeCapWithEscalatedTip,
				"max_tx_price", t.maxTxPrice)
			return gasFeeCapWithEscalatedTip, escalatedGasTip, nil
		}

		t.logger.Warning("gas cap fee with escalated gas tip is too high, fallback to previous gas tip",
			"escalated_gas_tip_cap", escalatedGasTip.String(),
			"escalated_gas_fee_cap", gasFeeCapWithEscalatedTip.String(),
			"previous_gas_tip_cap", prevGasTipCap.String())

		if gasFeeCapWithPreviousTip.Cmp(t.maxTxPrice) > 0 {
			return nil, nil, fmt.Errorf("%w: max_fee_per_gas %s exceeds limit %s", ErrTxMaxPriceExceeded, gasFeeCap, t.maxTxPrice)
		}
		return gasFeeCapWithPreviousTip, prevGasTipCap, nil
	}
	return gasFeeCapWithEscalatedTip, escalatedGasTip, nil
}

func (t *transactionService) prepareTransactionWithRetry(ctx context.Context, request *TxRequest, nonce uint64, prevGasTipCap *big.Int, overrides *RetryOverrides) (*types.Transaction, error) {
	gasLimit, err := t.estimateGasLimit(ctx, request)
	if err != nil {
		return nil, err
	}

	gasFeeCap, newGasTipCap, err := t.suggestGasFeeGasTipCapWithHistory(ctx, prevGasTipCap, overrides)
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
		GasTipCap: newGasTipCap,
		Data:      request.Data,
	})
	return tx, nil
}

// broadcastTx prepares, signs, and sends a transaction.
// When fixedNonce is nil a new nonce is allocated (first attempt);
// otherwise the supplied nonce is reused (replacement transaction).
func (t *transactionService) broadcastTx(ctx context.Context, request *TxRequest, fixedNonce *uint64, gasTipCap *big.Int, attempt int, overrides *RetryOverrides) (*types.Transaction, error) {
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
	tx, err := t.prepareTransactionWithRetry(ctx, request, nonce, gasTipCap, overrides)
	if err != nil {
		return nil, err
	}

	signedTx, err := t.signer.SignTx(tx, t.chainID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSignTransaction, err)
	}

	t.logger.Info("send with retry: broadcast",
		"attempt", attempt,
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

	t.recordRetryBroadcast(attempt, tx.GasTipCap(), tx.GasFeeCap())

	err = t.backend.SendTransaction(ctx, signedTx)
	return signedTx, err
}

func (t *transactionService) deleteRetryStateAndPending(retryKey string, state TransactionRetryState) {
	if retryKey == "" {
		return
	}
	_ = t.store.Delete(retryKey)
	for _, h := range state.AllTxHashes {
		_ = t.store.Delete(pendingTransactionKey(h))
	}
	if state.LastTxHash != (common.Hash{}) {
		_ = t.store.Delete(pendingTransactionKey(state.LastTxHash))
	}
}
func (t *transactionService) retry(ctx context.Context, txRetryKey string, request *TxRequest, overrides *RetryOverrides) (common.Hash, *types.Receipt, error) {
	var (
		txState TransactionRetryState
		nonce   *uint64
	)

	if txRetryKey != "" {
		if err := t.store.Get(txRetryKey, &txState); err != nil {
			return common.Hash{}, nil, err
		}
	}

	if request == nil {
		request = &TxRequest{
			To:          txState.To,
			Data:        txState.Data,
			GasLimit:    txState.GasLimit,
			Value:       txState.Value,
			Description: txState.Description,
		}
	}

	loggerV1 := t.logger.V(1).Register()
	loggerV1.Debug("send with retry: started",
		"description", request.Description,
		"to", retryToForLog(request, &txState),
		"max_retries", t.txMaxRetries,
		"retry_delay", t.txRetryDelay,
		"gas_increase_percent", t.txRetryGasIncreasePercent,
		"resume_from_attempt", txState.NextAttempt,
		"nonce_assigned", txState.NonceAssigned,
		"previous_tip", txState.PreviousTip)

	retryDelay := t.txRetryDelay
	if overrides != nil && overrides.RetryDelay != nil {
		retryDelay = overrides.RetryDelay(t.txRetryDelay)
	}

	for attempt := txState.NextAttempt; attempt < t.txMaxRetries; attempt++ {
		if txState.NonceAssigned {
			nonce = &txState.Nonce
		}

		signedTx, err := t.broadcastTx(ctx, request, nonce, txState.PreviousTip, attempt, overrides)
		if err != nil {
			if isErrCritical(err) {
				t.logger.Error(err,
					"transaction with retry: broadcast failed with critical error, stop retry",
					"attempt", attempt, "nonce", nonce, "to", retryToForLog(request, &txState))
				t.deleteRetryStateAndPending(txRetryKey, txState)
				t.recordRetryComplete(txState.NextAttempt, err)
				return common.Hash{}, nil, err
			}
			t.logger.Warning("transaction retry broadcast failed, will retry", "attempt", attempt, "error", err, "to", retryToForLog(request, &txState))
		}

		if err := t.updateStates(signedTx, &txState); err != nil {
			t.logger.Error(err,
				"transaction with retry: failed update states, stop retry",
				"attempt", attempt, "nonce", nonce, "to", retryToForLog(request, &txState))
			t.deleteRetryStateAndPending(txRetryKey, txState)
			t.recordRetryComplete(txState.NextAttempt, err)
			return common.Hash{}, nil, err
		}

		if txState.NonceAssigned {
			txRetryKey = retryStateKey(txState.Nonce)
		}

		loggerV1.Debug("send with retry: state updated",
			"attempt", attempt,
			"tx_hash", txState.LastTxHash,
			"nonce", txState.Nonce,
			"nonce_assigned", txState.NonceAssigned,
			"previous_tip", txState.PreviousTip,
			"description", request.Description)

		delay := retryDelay

		if txState.LastTxHash == (common.Hash{}) {
			loggerV1.Debug("send with retry: no tx hash after broadcast failure, waiting before next attempt",
				"attempt", attempt,
				"retry_delay", delay,
				"description", request.Description)
			select {
			case <-ctx.Done():
				err := ctx.Err()
				t.recordRetryComplete(txState.NextAttempt, err)
				return common.Hash{}, nil, err
			case <-time.After(delay):
				continue
			}
		}

		waitCtx, cancel := context.WithTimeout(ctx, delay)
		rec, waitErr := t.WaitForReceipt(waitCtx, txState.LastTxHash)
		cancel()

		if waitErr == nil {
			loggerV1.Debug("send with retry: receipt received",
				"tx_hash", txState.LastTxHash,
				"status", rec.Status,
				"gas_used", rec.GasUsed,
				"block_number", rec.BlockNumber,
				"nonce", txState.Nonce,
				"description", request.Description)
			t.deleteRetryStateAndPending(txRetryKey, txState)
			if rec.Status == 0 {
				t.recordRetryComplete(txState.NextAttempt, ErrTransactionReverted)
				return txState.LastTxHash, rec, ErrTransactionReverted
			}
			t.recordRetryComplete(txState.NextAttempt, nil)
			return txState.LastTxHash, rec, nil
		} else if isErrCritical(waitErr) {
			t.logger.Error(waitErr,
				"send with retry: wait for receipt failed with critical error, stop retry",
				"attempt", attempt,
				"tx_hash", txState.LastTxHash,
				"nonce", txState.Nonce,
				"description", request.Description)
			t.deleteRetryStateAndPending(txRetryKey, txState)
			t.recordRetryComplete(txState.NextAttempt, waitErr)
			return common.Hash{}, nil, waitErr
		} else {
			loggerV1.Debug("send with retry: receipt not received, will escalate gas",
				"attempt", attempt,
				"tx_hash", txState.LastTxHash,
				"nonce", txState.Nonce,
				"wait_error", waitErr,
				"description", request.Description)
		}
	}

	exhaustionErr := fmt.Errorf("transaction failed after %d attempts (nonce=%d, description=%s)", t.txMaxRetries, txState.Nonce, txState.Description)
	t.logger.Error(exhaustionErr,
		"send with retry: all attempts exhausted",
		"max_retries", t.txMaxRetries,
		"nonce", txState.Nonce,
		"last_tx_hash", txState.LastTxHash,
		"description", txState.Description)
	t.deleteRetryStateAndPending(txRetryKey, txState)
	t.recordRetryComplete(txState.NextAttempt, exhaustionErr)
	return txState.LastTxHash, nil, exhaustionErr
}

func (t *transactionService) updateStates(signedTx *types.Transaction, txState *TransactionRetryState) error {
	if txState.LastTxHash != (common.Hash{}) {
		txState.AllTxHashes = append(txState.AllTxHashes, txState.LastTxHash)
		_ = t.store.Delete(pendingTransactionKey(txState.LastTxHash))
	}

	txState.NextAttempt++

	if signedTx == nil {
		txState.LastTxHash = common.Hash{}
	} else {
		txHash := signedTx.Hash()
		now := time.Now().Unix()

		if err := t.store.Put(storedTransactionKey(txHash), StoredTransaction{
			To:        signedTx.To(),
			Data:      signedTx.Data(),
			GasPrice:  signedTx.GasPrice(),
			GasLimit:  signedTx.Gas(),
			GasTipCap: signedTx.GasTipCap(),
			GasFeeCap: signedTx.GasFeeCap(),
			Value:     signedTx.Value(),
			Nonce:     signedTx.Nonce(),
			Created:   now,
		}); err != nil {
			return err
		}

		if err := t.store.Put(pendingTransactionKey(txHash), struct{}{}); err != nil {
			return err
		}

		txState.LastTxHash = txHash
		txState.PreviousTip = signedTx.GasTipCap()

		if !txState.NonceAssigned {
			txState.Nonce = signedTx.Nonce()
			txState.NonceAssigned = true
			txState.GasLimit = signedTx.Gas()
			txState.To = signedTx.To()
			txState.Data = signedTx.Data()
			txState.Value = signedTx.Value()
		}
	}
	if txState.NonceAssigned {
		return t.store.Put(retryStateKey(txState.Nonce), txState)
	}
	return nil
}

func isErrCritical(err error) bool {
	if errors.Is(err, ErrTransactionReverted) ||
		errors.Is(err, ErrTransactionCancelled) ||
		errors.Is(err, ErrSignTransaction) ||
		errors.Is(err, context.Canceled) {
		return true
	}

	s := err.Error()
	nonRetryable := []string{
		"specified gas price",
		"nonce too low",
		"AlreadyCommitted",
		"AlreadyRevealed",
		"AlreadyClaimed",
		"NotCommitPhase",
		"NotRevealPhase",
		"NotClaimPhase",
		"CommitRoundOver",
		"CommitRoundNotStarted",
		"PhaseLastBlock",
		"OutOfDepth",
		"OutOfDepthReveal",
		"OutOfDepthClaim",
		"NotStaked",
		"MustStake2Rounds",
		"NoReveals",
		"NoCommitsReceived",
		"execution reverted",
		"insufficient funds",
	}
	for _, sub := range nonRetryable {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func (t *transactionService) retryPendingHashes() (map[common.Hash]struct{}, error) {
	out := make(map[common.Hash]struct{})
	err := t.store.Iterate(retryStatePrefix, func(key, val []byte) (stop bool, err error) {
		var s TransactionRetryState
		if uErr := json.Unmarshal(val, &s); uErr != nil {
			return false, uErr
		}
		for _, h := range s.AllTxHashes {
			out[h] = struct{}{}
		}
		if s.LastTxHash != (common.Hash{}) {
			out[s.LastTxHash] = struct{}{}
		}
		return false, nil
	})
	return out, err
}

func (t *transactionService) resumeRetryTransactions() error {
	var keys []string
	var states []TransactionRetryState
	err := t.store.Iterate(retryStatePrefix, func(key, val []byte) (stop bool, err error) {
		var s TransactionRetryState
		if uErr := json.Unmarshal(val, &s); uErr != nil {
			return false, uErr
		}
		keys = append(keys, string(key))
		states = append(states, s)
		return false, nil
	})
	if err != nil {
		return err
	}

	confirmed, err := t.backend.NonceAt(t.ctx, t.sender, nil)
	if err != nil {
		t.logger.Warning("resume retry: failed to get confirmed nonce, resuming all", "error", err)
	}

	loggerV1 := t.logger.V(1).Register()
	loggerV1.Debug("resume retry: scanning persisted retry states",
		"count", len(keys),
		"confirmed_nonce", confirmed)

	for i := range keys {
		key := keys[i]
		state := states[i]

		if confirmed > state.Nonce {
			loggerV1.Debug("resume retry: skipping already confirmed transaction",
				"nonce", state.Nonce,
				"confirmed_nonce", confirmed,
				"description", state.Description)
			t.deleteRetryStateAndPending(key, state)
			continue
		}

		loggerV1.Debug("resume retry: resuming persisted retry",
			"nonce", state.Nonce,
			"next_attempt", state.NextAttempt,
			"last_tx_hash", state.LastTxHash,
			"previous_tip", state.PreviousTip,
			"description", state.Description)

		sk := key
		st := state
		t.wg.Go(func() {
			if _, _, err := t.retry(t.ctx, sk, nil, nil); err != nil {
				t.logger.Error(err, "resumed transaction retry aborted", "nonce", st.Nonce, "description", st.Description)
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

func retryToForLog(req *TxRequest, state *TransactionRetryState) string {
	if state != nil && state.To != nil {
		return state.To.Hex()
	}
	if req != nil && req.To != nil {
		return req.To.Hex()
	}
	return ""
}
