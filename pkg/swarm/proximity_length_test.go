// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestProximityLongOperands guards against the operand length being truncated
// to uint8: a 256-byte address used to wrap to length 0 and report MaxPO for an
// address sharing no prefix at all.
func TestProximityLongOperands(t *testing.T) {
	t.Parallel()

	base := bytes.Repeat([]byte{0xAA}, swarm.HashSize)

	for _, length := range []int{256, 512} {
		// 0x55 is the complement of 0xAA, so the operands differ at bit 0.
		other := bytes.Repeat([]byte{0x55}, length)

		if got := swarm.Proximity(base, other); got != 0 {
			t.Errorf("Proximity with a %d-byte operand = %d, want 0", length, got)
		}
		if got := swarm.ExtendedProximity(base, other); got != 0 {
			t.Errorf("ExtendedProximity with a %d-byte operand = %d, want 0", length, got)
		}
	}
}
