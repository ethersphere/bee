// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"math"
	"math/big"
	"math/rand"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// CommitBlock produces the next block, including eligible mempool transactions.
func (s *SimChain) CommitBlock() uint64 {
	s.mu.Lock()
	num := s.commitBlockLocked()
	hook := s.onBlockCommit
	s.mu.Unlock()
	if hook != nil {
		hook(num)
	}
	return num
}

// CommitEmptyBlock produces a block without including mempool transactions.
func (s *SimChain) CommitEmptyBlock() uint64 {
	s.mu.Lock()
	num := s.commitBlockLocked(true)
	hook := s.onBlockCommit
	s.mu.Unlock()
	if hook != nil {
		hook(num)
	}
	return num
}

func (s *SimChain) commitBlockLocked(skipInclusion ...bool) uint64 {
	include := len(skipInclusion) == 0 || !skipInclusion[0]

	nextNum := s.blockNum + 1
	delta := s.blockTimeDelta()
	nextTime := s.blockTs + uint64(delta.Seconds())
	if delta <= 0 {
		nextTime = s.blockTs + 1
	}

	block := &simBlock{
		number:   nextNum,
		time:     nextTime,
		baseFee:  new(big.Int).Set(s.baseFee),
		gasLimit: s.cfg.BlockGasLimit,
	}

	var includedGas uint64
	if include {
		includedGas = s.includeTransactions(block)
	}

	bgCongestion := s.congestion
	if s.cfg.CongestionStdDev > 0 {
		bgCongestion += s.rng.NormFloat64() * s.cfg.CongestionStdDev
		if bgCongestion < 0 {
			bgCongestion = 0
		}
		if bgCongestion > 1 {
			bgCongestion = 1
		}
	}
	backgroundGas := uint64(float64(s.cfg.BlockGasLimit) * bgCongestion)
	remaining := s.cfg.BlockGasLimit - includedGas
	if backgroundGas > remaining {
		backgroundGas = remaining
	}
	block.gasUsed = includedGas + backgroundGas
	block.tips = s.backgroundTips()

	s.baseFee = nextBaseFee(s.baseFee, block.gasUsed, s.cfg.BlockGasLimit)
	block.baseFee = new(big.Int).Set(s.baseFee)

	s.blockNum = nextNum
	s.blockTs = nextTime
	s.blocks = append(s.blocks, block)
	evicted := s.pool.evictExpired(s.blockNum)
	s.recordMempoolTTLExpirations(len(evicted))

	if len(s.blocks) > s.cfg.FeeHistoryDepth+1 {
		s.blocks = append([]*simBlock(nil), s.blocks[len(s.blocks)-s.cfg.FeeHistoryDepth-1:]...)
	}

	s.trimHistoryLocked()

	s.recordBlockProduced()
	s.logger.Info("new block",
		"number", nextNum,
		"timestamp", nextTime,
		"base_fee", block.baseFee,
		"gas_used", block.gasUsed,
		"gas_limit", block.gasLimit,
		"tx_count", len(block.txHashes),
		"mempool_size", s.pool.size(),
		"ttl_evicted", len(evicted),
	)

	return nextNum
}

func (s *SimChain) includeTransactions(block *simBlock) uint64 {
	availableGas := uint64(float64(s.cfg.BlockGasLimit) * (1 - s.congestion))
	if availableGas == 0 {
		return 0
	}

	eligible := s.pool.eligible(s.nonces, s.baseFee)
	refTip := s.referenceInclusionTip()
	var gasUsed uint64
	includedCount := 0

	for _, entry := range eligible {
		if s.cfg.MaxTxsPerBlock > 0 && includedCount >= s.cfg.MaxTxsPerBlock {
			break
		}

		if entry.tx.Nonce() != s.confirmedNonce(entry.sender) {
			continue
		}

		if !s.shouldIncludeTx(entry, refTip) {
			s.recordInclusionDeferred()
			tip := entry.effectiveTip(s.baseFee)
			s.logger.Info("transaction deferred",
				append(txLogFields(entry.tx, entry.sender),
					"block", block.number,
					"effective_tip", tip,
					"reference_tip", refTip,
					"inclusion_probability", inclusionProbability(tip, refTip, s.cfg.InclusionMinProbability),
				)...,
			)
			continue
		}

		txGas := entry.tx.Gas()
		actualGas := txGas
		if s.cfg.BaseGasUsed > 0 && s.cfg.BaseGasUsed < txGas {
			actualGas = s.cfg.BaseGasUsed
		}
		if gasUsed+txGas > availableGas {
			continue
		}

		gasUsed += actualGas
		includedCount++
		block.txHashes = append(block.txHashes, entry.tx.Hash())
		block.tips = append(block.tips, new(big.Int).Set(entry.effectiveTip(s.baseFee)))

		status := uint64(1)
		if entry.tx.To() != nil {
			if _, revert := s.revertAddresses[*entry.tx.To()]; revert {
				status = 0
			}
		}
		if status == 1 && s.cfg.RandomRevertRate > 0 && s.rng.Float64() < s.cfg.RandomRevertRate {
			status = 0
		}

		receipt := &types.Receipt{
			TxHash:      entry.tx.Hash(),
			Status:      status,
			GasUsed:     actualGas,
			BlockNumber: new(big.Int).SetUint64(block.number),
			BlockHash:   syntheticBlockHash(block.number),
		}

		s.receipts[entry.tx.Hash()] = &receiptRecord{
			receipt:    receipt,
			includedAt: block.number,
		}
		s.minedTxs[entry.tx.Hash()] = entry.tx
		s.minedOrder = append(s.minedOrder, minedRef{block: block.number, hash: entry.tx.Hash()})
		s.pool.remove(entry.tx.Hash())

		newNonce := entry.tx.Nonce() + 1
		s.recordNonce(entry.sender, block.number, newNonce)
		s.deductCost(entry, actualGas)

		reverted := status == 0
		s.recordTxExecuted(reverted)
		fields := append(txLogFields(entry.tx, entry.sender),
			"block", block.number,
			"status", status,
			"effective_gas_price", new(big.Int).Add(s.baseFee, entry.effectiveTip(s.baseFee)),
		)
		s.logger.Info("transaction executed", fields...)
	}

	return gasUsed
}

func (s *SimChain) deductCost(entry *poolEntry, gasUsed uint64) {
	balance := s.balanceOf(entry.sender)
	if balance.Sign() == 0 && s.balances[entry.sender] == nil {
		return
	}

	effectiveGasPrice := new(big.Int).Add(s.baseFee, entry.effectiveTip(s.baseFee))
	if effectiveGasPrice.Cmp(entry.tx.GasFeeCap()) > 0 {
		effectiveGasPrice.Set(entry.tx.GasFeeCap())
	}

	cost := new(big.Int).Mul(new(big.Int).SetUint64(gasUsed), effectiveGasPrice)
	cost.Add(cost, entry.tx.Value())
	balance.Sub(balance, cost)
	s.balances[entry.sender] = balance
}

func nextBaseFee(current *big.Int, gasUsed, gasLimit uint64) *big.Int {
	if gasLimit == 0 {
		return new(big.Int).Set(current)
	}

	gasTarget := gasLimit / 2
	next := new(big.Int).Set(current)

	if gasUsed > gasTarget {
		increase := new(big.Int).Mul(current, new(big.Int).SetUint64(gasUsed-gasTarget))
		increase.Div(increase, new(big.Int).SetUint64(gasTarget))
		increase.Div(increase, big.NewInt(8))
		if increase.Sign() == 0 {
			increase.SetInt64(1)
		}
		next.Add(next, increase)
		return next
	}

	if gasUsed < gasTarget {
		decrease := new(big.Int).Mul(current, new(big.Int).SetUint64(gasTarget-gasUsed))
		decrease.Div(decrease, new(big.Int).SetUint64(gasTarget))
		decrease.Div(decrease, big.NewInt(8))
		next.Sub(next, decrease)
		if next.Sign() <= 0 {
			next.SetInt64(1)
		}
	}
	return next
}

func syntheticBlockHash(number uint64) common.Hash {
	var hash common.Hash
	hash[31] = byte(number)
	hash[30] = byte(number >> 8)
	hash[29] = byte(number >> 16)
	hash[28] = byte(number >> 24)
	return hash
}

func (s *SimChain) backgroundTips() []*big.Int {
	const backgroundTxCount = 20
	tips := make([]*big.Int, 0, backgroundTxCount)

	for i := 0; i < backgroundTxCount; i++ {
		tip := sampleTip(s.rng, s.backgroundTipMean, s.backgroundTipStdDev)
		if tip.Cmp(s.minMempoolTip) < 0 {
			tip.Set(s.minMempoolTip)
		}
		tips = append(tips, tip)
	}
	return tips
}

func sampleTip(rng *rand.Rand, mean, stdDev *big.Int) *big.Int {
	if stdDev.Sign() == 0 {
		return new(big.Int).Set(mean)
	}

	u1 := rng.Float64()
	if u1 == 0 {
		u1 = 1e-10
	}
	u2 := rng.Float64()
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)

	sample := new(big.Int).Set(stdDev)
	sample.Mul(sample, big.NewInt(int64(z*1000)))
	sample.Div(sample, big.NewInt(1000))
	sample.Add(sample, mean)
	if sample.Sign() < 0 {
		sample.SetInt64(0)
	}
	return sample
}
