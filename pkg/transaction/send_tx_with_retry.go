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

// SendWithRetry sends an EIP-1559 transaction using fee-history tiers with automatic
// escalation. Each tier gets attemptsPerTier broadcast rounds with fresh eth_feeHistory
// data. A +15% mempool bump floor is applied to ensure replacement transactions are accepted.
// Optional RetryOption values can override per-call retry behaviour (e.g. bypass price cap).
func (t *transactionService) SendWithRetry(ctx context.Context, request *TxRequest, opts ...RetryOption) (txHash common.Hash, receipt *types.Receipt, err error) {
	if request.GasPrice != nil {
		err = errors.New("send txs with retry requires automatic gas pricing")
		t.recordRetryComplete(1, err)
		return common.Hash{}, nil, err
	}
	return t.retry(ctx, "", request, applyRetryOptions(opts))
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
func (t *transactionService) suggestGasFeeForTier(ctx context.Context, tier feeTier, previousTip *big.Int, overrides *RetryOverrides) (gasFeeCap, gasTipCap *big.Int, err error) {
	header, err := t.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	if header == nil || header.BaseFee == nil {
		return nil, nil, errors.New("latest block header or base fee unavailable")
	}

	// get fee history
	fh, err := t.backend.SuggestedFeeAndTipsFromHistory(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("fee history: %w", err)
	}
	if fh == nil {
		return nil, nil, errors.New("fee history: empty response")
	}

	tip := tierTip(tier, fh)

	// Apply mempool bump floor: replacement tx must pay at least +15% over previous.
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

func (t *transactionService) tierRangeForRequest(ctx context.Context) ([]feeTier, error) {
	start := t.startTier
	if override := sctx.GetFeePriority(ctx); override != "" {
		parsed, err := parseFeeTier(override)
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

func (t *transactionService) prepareTransactionForTier(ctx context.Context, request *TxRequest, nonce uint64, tier feeTier, previousTip *big.Int, overrides *RetryOverrides) (*types.Transaction, error) {
	gasLimit, err := t.estimateGasLimit(ctx, request)
	if err != nil {
		return nil, err
	}

	gasFeeCap, gasTipCap, err := t.suggestGasFeeForTier(ctx, tier, previousTip, overrides)
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
	return tx, nil
}

// broadcastTx prepares, signs, and sends a transaction.
// When fixedNonce is nil a new nonce is allocated (first attempt);
// otherwise the supplied nonce is reused (replacement transaction).
func (t *transactionService) broadcastTx(ctx context.Context, request *TxRequest, fixedNonce *uint64, tier feeTier, previousTip *big.Int, overrides *RetryOverrides) (*types.Transaction, error) {
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
	tx, err := t.prepareTransactionForTier(ctx, request, nonce, tier, previousTip, overrides)
	if err != nil {
		return nil, err
	}

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

func (t *transactionService) deleteRetryStateAndPending(retryKey string, state TransactionRetryState, keepLast bool) {
	if retryKey == "" {
		return
	}
	_ = t.store.Delete(retryKey)
	for _, h := range state.AllTxHashes {
		_ = t.store.Delete(pendingTransactionKey(h))
	}
	if state.LastTxHash != (common.Hash{}) {
		if !keepLast {
			_ = t.store.Delete(pendingTransactionKey(state.LastTxHash))
		}
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

	tiers, err := t.tierRangeForRequest(ctx)
	if err != nil {
		return common.Hash{}, nil, err
	}

	t.logger.Debug("send with retry: started",
		"description", request.Description,
		"to", retryToForLog(request, &txState),
		"start_tier", tiers[0].String(),
		"end_tier", t.endTier.String(),
		"attempts_per_tier", t.attemptsPerTier,
		"retry_delay", t.txRetryDelay,
		"nonce_assigned", txState.NonceAssigned)

	var (
		previousTip    *big.Int
		terminateTxErr error
	)

	attempt := 1 // start attempts counting from 1 for metrics
	defer func() {
		if terminateTxErr != nil {
			t.logger.Error(terminateTxErr,
				"send with retry: finished with error",
				"attempt", attempt+1,
				"tx_hash", txState.LastTxHash,
				"nonce", txState.Nonce,
				"to", request.To.String(),
				"value", request.Value.String(),
				"description", request.Description)
		}

		monitorLast := errors.Is(terminateTxErr, ErrTxMaxPriceExceeded) || errors.Is(terminateTxErr, ErrAllAttemptsExhausted)
		t.deleteRetryStateAndPending(txRetryKey, txState, monitorLast)
		t.recordRetryComplete(attempt, terminateTxErr)
	}()

	retryDelay := t.txRetryDelay
	if overrides != nil && overrides.RetryDelay != nil {
		retryDelay = overrides.RetryDelay(t.txRetryDelay)
	}
	for _, tier := range tiers {
		for k := 0; k < t.attemptsPerTier; k++ {
			if txState.NonceAssigned {
				nonce = &txState.Nonce
			}

			replaced := true
			signedTx, err := t.broadcastTx(ctx, request, nonce, tier, previousTip, overrides)
			if err != nil {
				switch {
				case isNonceTooLow(err):
					// The nonce was consumed between our last receipt check and
					// this rebroadcast: our previously broadcast tx was most
					// likely mined. Try to read its receipt exactly once; if
					// it is absent, propagate the error and stop retrying.
					if txState.LastTxHash != (common.Hash{}) {
						if rec, recErr := t.backend.TransactionReceipt(ctx, txState.LastTxHash); recErr == nil && rec != nil {
							if rec.Status == 0 {
								terminateTxErr = ErrTransactionReverted
								return txState.LastTxHash, rec, terminateTxErr
							}
							return txState.LastTxHash, rec, nil
						}
					}
					terminateTxErr = err
					return common.Hash{}, nil, terminateTxErr
				case isReplacementUnderpriced(err):
					// Couldn't replace transaction, keep watching latest one
					replaced = false
					t.logger.Warning("transaction retry broadcast underpriced, keep watching pending tx",
						"attempt", attempt, "tier", tier.String(), "pending_tx", txState.LastTxHash,
						"error", err, "to", retryToForLog(request, &txState))
				case isNonRetryable(err):
					terminateTxErr = err
					return common.Hash{}, nil, terminateTxErr
				default:
					t.logger.Warning("transaction retry broadcast failed, will retry",
						"attempt", attempt, "tier", tier.String(), "error", err, "to", retryToForLog(request, &txState))
				}
			}

			if replaced {
				// update states only in case if previous tx was replaced - otherwise keep watching latest known tx hash
				if terminateTxErr = t.updateStates(signedTx, &txState); terminateTxErr != nil {
					return common.Hash{}, nil, terminateTxErr
				}

				if signedTx != nil {
					previousTip = signedTx.GasTipCap()
				}
			}

			if txState.NonceAssigned {
				txRetryKey = retryStateKey(txState.Nonce)
			}

			if txState.LastTxHash == (common.Hash{}) {
				t.logger.Debug("send with retry: no tx hash after broadcast failure, waiting before next attempt",
					"attempt", attempt, "nonce", txState.Nonce, "retry_delay", retryDelay)
				select {
				case <-ctx.Done():
					err := ctx.Err()
					terminateTxErr = err
					return common.Hash{}, nil, err
				case <-time.After(retryDelay):
					attempt++
					continue
				}
			}

			waitCtx, cancel := context.WithTimeout(ctx, retryDelay)
			rec, waitErr := t.WaitForReceipt(waitCtx, txState.LastTxHash)
			cancel()

			if waitErr == nil {
				t.logger.Debug("send with retry: receipt received",
					"tx_hash", txState.LastTxHash,
					"status", rec.Status,
					"gas_used", rec.GasUsed,
					"block_number", rec.BlockNumber,
					"nonce", txState.Nonce,
					"description", request.Description)

				if rec.Status == 0 {
					terminateTxErr = ErrTransactionReverted
					return txState.LastTxHash, rec, terminateTxErr
				}
				return txState.LastTxHash, rec, nil
			} else if isNonRetryable(waitErr) {
				terminateTxErr = waitErr
				return common.Hash{}, nil, terminateTxErr
			}

			t.logger.Debug("send with retry: receipt not received, will escalate",
				"attempt", attempt,
				"tier", tier.String(),
				"tx_hash", txState.LastTxHash,
				"nonce", txState.Nonce,
				"wait_error", waitErr,
				"description", request.Description)
			attempt++
		}
	}

	terminateTxErr = ErrAllAttemptsExhausted
	return txState.LastTxHash, nil, terminateTxErr
}

func (t *transactionService) updateStates(signedTx *types.Transaction, txState *TransactionRetryState) error {
	if txState.LastTxHash != (common.Hash{}) {
		txState.AllTxHashes = append(txState.AllTxHashes, txState.LastTxHash)
		// remove latest known hash from pending hashes since it is not actual anymore
		_ = t.store.Delete(pendingTransactionKey(txState.LastTxHash))
	}

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

		if !txState.NonceAssigned {
			// update retry-state in memory (do it only once)
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

	t.logger.Debug("resume retry: scanning persisted retry states",
		"count", len(keys),
		"confirmed_nonce", confirmed)

	for i := range keys {
		key := keys[i]
		state := states[i]

		if confirmed > state.Nonce {
			t.logger.Debug("resume retry: skipping already confirmed transaction",
				"nonce", state.Nonce,
				"confirmed_nonce", confirmed,
				"description", state.Description)
			t.deleteRetryStateAndPending(key, state, false)
			continue
		}

		t.logger.Debug("resume retry: resuming persisted retry",
			"nonce", state.Nonce,
			"last_tx_hash", state.LastTxHash,
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
