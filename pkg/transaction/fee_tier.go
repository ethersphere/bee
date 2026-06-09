// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"fmt"
	"math/big"
	"strings"
)

// feeTier selects which priority-fee percentile from eth_feeHistory to use.
type feeTier int

const (
	feeTierLow feeTier = iota + 1
	feeTierMarket
	feeTierAggressive
)

func (ft feeTier) String() string {
	switch ft {
	case feeTierLow:
		return "low"
	case feeTierMarket:
		return "market"
	case feeTierAggressive:
		return "aggressive"
	default:
		return "unknown"
	}
}

// ParseFeeTier validates a fee priority tier name (for API headers and config).
func ParseFeeTier(s string) (feeTier, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return feeTierLow, nil
	case "market", "":
		return feeTierMarket, nil
	case "aggressive":
		return feeTierAggressive, nil
	default:
		return 0, fmt.Errorf("unknown fee tier %q (valid: low, market, aggressive)", s)
	}
}

// tierRange returns the ordered slice of tiers from start to end inclusive.
func tierRange(start, end feeTier) []feeTier {
	var tiers []feeTier
	for t := start; t <= end; t++ {
		tiers = append(tiers, t)
	}
	return tiers
}

// tierTip selects the appropriate tip from a FeeHistorySuggestedFeeAndTips for the given tier.
func tierTip(tier feeTier, fh *FeeHistorySuggestedFeeAndTips) *big.Int {
	switch tier {
	case feeTierLow:
		return fh.LowTip
	case feeTierMarket:
		return fh.MarketTip
	case feeTierAggressive:
		return fh.AggressiveTip
	default:
		return fh.MarketTip
	}
}
