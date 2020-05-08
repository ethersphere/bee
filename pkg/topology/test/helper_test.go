// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/test"
)

func TestRandomAddressAt(t *testing.T) {
	base := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")

	bitsInCommon := 16
	addr := test.RandomAddressAt(base, bitsInCommon)
	b0 := base.Bytes()
	b1 := addr.Bytes()

	hw0 := []byte{b0[0], b0[1]} // highest words of 0
	hw1 := []byte{b1[0], b1[1]} // highest words of 1

	// bb0 is the result of the XOR between b0 and b1 words
	bb0 := uint16(0)
	for i := 0; i < bitsInCommon; i++ {
		bb0 |= (1 << (31 - i))
	}
	xorb := make([]byte, 2)
	binary.BigEndian.PutUint16(xorb, bb0)

	for i := 0; i < 2; i++ {
		xb := hw0[i] ^ hw1[i]
		if xorb[i] != xb {
			t.Fatal("xor value mismatch. good luck debugging this")
		}
	}
}
