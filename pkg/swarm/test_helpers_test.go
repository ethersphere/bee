// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func Test_RandAddress(t *testing.T) {
	t.Parallel()

	addr := swarm.RandAddress(t)
	assertNotZeroAddress(t, addr)
}

// TestRandAddressAt checks that RandAddressAt generates a correct random address
// at a given proximity order. It compares the number of leading equal bits in the generated
// address to the base address.
func Test_RandAddressAt(t *testing.T) {
	t.Parallel()

	base := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	b0 := base.Bytes()
	hw0 := []byte{b0[0], b0[1], 0, 0} // highest words of base address
	hw0int := binary.BigEndian.Uint32(hw0)

	for bitsInCommon := 0; bitsInCommon < 30; bitsInCommon++ {
		addr := swarm.RandAddressAt(t, base, bitsInCommon)
		assertNotZeroAddress(t, addr)

		b1 := addr.Bytes()
		hw1 := []byte{b1[0], b1[1], 0, 0} // highest words of 1
		hw1int := binary.BigEndian.Uint32(hw1)

		//bb0 is the bit mask to AND with hw0 and hw1
		bb0 := uint32(0)
		for i := 0; i < bitsInCommon; i++ {
			bb0 |= (1 << (31 - i))
		}

		andhw0 := hw0int & bb0
		andhw1 := hw1int & bb0

		// the result of the AND with both highest words of b0 and b1 should be equal
		if andhw0 != andhw1 {
			t.Fatalf("hw0 %08b hw1 %08b mask %08b &0 %08b &1 %08b", hw0int, hw1int, bb0, andhw0, andhw1)
		}
	}
}

func Test_RandAddresses(t *testing.T) {
	t.Parallel()

	count := 20
	addrs := swarm.RandAddresses(t, count)

	if got := len(addrs); got != count {
		t.Fatalf("expected %d, got %d", count, got)
	}
	for i := 0; i < count; i++ {
		assertNotZeroAddress(t, addrs[i])
	}
}

func assertNotZeroAddress(t *testing.T, addr swarm.Address) {
	t.Helper()

	if got := len(addr.Bytes()); got != swarm.HashSize {
		t.Fatalf("expected %d, got %d", swarm.HashSize, got)
	}

	if addr.Equal(swarm.ZeroAddress) {
		t.Fatalf("should not be zero address")
	}
}
