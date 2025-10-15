// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

// Proximity returns the proximity order of the MSB distance between x and y
//
// The distance metric MSB(x, y) of two equal length byte sequences x an y is the
// value of the binary integer cast of the x^y, ie., x and y bitwise xor-ed.
// the binary cast is big endian: most significant bit first (=MSB).
//
// Proximity(x, y) is a discrete logarithmic scaling of the MSB distance.
// It is defined as the reverse rank of the integer part of the base 2
// logarithm of the distance.
// It is calculated by counting the number of common leading zeros in the (MSB)
// binary representation of the x^y.
//
// (0 farthest, 255 closest, 256 self)
func Proximity(one, other []byte) (ret uint8) {
	b := MaxPO/8 + 1
	if l := uint8(len(one)); b > l {
		b = l
	}
	if l := uint8(len(other)); b > l {
		b = l
	}
	var m uint8 = 8
	for i := uint8(0); i < b; i++ {
		oxo := one[i] ^ other[i]
		for j := uint8(0); j < m; j++ {
			if (oxo>>(7-j))&0x01 != 0 {
				return i*8 + j
			}
		}
	}
	return MaxPO
}

func ExtendedProximity(one, other []byte) (ret uint8) {
	b := ExtendedPO/8 + 1
	if l := uint8(len(one)); b > l {
		b = l
	}
	if l := uint8(len(other)); b > l {
		b = l
	}
	var m uint8 = 8
	for i := uint8(0); i < b; i++ {
		oxo := one[i] ^ other[i]
		for j := uint8(0); j < m; j++ {
			if (oxo>>(7-j))&0x01 != 0 {
				return i*8 + j
			}
		}
	}
	return ExtendedPO
}
