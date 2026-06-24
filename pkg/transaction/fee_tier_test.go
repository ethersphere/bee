// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/transaction"
)

func TestParseFeeTier(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want string
		err  bool
	}{
		{"low", "low", false},
		{" Market ", "market", false},
		{"AGGRESSIVE", "aggressive", false},
		{"", "market", false},
		{"banana", "", true},
	} {
		tier, err := transaction.ParseFeeTier(tc.in)
		if tc.err {
			if err == nil {
				t.Fatalf("ParseFeeTier(%q): want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseFeeTier(%q): %v", tc.in, err)
		}
		if got := tier.String(); got != tc.want {
			t.Fatalf("ParseFeeTier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
