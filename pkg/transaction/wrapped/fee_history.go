// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum"
)

func suggestedFeesFromFeeHistoryResult(fh *ethereum.FeeHistory, minimumTip int64) (low, market, aggressive *big.Int, err error) {
	if fh == nil {
		return nil, nil, nil, errors.New("fee history: empty response")
	}
	if len(fh.BaseFee) == 0 {
		return nil, nil, nil, errors.New("fee history: no base fees")
	}
	baseFee := fh.BaseFee[len(fh.BaseFee)-1]
	if baseFee == nil {
		return nil, nil, nil, ErrEIP1559NotSupported
	}

	minTip := big.NewInt(minimumTip)
	low = feeTierSum(baseFee, medianPriorityTipAtPercentileIndex(fh.Reward, 0), minTip)
	market = feeTierSum(baseFee, medianPriorityTipAtPercentileIndex(fh.Reward, 1), minTip)
	aggressive = feeTierSum(baseFee, medianPriorityTipAtPercentileIndex(fh.Reward, 2), minTip)
	return low, market, aggressive, nil
}

func feeTierSum(baseFee, percentileTip, minTip *big.Int) *big.Int {
	tip := new(big.Int).Set(percentileTip)
	if tip.Cmp(minTip) < 0 {
		tip.Set(minTip)
	}
	out := new(big.Int).Add(new(big.Int).Set(baseFee), tip)
	return out
}

func medianPriorityTipAtPercentileIndex(reward [][]*big.Int, idx int) *big.Int {
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
		return big.NewInt(0)
	}
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].Cmp(vals[j]) < 0
	})
	mid := len(vals) / 2
	if len(vals)%2 == 0 {
		sum := new(big.Int).Add(vals[mid-1], vals[mid])
		return sum.Div(sum, big.NewInt(2))
	}
	return vals[mid]
}
