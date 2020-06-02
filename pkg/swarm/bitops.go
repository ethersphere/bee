// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

// Bit related utility functions

func (a *Address) Get(i int) bool {
	return Get(a.b, i)
}

func (a *Address) Set(i int, v bool) {
	Set(a.b, i, v)
}

func (a *Address) SwitchOneBit(i int) {
	Set(a.b, i, !Get(a.b, i))
}

func Get(by []byte, i int) bool {
	e := i / 8
	return by[e]&(0x1<<uint(i%8)) != 0
}

func Set(by []byte, i int, v bool) {
	e := i / 8
	cv := Get(by, i)
	if cv != v {
		by[e] ^= 0x1 << uint8(i%8)
	}
	//fmt.Println("sot %v", i)
}

// Prefix(n) returns first n bits of an address padded by zeroes
func (a *Address) Prefix(n int) Address {
	prefb := make([]byte, 64)
	pref := NewAddress(prefb)
	for i := 1; i <= n; i++ {
		pref.Set(i, a.Get(i))
	}
	for i := n + 1; i <= len(a.b)*8; i++ {
		pref.Set(i, false)
	}
	return pref
}

//AddSuffix sets the bits in an address beginning from "suffixfrom" until length of suffix byteslice to the bits in suffix
func (a *Address) AddSuffix(suffix []byte, suffixfrom int) *Address {
	suffixtill := MinimumInt(suffixfrom+len(suffix)*8-1, len(a.b)*8)
	for i := suffixfrom; i < suffixtill; i++ {
		currentsuffixbit := i + 1 - suffixfrom
		a.Set(i, Get(suffix, currentsuffixbit))
	}
	return a
}

func MinimumInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
