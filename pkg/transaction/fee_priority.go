// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"errors"
	"fmt"
	"strings"
)

// Fee priority selects which priority-fee percentile from eth_feeHistory to use.
// The zero value means the priority is unset.
const (
	FeePriorityLow = iota + 1
	FeePriorityMarket
	FeePriorityAggressive
)

var ErrUnknownFeePriority = errors.New("unknown fee priority")

// ValidateFeePriority returns ErrUnknownFeePriority for any tier other than low, market, or aggressive.
func ValidateFeePriority(tier int) error {
	switch tier {
	case FeePriorityLow, FeePriorityMarket, FeePriorityAggressive:
		return nil
	default:
		return fmt.Errorf("%w %d", ErrUnknownFeePriority, tier)
	}
}

// FeePriorityName returns the priority name. The zero value returns an empty string.
func FeePriorityName(tier int) string {
	switch tier {
	case FeePriorityLow:
		return "low"
	case FeePriorityMarket:
		return "market"
	case FeePriorityAggressive:
		return "aggressive"
	case 0:
		return ""
	default:
		return "unknown"
	}
}

// ParseFeePriority validates a fee priority name (for API headers and config).
// An empty name returns 0, which means the priority is unset.
func ParseFeePriority(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return 0, nil
	case "low":
		return FeePriorityLow, nil
	case "market":
		return FeePriorityMarket, nil
	case "aggressive":
		return FeePriorityAggressive, nil
	default:
		return 0, fmt.Errorf("%w %q (valid: low, market, aggressive)", ErrUnknownFeePriority, s)
	}
}
