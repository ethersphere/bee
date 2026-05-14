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
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const retryStatePrefix = "transaction_retry_"

// RetryState is persisted so SendWithRetry can resume after a node restart.
type RetryState struct {
	Nonce       uint64          `json:"nonce"`
	NextAttempt int             `json:"next_attempt"`
	LastTxHash  common.Hash     `json:"last_tx_hash"`
	AllTxHashes []common.Hash   `json:"all_tx_hashes"`
	GasLimit    uint64          `json:"gas_limit"`
	To          *common.Address `json:"to,omitempty"`
	Data        []byte          `json:"data,omitempty"`
	Value       *big.Int        `json:"value,omitempty"`
	Description string          `json:"description,omitempty"`

	// InitialTip is the starting maxPriorityFeePerGas (from fee history); each retry multiplies by (100+GasIncreasePercent)/100.
	InitialTip *big.Int `json:"initial_tip,omitempty"`
}

func retryStateKey(nonce uint64) string {
	return fmt.Sprintf("%s%020d", retryStatePrefix, nonce)
}

func mulDivPercent(x *big.Int, num, den int64) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(new(big.Int).Set(x), big.NewInt(num)), big.NewInt(den))
}

// escalateGasTip returns initialTip * ((100+increasePct)/100)^attempt.
func escalateGasTip(initial *big.Int, attempt, increasePct int) *big.Int {
	if attempt != 0 {
		increasePct = increasePct * attempt
	}
	tip := new(big.Int).Set(initial)
	tip = mulDivPercent(tip, int64(100+increasePct), 100)
	return tip
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

	if gasTipCap == nil || gasTipCap.Sign() == 0 {
		fh, err := t.backend.GetFeeAndTipsFromFeeHistory(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("fee history: %w", err)
		}

		if fh == nil || fh.LatestBaseFee == nil {
			return nil, errors.New("fee history: missing base fee")
		}
		gasTipCap = fh.LowTip
	}
	gasFeeCap, err := t.dynamicGasFeeCap(ctx, gasTipCap)
	if err != nil {
		return nil, err
	}
	if t.maxTxPrice != nil && gasFeeCap.Cmp(t.maxTxPrice) > 0 {
		return nil, fmt.Errorf("%w: max_fee_per_gas %s exceeds limit %s", ErrTxRetryMaxPriceExceeded, gasFeeCap, t.maxTxPrice)
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
		n, err := t.nextNonce(ctx)
		t.lock.Unlock()
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
		return nil, err
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
	_ = t.store.Delete(retryKey)
	for _, h := range state.AllTxHashes {
		_ = t.store.Delete(pendingTransactionKey(h))
	}
	if state.LastTxHash != (common.Hash{}) {
		_ = t.store.Delete(pendingTransactionKey(state.LastTxHash))
	}
}

func (t *transactionService) saveTxInState(signedTx *types.Transaction, saveForRetry bool) error {
	txHash := signedTx.Hash()
	now := time.Now().Unix()
	if saveForRetry {
		state := &RetryState{
			Nonce:       signedTx.Nonce(),
			NextAttempt: 1,
			LastTxHash:  signedTx.Hash(),
			GasLimit:    signedTx.Gas(),
			To:          signedTx.To(),
			Data:        signedTx.Data(),
			Value:       signedTx.Value(),
			InitialTip:  signedTx.GasTipCap(),
		}

		retryKey := retryStateKey(state.Nonce)
		if err := t.store.Put(retryKey, state); err != nil {
			return err
		}
	}

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

	return t.store.Put(pendingTransactionKey(txHash), struct{}{})
}

// SendWithRetry sends an EIP-1559 transaction using one eth_feeHistory snapshot for the initial tip,
// then increases maxPriorityFeePerGas by GasIncreasePercent after each unsuccessful wait, up to MaxRetries.
func (t *transactionService) SendWithRetry(ctx context.Context, request *TxRequest) (txHash common.Hash, receipt *types.Receipt, err error) {
	if request.GasPrice != nil {
		return common.Hash{}, nil, errors.New("SendWithRetry requires automatic gas pricing (GasPrice must be nil)")
	}

	signedTx, err := t.broadcastTxWithRetry(ctx, request, nil, nil, 0)
	if err != nil {
		return common.Hash{}, nil, err
	}

	if err := t.saveTxInState(signedTx, true); err != nil {
		return common.Hash{}, nil, err
	}

	txRetryKey := retryStateKey(signedTx.Nonce())
	return t.retry(ctx, txRetryKey)
}

func (t *transactionService) retry(ctx context.Context, txRetryKey string) (common.Hash, *types.Receipt, error) {
	var txState RetryState
	if err := t.store.Get(txRetryKey, &txState); err != nil {
		return common.Hash{}, nil, err
	}

	for attempt := txState.NextAttempt; attempt <= t.txMaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return common.Hash{}, nil, ctx.Err()
		default:
		}

		// Wait for the last broadcast transaction to confirm.
		if txState.LastTxHash != (common.Hash{}) {
			waitCtx, cancel := context.WithTimeout(ctx, t.txRetryDelay)
			rec, waitErr := t.WaitForReceipt(waitCtx, txState.LastTxHash)
			cancel()
			if waitErr == nil {
				t.deleteRetryStateAndPending(txRetryKey, txState)
				if rec.Status == 0 {
					return txState.LastTxHash, rec, ErrTransactionReverted
				}
				return txState.LastTxHash, rec, nil
			}
		}

		// Escalate tip and rebroadcast with the SAME nonce (replacement tx).
		escalatedTip := escalateGasTip(txState.InitialTip, attempt, t.txRetryGasIncreasePercent)
		nonce := txState.Nonce
		request := &TxRequest{
			To:          txState.To,
			Data:        txState.Data,
			GasLimit:    txState.GasLimit,
			Value:       txState.Value,
			Description: txState.Description,
		}

		signedTx, err := t.broadcastTxWithRetry(ctx, request, &nonce, escalatedTip, attempt)
		if isErrCritical(err) {
			t.deleteRetryStateAndPending(txRetryKey, txState)
			return common.Hash{}, nil, err
		}
		if err != nil {
			t.logger.Warning("transaction retry broadcast failed, will retry", "attempt", attempt, "error", err)
			continue
		}

		// Remove old pending key, register the new tx hash.
		if txState.LastTxHash != (common.Hash{}) {
			_ = t.store.Delete(pendingTransactionKey(txState.LastTxHash))
		}

		if err := t.saveTxInState(signedTx, false); err != nil {
			return common.Hash{}, nil, err
		}

		txState.AllTxHashes = append(txState.AllTxHashes, txState.LastTxHash)
		txState.NextAttempt = attempt + 1
		txState.LastTxHash = signedTx.Hash()

		if err := t.store.Put(txRetryKey, txState); err != nil {
			return common.Hash{}, nil, err
		}
	}

	t.deleteRetryStateAndPending(txRetryKey, txState)
	return txState.LastTxHash, nil, fmt.Errorf(
		"transaction failed after %d attempts due to network congestion (nonce=%d, description=%s). Please try again",
		t.txMaxRetries, txState.Nonce, txState.Description,
	)
}

func isErrCritical(err error) bool {
	// TODO implement
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

func (t *transactionService) resumeRetryStates() error {
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

	for i := range keys {
		key := keys[i]
		state := states[i]

		if done, err := t.retryStateAlreadyDone(&state); err != nil {
			return err
		} else if done {
			t.deleteRetryStateAndPending(key, state)
			continue
		}

		sk := key
		st := state
		t.wg.Go(func() {
			if _, _, err := t.retry(t.ctx, sk); err != nil {
				t.logger.Error(err, "resumed transaction retry aborted", "nonce", st.Nonce, "description", st.Description)
			}
		})
	}
	return nil
}

// TODO simplify -> may be refuse
func (t *transactionService) retryStateAlreadyDone(state *RetryState) (bool, error) {
	if state.LastTxHash != (common.Hash{}) {
		rec, err := t.backend.TransactionReceipt(t.ctx, state.LastTxHash)
		if err == nil && rec != nil {
			return true, nil
		}
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return false, err
		}
	}

	for _, h := range state.AllTxHashes {
		rec, err := t.backend.TransactionReceipt(t.ctx, h)
		if err == nil && rec != nil {
			return true, nil
		}
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return false, err
		}
	}

	confirmed, err := t.backend.NonceAt(t.ctx, t.sender, nil)
	if err != nil {
		return false, err
	}
	if confirmed > state.Nonce {
		return true, nil
	}
	return false, nil
}
