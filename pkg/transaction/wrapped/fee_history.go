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

func suggestedFeesFromFeeHistoryResult(fh *ethereum.FeeHistory) (low, market, aggressive *big.Int, err error) {
	if fh == nil {
		return nil, nil, nil, errors.New("fee history: empty response")
	}
	if len(fh.BaseFee) == 0 {
		return nil, nil, nil, errors.New("fee history: no base fees")
	}
	low = medianPriorityTipAtPercentileIndex(fh.Reward, 0)
	market = medianPriorityTipAtPercentileIndex(fh.Reward, 1)
	aggressive = medianPriorityTipAtPercentileIndex(fh.Reward, 2)
	return low, market, aggressive, nil
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
