// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// MempoolTxInfo describes a transaction waiting in the mempool.
type MempoolTxInfo struct {
	Hash         common.Hash     `json:"hash"`
	Sender       common.Address  `json:"sender"`
	Nonce        uint64          `json:"nonce"`
	Gas          uint64          `json:"gas"`
	GasTipCap    string          `json:"gas_tip_cap"`
	GasFeeCap    string          `json:"gas_fee_cap"`
	Value        string          `json:"value"`
	To           *common.Address `json:"to,omitempty"`
	AddedAtBlock uint64          `json:"added_at_block"`
}

// SuggestedFeesInfo holds current fee suggestions derived from fee history.
type SuggestedFeesInfo struct {
	BaseFee       string `json:"base_fee"`
	MinMempoolTip string `json:"min_mempool_tip"`
	LowTip        string `json:"low_tip"`
	MarketTip     string `json:"market_tip"`
	AggressiveTip string `json:"aggressive_tip"`
	GasFeeCap     string `json:"gas_fee_cap"`
}

// DebugStatus is the current simulation state exposed by the debug endpoint.
type DebugStatus struct {
	BlockNumber    uint64            `json:"block_number"`
	BlockTimestamp uint64            `json:"block_timestamp"`
	MempoolSize    int               `json:"mempool_size"`
	Mempool        []MempoolTxInfo   `json:"mempool"`
	Fees           SuggestedFeesInfo `json:"fees"`
	Stats          Stats             `json:"stats"`
}

// DebugStatus returns a snapshot of the current simulation state for debugging.
func (s *SimChain) DebugStatus(ctx context.Context) (DebugStatus, error) {
	if err := s.checkClosed(); err != nil {
		return DebugStatus{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	status := DebugStatus{
		BlockNumber:    s.blockNum,
		BlockTimestamp: s.blockTs,
		MempoolSize:    s.pool.size(),
		Mempool:        s.mempoolInfoLocked(),
		Fees: SuggestedFeesInfo{
			BaseFee:       s.baseFee.String(),
			MinMempoolTip: s.minMempoolTip.String(),
		},
		Stats: s.stats.copy(),
	}

	tips, err := s.suggestedFeesLocked()
	if err != nil {
		return DebugStatus{}, err
	}
	status.Fees.LowTip = tips.LowTip.String()
	status.Fees.MarketTip = tips.MarketTip.String()
	status.Fees.AggressiveTip = tips.AggressiveTip.String()

	gasFeeCap := new(big.Int).Mul(s.baseFee, big.NewInt(2))
	gasFeeCap.Add(gasFeeCap, tips.MarketTip)
	status.Fees.GasFeeCap = gasFeeCap.String()

	return status, nil
}

func (s *SimChain) mempoolInfoLocked() []MempoolTxInfo {
	result := make([]MempoolTxInfo, 0, len(s.pool.entries))
	for _, entry := range s.pool.entries {
		info := MempoolTxInfo{
			Hash:         entry.tx.Hash(),
			Sender:       entry.sender,
			Nonce:        entry.tx.Nonce(),
			Gas:          entry.tx.Gas(),
			GasTipCap:    entry.tx.GasTipCap().String(),
			GasFeeCap:    entry.tx.GasFeeCap().String(),
			Value:        entry.tx.Value().String(),
			AddedAtBlock: entry.addedAt,
		}
		if to := entry.tx.To(); to != nil {
			info.To = to
		}
		result = append(result, info)
	}
	return result
}

func (s *SimChain) suggestedFeesLocked() (*transaction.FeeHistorySuggestedFeeAndTips, error) {
	fh, err := s.feeHistoryLocked(nil, s.cfg.FeeHistoryDepth, []float64{10, 50, 90})
	if err != nil {
		return nil, err
	}
	return suggestedFeesFromFeeHistory(fh)
}
