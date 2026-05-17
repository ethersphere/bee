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

// RetryState is persisted so SendWithRetry can resume after a node restart.
type RetryState struct {
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

func mulDivPercent(x *big.Int, num, den int64) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(new(big.Int).Set(x), big.NewInt(num)), big.NewInt(den))
}

// escalateGasTip returns tip * (100+increasePct)/100 — a single escalation step.
func escalateGasTip(tip *big.Int, increasePct int) *big.Int {
	if tip == nil {
		return nil
	}
	return mulDivPercent(tip, int64(100+increasePct), 100)
}

func (t *transactionService) dynamicGasFeeCap(ctx context.Context, gasTipCap *big.Int) (gasFeeCap *big.Int, err error) {
	header, err := t.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	if header == nil || header.BaseFee == nil {
		return nil, fmt.Errorf("latest block header or base fee unavailable")
	}
	gasFeeCap = new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	gasFeeCap.Add(gasFeeCap, gasTipCap)
	return gasFeeCap, nil
}

func (t *transactionService) prepareTransactionWithRetry(ctx context.Context, request *TxRequest, nonce uint64, gasTipCap *big.Int) (*types.Transaction, error) {
	gasLimit, err := t.estimateGasLimit(ctx, request)
	if err != nil {
		return nil, err
	}

	var gasFeeCap *big.Int

	if gasTipCap == nil || gasTipCap.Sign() == 0 {
		fh, err := t.backend.GetFeeAndTipsFromFeeHistory(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("fee history: %w", err)
		}
		if fh == nil || fh.LatestBaseFee == nil {
			return nil, errors.New("fee history: missing base fee")
		}
		gasTipCap = fh.LowTip
		gasFeeCap, err = t.dynamicGasFeeCap(ctx, gasTipCap)
		if err != nil {
			return nil, err
		}
	} else {
		escalated := escalateGasTip(gasTipCap, t.txRetryGasIncreasePercent)
		gasFeeCap, err = t.dynamicGasFeeCap(ctx, escalated)
		if err != nil {
			return nil, err
		}
		if t.maxTxPrice == nil || gasFeeCap.Cmp(t.maxTxPrice) <= 0 {
			gasTipCap = escalated
		} else {
			// Escalated tip too expensive — fall back to current tip without escalation.
			gasFeeCap = new(big.Int).Sub(gasFeeCap, escalated)
			gasFeeCap.Add(gasFeeCap, gasTipCap)
		}
	}

	if t.maxTxPrice != nil && gasFeeCap.Cmp(t.maxTxPrice) > 0 {
		return nil, fmt.Errorf("%w: max_fee_per_gas %s exceeds limit %s", ErrTxMaxPriceExceeded, gasFeeCap, t.maxTxPrice)
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

// broadcastTxWithRetry prepares, signs, and sends a transaction.
// When fixedNonce is nil a new nonce is allocated (first attempt);
// otherwise the supplied nonce is reused (replacement transaction).
func (t *transactionService) broadcastTxWithRetry(ctx context.Context, request *TxRequest, fixedNonce *uint64, gasTipCap *big.Int, attempt int) (*types.Transaction, error) {
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
	tx, err := t.prepareTransactionWithRetry(ctx, request, nonce, gasTipCap)
	if err != nil {
		return nil, err
	}

	signedTx, err := t.signer.SignTx(tx, t.chainID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSignTransaction, err)
	}

	t.logger.Info("transaction retry broadcast",
		"attempt", attempt,
		"nonce", nonce,
		"gas_fee_cap", tx.GasFeeCap(),
		"gas_tip_cap", tx.GasTipCap(),
		"tx", signedTx.Hash(),
		"timestamp", time.Now().Unix(),
	)

	err = t.backend.SendTransaction(ctx, signedTx)
	return signedTx, err
}

func (t *transactionService) deleteRetryStateAndPending(retryKey string, state RetryState) {
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

// SendWithRetry sends an EIP-1559 transaction using one eth_feeHistory snapshot for the initial tip,
// then increases gas tip by gas_increase_percent after each unsuccessful wait, up to max_retries.
func (t *transactionService) SendWithRetry(ctx context.Context, request *TxRequest) (txHash common.Hash, receipt *types.Receipt, err error) {
	if request.GasPrice != nil {
		return common.Hash{}, nil, errors.New("send txs with retry requires automatic gas pricing") // TODO fallback to send
	}
	return t.retry(ctx, "", request)
}

func (t *transactionService) retry(ctx context.Context, txRetryKey string, request *TxRequest) (common.Hash, *types.Receipt, error) {
	var (
		txState RetryState
		nonce   *uint64
		gasTip  *big.Int
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

	if txState.PreviousTip != nil {
		gasTip = new(big.Int).Set(txState.PreviousTip)
	}

	for attempt := txState.NextAttempt; attempt < t.txMaxRetries; attempt++ {
		if txState.NonceAssigned {
			nonce = &txState.Nonce
		}

		signedTx, err := t.broadcastTxWithRetry(ctx, request, nonce, gasTip, attempt)
		if err != nil {
			if isErrCritical(err) {
				t.logger.Warning("transaction broadcast failed with critical error, stop retry", "attempt", attempt, "error", err)
				t.deleteRetryStateAndPending(txRetryKey, txState)
				return common.Hash{}, nil, err
			}
			t.logger.Warning("transaction retry broadcast failed, will retry", "attempt", attempt, "error", err)
		}

		if err := t.updateStates(signedTx, &txState); err != nil {
			t.deleteRetryStateAndPending(txRetryKey, txState)
			return common.Hash{}, nil, err
		}

		if txState.NonceAssigned {
			txRetryKey = retryStateKey(txState.Nonce)
		}

		if txState.LastTxHash == (common.Hash{}) {
			select {
			case <-ctx.Done():
				return common.Hash{}, nil, ctx.Err()
			case <-time.After(t.txRetryDelay):
				continue
			}
		}

		waitCtx, cancel := context.WithTimeout(ctx, t.txRetryDelay)
		rec, waitErr := t.WaitForReceipt(waitCtx, txState.LastTxHash)
		cancel()

		if waitErr == nil {
			t.deleteRetryStateAndPending(txRetryKey, txState)
			if rec.Status == 0 {
				return txState.LastTxHash, rec, ErrTransactionReverted
			}
			return txState.LastTxHash, rec, nil
		} else if isErrCritical(waitErr) {
			t.deleteRetryStateAndPending(txRetryKey, txState)
			return common.Hash{}, nil, waitErr
		}
	}

	t.deleteRetryStateAndPending(txRetryKey, txState)
	return txState.LastTxHash, nil, fmt.Errorf("transaction failed after %d attempts (nonce=%d, description=%s)", t.txMaxRetries, txState.Nonce, txState.Description)
}

func (t *transactionService) updateStates(signedTx *types.Transaction, txState *RetryState) error {
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
		var s RetryState
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
	var states []RetryState
	err := t.store.Iterate(retryStatePrefix, func(key, val []byte) (stop bool, err error) {
		var s RetryState
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
		// TODO logging, but keep going
	}

	for i := range keys {
		key := keys[i]
		state := states[i]

		if confirmed > state.Nonce {
			t.deleteRetryStateAndPending(key, state)
			continue
		}

		sk := key
		st := state
		t.wg.Go(func() {
			if _, _, err := t.retry(t.ctx, sk, nil); err != nil {
				t.logger.Error(err, "resumed transaction retry aborted", "nonce", st.Nonce, "description", st.Description)
			}
		})
	}
	return nil
}
