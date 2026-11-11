// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"fmt"
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
