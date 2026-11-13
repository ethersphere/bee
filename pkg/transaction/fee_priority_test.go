// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/transaction"
)

func TestParseFeePriority(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want int
		name string
		err  bool
	}{
		{"low", transaction.FeePriorityLow, "low", false},
		{" Market ", transaction.FeePriorityMarket, "market", false},
		{"AGGRESSIVE", transaction.FeePriorityAggressive, "aggressive", false},
		{"", 0, "", false},
		{"   ", 0, "", false},
		{"banana", 0, "", true},
	} {
		tier, err := transaction.ParseFeePriority(tc.in)
		if tc.err {
			if !errors.Is(err, transaction.ErrUnknownFeePriority) {
				t.Fatalf("ParseFeePriority(%q): got %v, want %v", tc.in, err, transaction.ErrUnknownFeePriority)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseFeePriority(%q): %v", tc.in, err)
		}
		if tier != tc.want {
			t.Fatalf("ParseFeePriority(%q) = %d, want %d", tc.in, tier, tc.want)
		}
		if got := transaction.FeePriorityName(tier); got != tc.name {
			t.Fatalf("FeePriorityName(%d) = %q, want %q", tier, got, tc.name)
		}
	}
}

func TestValidateFeePriority(t *testing.T) {
	t.Parallel()

	for _, tier := range []int{transaction.FeePriorityLow, transaction.FeePriorityMarket, transaction.FeePriorityAggressive} {
		if err := transaction.ValidateFeePriority(tier); err != nil {
			t.Fatalf("ValidateFeePriority(%d): %v", tier, err)
		}
	}
	for _, tier := range []int{0, -1, transaction.FeePriorityAggressive + 1} {
		if err := transaction.ValidateFeePriority(tier); !errors.Is(err, transaction.ErrUnknownFeePriority) {
			t.Fatalf("ValidateFeePriority(%d): got %v, want %v", tier, err, transaction.ErrUnknownFeePriority)
		}
	}
}
