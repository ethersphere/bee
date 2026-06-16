// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// SetCongestion sets the fraction of block gas used by synthetic background traffic (0.0–1.0).
func (s *SimChain) SetCongestion(ratio float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	s.congestion = ratio
}

// SetMinMempoolTip sets the minimum priority fee accepted by the mempool.
func (s *SimChain) SetMinMempoolTip(tip *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.minMempoolTip = new(big.Int).Set(tip)
}

// SetBaseFee overrides the current base fee immediately.
func (s *SimChain) SetBaseFee(fee *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseFee = new(big.Int).Set(fee)
	if block := s.latestBlock(); block != nil {
		block.baseFee = new(big.Int).Set(fee)
	}
}

// SetBackgroundTipMean sets the mean of synthetic background tips used for fee history.
func (s *SimChain) SetBackgroundTipMean(tip *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backgroundTipMean = new(big.Int).Set(tip)
}

// SetBackgroundTipStdDev sets the standard deviation of synthetic background tips.
func (s *SimChain) SetBackgroundTipStdDev(stdDev *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backgroundTipStdDev = new(big.Int).Set(stdDev)
}

// SetBalance sets an account balance used for mempool validation and cost deduction.
func (s *SimChain) SetBalance(addr common.Address, bal *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances[addr] = new(big.Int).Set(bal)
}

// SetNonce sets the confirmed on-chain nonce for an account.
func (s *SimChain) SetNonce(addr common.Address, nonce uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordNonce(addr, s.blockNum, nonce)
}

// SetEstimateGas overrides the value returned by EstimateGas.
func (s *SimChain) SetEstimateGas(gas uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.estimateGas = gas
}

// SetRevertAddress marks contract addresses whose transactions should produce status=0 receipts.
func (s *SimChain) SetRevertAddress(addr common.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revertAddresses[addr] = struct{}{}
}

// InjectError makes the next count calls to method return err.
func (s *SimChain) InjectError(method string, err error, count int) {
	if count <= 0 || err == nil {
		return
	}
	s.errMu.Lock()
	defer s.errMu.Unlock()
	s.errInjections[method] = append(s.errInjections[method], errorInjection{
		err:   err,
		count: count,
	})
}

// MempoolSize returns the number of transactions currently in the mempool.
func (s *SimChain) MempoolSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pool.size()
}

// MempoolTxs returns mempool transactions for a sender.
func (s *SimChain) MempoolTxs(sender common.Address) []*types.Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var txs []*types.Transaction
	senderMap, ok := s.pool.bySender[sender]
	if !ok {
		return txs
	}
	for _, entry := range senderMap {
		txs = append(txs, entry.tx)
	}
	return txs
}

// BlockCount returns the latest block number.
func (s *SimChain) BlockCount() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockNum
}

// CurrentBaseFee returns the current base fee.
func (s *SimChain) CurrentBaseFee() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return new(big.Int).Set(s.baseFee)
}

// Signer returns the chain signer used to recover senders from transactions.
func (s *SimChain) Signer() types.Signer {
	return s.signer
}
