// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

// RandAddress generates a random address.
func RandAddress(tb testing.TB) Address {
	tb.Helper()

	return NewAddress(testutil.RandBytes(tb, HashSize))
}

// RandAddressAt generates a random address at proximity order prox relative to address.
func RandAddressAt(tb testing.TB, self Address, prox int) Address {
	tb.Helper()

	addr := make([]byte, len(self.Bytes()))
	copy(addr, self.Bytes())
	pos := -1
	if prox >= 0 {
		pos = prox / 8
		trans := prox % 8
		transbytea := byte(0)
		for j := 0; j <= trans; j++ {
			transbytea |= 1 << uint8(7-j)
		}
		flipbyte := byte(1 << uint8(7-trans))
		transbyteb := transbytea ^ byte(255)
		randbyte := byte(rand.Intn(255))
		addr[pos] = ((addr[pos] & transbytea) ^ flipbyte) | randbyte&transbyteb
	}

	for i := pos + 1; i < len(addr); i++ {
		addr[i] = byte(rand.Intn(255))
	}

	a := NewAddress(addr)
	if a.Equal(self) {
		tb.Fatalf("generated same address")
	}

	return a
}

// RandAddresses generates slice with a random address.
func RandAddresses(tb testing.TB, count int) []Address {
	tb.Helper()

	result := make([]Address, count)
	for i := 0; i < count; i++ {
		result[i] = RandAddress(tb)
	}
	return result
}

// RandBatchID generates a random BatchID.
func RandBatchID(tb testing.TB) []byte {
	tb.Helper()

	return testutil.RandBytes(tb, HashSize)
}
