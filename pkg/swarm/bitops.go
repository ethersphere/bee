// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

// Bit related utility functions

func (a *Address) Get(i int) bool {
	return getbit(a.b, i)
}

func (a *Address) Set(i int, v bool) {
	setbit(a.b, i, v)
}

func getbit(by []byte, i int) bool {
	bi := i / 8
	return by[bi]&(0x1<<uint(i%8)) != 0
}

func setbit(by []byte, i int, v bool) {
	bi := i / 8
	cv := getbit(by, i)
	if cv != v {
		by[bi] ^= 0x1 << uint8(i%8)
	}
}

// Prefix(n) returns first n bits of an address padded by zeroes
func (a *Address) Prefix(n int) (pref Address) {
	
	for i:=1; i <= n; i++ {
		pref.Set(i, a.Get(i))
	}
	for i:=n+1; i<= len(a.b)*8; i++{
		pref.Set(i, false)
	}
	return pref
}

//AddSuffix sets the bits in an address beginning from "suffixfrom" until length of suffix byteslice to the bits in suffix
func (a *Address) AddSuffix(suffix []byte, suffixfrom int) *Address {
	suffixtill := min(suffixfrom + len(suffix)*8, len(a.b)*8)
	for i:=suffixfrom; i<suffixtill; i++ {
		currentsuffixbit := i + 1 - suffixfrom
		a.Set(i, getbit(suffix, currentsuffixbit))
	}
	return a

}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}