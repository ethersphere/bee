// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"math"
)

// Bit related utility functions

func (a *Address) Get(i int) bool {
	return a.b.bitget(i)
}

func (a *Address) Set(i int, v bool) {
	return a.b.bitset(i, v)
}

func (by []byte) bitget(i int) bool {
	bi := i / 8
	return by[bi]&(0x1<<uint(i%8)) != 0
}

func (by []byte) bitset(i int, v bool) {
	bi := i / 8
	cv := by.get(i)
	if cv != v {
		by[bi] ^= 0x1 << uint8(i%8)
	}
}

// Prefix(n) returns first n bits of an address padded by zeroes
func (a *Address) Prefix(n int) (pref Address) {
	
	prefix := make([]byte, len(a.b) )
	if n <= 0 {
		return 
	}
	for i:=1; i <= n; i++ {
		pref.Set(i, a.Get(i))
	}
	for i:=n+1; i<= len(a.b)*8; i++{
		pref.Set(i, false)
	}
}

//AddSuffix sets the bits in an address beginning from "suffixfrom" until length of suffix byteslice to the bits in suffix
func (a *Address) AddSuffix(suffix []byte, suffixfrom int) []byte {
	suffixtill := min(suffixfrom + len(suffix)*8, len(a.b)*8)
	for i:=suffixfrom; i<suffixtill; i++ {
		currentsuffixbit := i + 1 - suffixfrom
		a.b.bitset(i, suffix.getbit(currentsuffixbit))
	}
}

func (a []byte) CreateIdealAddress() Address {

}