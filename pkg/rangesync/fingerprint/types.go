// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"errors"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// IDSize is the length of an ID in bytes.
const IDSize = 2 * swarm.HashSize

// ID identifies one (chunk, stamp) pair of the reserve: the chunk address
// (32 bytes) followed by the stamp hash (32 bytes). IDs are ordered
// lexicographically, which orders them by address first.
type ID [IDSize]byte

// ErrInvalidID is returned by ParseID when the address or the stamp hash does
// not have exactly swarm.HashSize bytes.
var ErrInvalidID = errors.New("fingerprint: invalid id")

// NewID returns the ID of the chunk address addr and the stamp hash stampHash.
// It is meant for trusted values (from the storer): it does not validate its
// input, and a value that is not swarm.HashSize bytes long is zero padded on
// the right or truncated. Untrusted input goes through ParseID.
func NewID(addr swarm.Address, stampHash []byte) ID {
	return ID{}
}

// ParseID returns the ID of the chunk address addr and the stamp hash
// stampHash, as received from a peer. Both must be exactly swarm.HashSize
// bytes long; otherwise it returns an error wrapping ErrInvalidID.
func ParseID(addr, stampHash []byte) (ID, error) {
	return ID{}, nil
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

// Child returns the i-th sub-range of r that is bits bits deeper: r's prefix
// followed by the low bits bits of i, most significant bit first. With
// bits == FanoutBits it is the i-th child of a split; with bits == LeafBits
// below the root it is bucket i. bits must be at most 64 and r.Bits+bits at
// most 8*swarm.HashSize; the result is canonical.
func (r Range) Child(bits uint8, i uint64) Range {
	return Range{}
}

// Children returns the 1<<bits sub-ranges of r that are bits bits deeper, in
// address order: Children(bits)[i] equals Child(bits, i). They partition r.
// bits must be small enough for the result to fit in memory.
func (r Range) Children(bits uint8) []Range {
	return nil
}

// ChildIndex returns the bits-bit value of addr right after the first r.Bits
// bits, most significant bit first: the index i of the sub-range
// r.Child(bits, i) that contains addr. With bits == LeafBits below the root it
// is the bucket index; with bits == FanoutBits it is the child index of a
// split (the level-1 index below the root). It does not check that r contains
// addr; missing address bytes are taken as zero. bits must be at most 64.
func (r Range) ChildIndex(addr swarm.Address, bits uint8) uint64 {
	return 0
}

// String returns a readable form of r: the first r.Bits bits of the prefix as
// binary digits, followed by "/" and the number of bits, for example "0110/4".
// The range with Bits == 0, which contains every address, is "/0".
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
