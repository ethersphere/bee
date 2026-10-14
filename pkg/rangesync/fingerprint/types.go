// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// IDSize is the length of an ID in bytes.
const IDSize = 2 * swarm.HashSize

// ID identifies one (chunk, stamp) pair of the reserve: the chunk address
// (32 bytes) followed by the stamp hash (32 bytes). IDs are ordered
// lexicographically, which orders them by address first.
type ID [IDSize]byte

// NewID returns the ID of the chunk address addr and the stamp hash stampHash.
// Both are expected to be swarm.HashSize bytes long; shorter values are zero
// padded on the right and longer values are truncated.
func NewID(addr swarm.Address, stampHash []byte) ID {
	return ID{}
}

// Address returns the chunk address part of the ID.
func (id ID) Address() swarm.Address {
	return swarm.ZeroAddress
}

// StampHash returns the stamp hash part of the ID.
func (id ID) StampHash() []byte {
	return nil
}

// Compare returns -1, 0 or +1 depending on whether id sorts before, equal to
// or after other in the lexicographic order of IDs.
func (id ID) Compare(other ID) int {
	return 0
}

// Range is an address prefix: it holds every element whose address starts
// with the first Bits bits of Prefix. Ranges are either nested or disjoint.
//
// Prefix must hold at least ceil(Bits/8) bytes; bits past Bits are ignored.
// NewRange returns the canonical form, which holds exactly ceil(Bits/8) bytes
// with the bits past Bits set to zero, as sent on the wire.
type Range struct {
	Prefix swarm.Address
	Bits   uint8
}

// RangeKey is the comparable form of a Range, usable as a map key. Ranges that
// are Equal have the same key.
type RangeKey struct {
	Prefix [swarm.HashSize]byte
	Bits   uint8
}

// NewRange returns the canonical range of the first bits bits of addr. If addr
// is shorter than ceil(bits/8) bytes, the missing bytes are taken as zero;
// callers decoding untrusted input validate the length first.
func NewRange(addr swarm.Address, bits uint8) Range {
	return Range{}
}

// Equal reports whether r and other select the same set of addresses.
func (r Range) Equal(other Range) bool {
	return false
}

// Key returns the comparable form of r.
func (r Range) Key() RangeKey {
	return RangeKey{}
}

// Contains reports whether addr starts with the first r.Bits bits of the
// prefix. An address shorter than ceil(r.Bits/8) bytes is not contained.
func (r Range) Contains(addr swarm.Address) bool {
	return false
}

// ContainsID reports whether the address part of id lies in r.
func (r Range) ContainsID(id ID) bool {
	return false
}

// ContainsRange reports whether every address in other also lies in r, that
// is other is r itself or one of its sub-ranges.
func (r Range) ContainsRange(other Range) bool {
	return false
}

// String returns a readable form of r: the prefix bits followed by "/" and
// the number of bits, for example "0110/4".
func (r Range) String() string {
	return ""
}

// Summary is the fingerprint of a range: the number of elements in it and the
// digest of their LtHash (keccak256 of the LtHash lanes). When Count is 0,
// Digest is the zero value, matching the wire, where it is omitted.
type Summary struct {
	Count  uint64
	Digest [32]byte
}
