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
		reward = append(reward, rewardsAtPercentiles(block.rewards, rewardPercentiles))
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

func rewardsAtPercentiles(samples []rewardSample, percentiles []float64) []*big.Int {
	if len(percentiles) == 0 {
		percentiles = []float64{10, 50, 90}
	}

	vals := make([]rewardSample, 0, len(samples))
	var totalGas uint64
	for _, sample := range samples {
		if sample.tip == nil || sample.gasUsed == 0 {
			continue
		}
		vals = append(vals, rewardSample{
			tip:     new(big.Int).Set(sample.tip),
			gasUsed: sample.gasUsed,
		})
		totalGas += sample.gasUsed
	}
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].tip.Cmp(vals[j].tip) < 0
	})

	out := make([]*big.Int, len(percentiles))
	for i, p := range percentiles {
		out[i] = gasWeightedPercentile(vals, totalGas, p)
	}
	return out
}

func gasWeightedPercentile(samples []rewardSample, totalGas uint64, percentile float64) *big.Int {
	if len(samples) == 0 || totalGas == 0 {
		return big.NewInt(0)
	}
	if percentile <= 0 {
		return new(big.Int).Set(samples[0].tip)
	}
	if percentile >= 100 {
		return new(big.Int).Set(samples[len(samples)-1].tip)
	}

	target := float64(totalGas) * percentile / 100
	var accumulated uint64
	for _, sample := range samples {
		accumulated += sample.gasUsed
		if float64(accumulated) >= target {
			return new(big.Int).Set(sample.tip)
		}
	}
	return new(big.Int).Set(samples[len(samples)-1].tip)
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
