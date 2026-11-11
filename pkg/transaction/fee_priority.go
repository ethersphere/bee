// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"fmt"
	"math/big"
	"strings"
)

// feePriority selects which priority-fee percentile from eth_feeHistory to use.
type feePriority int

const (
	FeePriorityLow feePriority = iota + 1
	FeePriorityMarket
	FeePriorityAggressive
)

func (ft feePriority) String() string {
	switch ft {
	case FeePriorityLow:
		return "low"
	case FeePriorityMarket:
		return "market"
	case FeePriorityAggressive:
		return "aggressive"
	default:
		return "unknown"
	}
}

// OptionalFeePriority returns nil when priority is unset. Send treats nil as the default suggestion.
func OptionalFeePriority(tier string) *string {
	if strings.TrimSpace(tier) == "" {
		return nil
	}
	return &tier
}

// ParseFeePriority validates a fee priority name (for API headers and config).
func ParseFeePriority(s string) (feePriority, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return FeePriorityLow, nil
	case "market", "":
		return FeePriorityMarket, nil
	case "aggressive":
		return FeePriorityAggressive, nil
	default:
		return 0, fmt.Errorf("unknown fee priority %q (valid: low, market, aggressive)", s)
	}
}

// FeePriorityAboveMax reports whether tier is a higher priority than max.
func FeePriorityAboveMax(tier, max string) (bool, error) {
	got, err := ParseFeePriority(tier)
	if err != nil {
		return false, err
	}
	limit, err := ParseFeePriority(max)
	if err != nil {
		return false, err
	}
	return got > limit, nil
}

// TipForPriority selects the priority fee for a fee priority from a fee-history result.
func TipForPriority(tier feePriority, fh *FeeHistorySuggestedFeeAndTips) *big.Int {
	switch tier {
	case FeePriorityLow:
		return fh.LowTip
	case FeePriorityAggressive:
		return fh.AggressiveTip
	default:
		return fh.MarketTip
	}
}
