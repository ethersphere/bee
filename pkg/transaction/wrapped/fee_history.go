// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"errors"
	"fmt"
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
	low, err = medianPriorityTipAtPercentileIndex(fh.Reward, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	market, err = medianPriorityTipAtPercentileIndex(fh.Reward, 1)
	if err != nil {
		return nil, nil, nil, err
	}
	aggressive, err = medianPriorityTipAtPercentileIndex(fh.Reward, 2)
	if err != nil {
		return nil, nil, nil, err
	}
	return low, market, aggressive, nil
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
