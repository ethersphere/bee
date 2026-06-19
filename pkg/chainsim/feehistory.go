// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

func (s *SimChain) feeHistoryLocked(lastBlock *big.Int, blockCount int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	if blockCount <= 0 {
		blockCount = 1
	}

	end := s.blockNum
	if lastBlock != nil {
		end = lastBlock.Uint64()
	}
	if end > s.blockNum {
		end = s.blockNum
	}

	start := end
	if int(end) >= blockCount {
		start = end - uint64(blockCount) + 1
	}

	if start > end {
		return nil, errors.New("fee history: no blocks available")
	}

	count := int(end - start + 1)
	baseFees := make([]*big.Int, 0, count+1)
	gasUsedRatio := make([]float64, 0, count)
	reward := make([][]*big.Int, 0, count)

	for num := start; num <= end; num++ {
		block, ok := s.blockByNumber(num)
		if !ok {
			continue
		}
		baseFees = append(baseFees, new(big.Int).Set(block.baseFee))
		if block.gasLimit == 0 {
			gasUsedRatio = append(gasUsedRatio, 0)
		} else {
			gasUsedRatio = append(gasUsedRatio, float64(block.gasUsed)/float64(block.gasLimit))
		}
		reward = append(reward, tipsAtPercentiles(block.tips, rewardPercentiles, s.minMempoolTip))
	}

	if len(baseFees) == 0 {
		return nil, errors.New("fee history: no blocks available")
	}

	baseFees = append(baseFees, new(big.Int).Set(s.baseFee))

	return &ethereum.FeeHistory{
		OldestBlock:  new(big.Int).SetUint64(start),
		BaseFee:      baseFees,
		GasUsedRatio: gasUsedRatio,
		Reward:       reward,
	}, nil
}

func tipsAtPercentiles(tips []*big.Int, percentiles []float64, fallback *big.Int) []*big.Int {
	if len(percentiles) == 0 {
		percentiles = []float64{10, 50, 90}
	}

	vals := make([]*big.Int, 0, len(tips))
	for _, tip := range tips {
		if tip == nil {
			continue
		}
		vals = append(vals, new(big.Int).Set(tip))
	}
	if len(vals) == 0 {
		fb := big.NewInt(0)
		if fallback != nil && fallback.Sign() > 0 {
			fb = new(big.Int).Set(fallback)
		}
		vals = append(vals, fb)
	}

	sort.Slice(vals, func(i, j int) bool {
		return vals[i].Cmp(vals[j]) < 0
	})

	out := make([]*big.Int, len(percentiles))
	for i, p := range percentiles {
		out[i] = percentileBigInt(vals, p)
	}
	return out
}

func percentileBigInt(vals []*big.Int, p float64) *big.Int {
	if len(vals) == 0 {
		return big.NewInt(0)
	}
	if p <= 0 {
		return new(big.Int).Set(vals[0])
	}
	if p >= 100 {
		return new(big.Int).Set(vals[len(vals)-1])
	}

	rank := int(float64(len(vals)-1) * p / 100.0)
	if rank < 0 {
		rank = 0
	}
	if rank >= len(vals) {
		rank = len(vals) - 1
	}
	return new(big.Int).Set(vals[rank])
}

func suggestedFeesFromFeeHistory(fh *ethereum.FeeHistory) (*transaction.FeeHistorySuggestedFeeAndTips, error) {
	if fh == nil {
		return nil, errors.New("fee history: empty response")
	}
	if len(fh.BaseFee) == 0 {
		return nil, errors.New("fee history: no base fees")
	}

	low, err := medianPriorityTipAtPercentileIndex(fh.Reward, 0)
	if err != nil {
		return nil, err
	}
	market, err := medianPriorityTipAtPercentileIndex(fh.Reward, 1)
	if err != nil {
		return nil, err
	}
	aggressive, err := medianPriorityTipAtPercentileIndex(fh.Reward, 2)
	if err != nil {
		return nil, err
	}

	return &transaction.FeeHistorySuggestedFeeAndTips{
		LowTip:        low,
		MarketTip:     market,
		AggressiveTip: aggressive,
	}, nil
}

func medianPriorityTipAtPercentileIndex(reward [][]*big.Int, idx int) (*big.Int, error) {
	var vals []*big.Int
	for _, row := range reward {
		if idx >= len(row) {
			continue
		}
		if row[idx] == nil {
			continue
		}
		vals = append(vals, new(big.Int).Set(row[idx]))
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("fee history: no reward entries for percentile index %d", idx)
	}

	sort.Slice(vals, func(i, j int) bool {
		return vals[i].Cmp(vals[j]) < 0
	})

	mid := len(vals) / 2
	if len(vals)%2 == 0 {
		sum := new(big.Int).Add(vals[mid-1], vals[mid])
		return sum.Div(sum, big.NewInt(2)), nil
	}
	return vals[mid], nil
}
