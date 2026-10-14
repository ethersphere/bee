// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	var id ID
	copy(id[:swarm.HashSize], addr.Bytes())
	copy(id[swarm.HashSize:], stampHash)
	return id
}

// ParseID returns the ID of the chunk address addr and the stamp hash
// stampHash, as received from a peer. Both must be exactly swarm.HashSize
// bytes long; otherwise it returns an error wrapping ErrInvalidID.
func ParseID(addr, stampHash []byte) (ID, error) {
	if len(addr) != swarm.HashSize {
		return ID{}, fmt.Errorf("%w: address length %d", ErrInvalidID, len(addr))
	}
	if len(stampHash) != swarm.HashSize {
		return ID{}, fmt.Errorf("%w: stamp hash length %d", ErrInvalidID, len(stampHash))
	}
	return NewID(swarm.NewAddress(addr), stampHash), nil
}

// Address returns the chunk address part of the ID.
func (id ID) Address() swarm.Address {
	return swarm.NewAddress(bytes.Clone(id[:swarm.HashSize]))
}

// StampHash returns the stamp hash part of the ID.
func (id ID) StampHash() []byte {
	return bytes.Clone(id[swarm.HashSize:])
}

// Compare returns -1, 0 or +1 depending on whether id sorts before, equal to
// or after other in the lexicographic order of IDs.
func (id ID) Compare(other ID) int {
	return bytes.Compare(id[:], other[:])
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

// MaxBits is the largest Bits a Range can hold (Bits is a uint8). Peer input
// that implies a deeper range, such as Status.Root.Bits + Status.LeafBits, is
// rejected against it.
const MaxBits = 255

const (
	// maxIndexBits is the largest number of bits a uint64 child index holds.
	maxIndexBits = 64
	// maxSpanBits is the largest leafBits of BucketSpan, so that the bucket
	// count 1<<leafBits fits in a uint64.
	maxSpanBits = 63
	// maxChildrenBits bounds the number of ranges Children allocates.
	maxChildrenBits = 16
)

// NewRange returns the canonical range of the first bits bits of addr. If addr
// is shorter than ceil(bits/8) bytes, the missing bytes are taken as zero;
// callers decoding untrusted input validate the length first.
func NewRange(addr swarm.Address, bits uint8) Range {
	p := make([]byte, prefixLen(int(bits)))
	copy(p, addr.Bytes())
	maskTail(p, int(bits))
	return Range{Prefix: swarm.NewAddress(p), Bits: bits}
}

// Equal reports whether r and other select the same set of addresses.
func (r Range) Equal(other Range) bool {
	return r.Bits == other.Bits && hasPrefix(other.Prefix.Bytes(), r.Prefix.Bytes(), int(r.Bits))
}

// Key returns the comparable form of r.
func (r Range) Key() RangeKey {
	k := RangeKey{Bits: r.Bits}
	n := prefixLen(int(r.Bits))
	copy(k.Prefix[:n], r.Prefix.Bytes())
	maskTail(k.Prefix[:n], int(r.Bits))
	return k
}

// Contains reports whether addr starts with the first r.Bits bits of the
// prefix. An address shorter than ceil(r.Bits/8) bytes is not contained.
func (r Range) Contains(addr swarm.Address) bool {
	return r.contains(addr.Bytes())
}

// ContainsID reports whether the address part of id lies in r.
func (r Range) ContainsID(id ID) bool {
	return r.contains(id[:swarm.HashSize])
}

func (r Range) contains(addr []byte) bool {
	if len(addr) < prefixLen(int(r.Bits)) {
		return false
	}
	return hasPrefix(addr, r.Prefix.Bytes(), int(r.Bits))
}

// ContainsRange reports whether every address in other also lies in r, that
// is other is r itself or one of its sub-ranges.
func (r Range) ContainsRange(other Range) bool {
	return other.Bits >= r.Bits && hasPrefix(other.Prefix.Bytes(), r.Prefix.Bytes(), int(r.Bits))
}

// Child returns the i-th sub-range of r that is bits bits deeper: r's prefix
// followed by the low bits bits of i, most significant bit first. With
// bits == FanoutBits it is the i-th child of a split; with bits == LeafBits
// below the root it is bucket i. bits must be at most 64 and r.Bits+bits at
// most MaxBits; Child panics otherwise. The result is canonical.
func (r Range) Child(bits uint8, i uint64) Range {
	if bits > maxIndexBits {
		panic(fmt.Sprintf("fingerprint: child bits %d over %d", bits, maxIndexBits))
	}
	depth := int(r.Bits) + int(bits)
	if depth > MaxBits {
		panic(fmt.Sprintf("fingerprint: child depth %d over %d", depth, MaxBits))
	}
	p := make([]byte, prefixLen(depth))
	n := prefixLen(int(r.Bits))
	copy(p[:n], r.Prefix.Bytes())
	maskTail(p[:n], int(r.Bits))
	for k := 0; k < int(bits); k++ {
		if i>>(int(bits)-1-k)&1 == 1 {
			pos := int(r.Bits) + k
			p[pos/8] |= 0x80 >> (pos % 8)
		}
	}
	return Range{Prefix: swarm.NewAddress(p), Bits: uint8(depth)}
}

// Children returns the 1<<bits sub-ranges of r that are bits bits deeper, in
// address order: Children(bits)[i] equals Child(bits, i). They partition r.
// bits must be at most 16 and r.Bits+bits at most MaxBits; Children panics
// otherwise, before allocating.
func (r Range) Children(bits uint8) []Range {
	if bits > maxChildrenBits {
		panic(fmt.Sprintf("fingerprint: children bits %d over %d", bits, maxChildrenBits))
	}
	if depth := int(r.Bits) + int(bits); depth > MaxBits {
		panic(fmt.Sprintf("fingerprint: children depth %d over %d", depth, MaxBits))
	}
	children := make([]Range, 1<<bits)
	for i := range children {
		children[i] = r.Child(bits, uint64(i))
	}
	return children
}

// ChildIndex returns the bits-bit value of addr right after the first r.Bits
// bits, most significant bit first: the index i of the sub-range
// r.Child(bits, i) that contains addr. With bits == LeafBits below the root it
// is the bucket index; with bits == FanoutBits it is the child index of a
// split (the level-1 index below the root). It does not check that r contains
// addr; missing address bytes are taken as zero. bits must be at most 64;
// ChildIndex panics otherwise.
//
// ChildIndex reads every bit of addr it is asked for. To locate a sub-range
// rather than an address, pass its canonical prefix (NewRange), or use
// BucketSpan: the prefix of a non-canonical range may carry bits past its
// Bits that would end up in the index.
func (r Range) ChildIndex(addr swarm.Address, bits uint8) uint64 {
	if bits > maxIndexBits {
		panic(fmt.Sprintf("fingerprint: child index bits %d over %d", bits, maxIndexBits))
	}
	a := addr.Bytes()
	var v uint64
	for k := 0; k < int(bits); k++ {
		v = v<<1 | uint64(bitAt(a, int(r.Bits)+k))
	}
	return v
}

// BucketSpan returns the buckets of root that r covers. The buckets are the
// 1<<leafBits sub-ranges leafBits bits below root, numbered as by Child and
// ChildIndex; r covers the n = 1<<(root.Bits+leafBits-r.Bits) consecutive
// buckets starting at lo. Bits past r.Bits in r's prefix are ignored.
//
// r may come from a peer, so violations are reported with ok false rather
// than a panic: root must contain r (root.ContainsRange(r)), r must be at
// most leafBits bits below root, and leafBits must be at most 63.
func (root Range) BucketSpan(r Range, leafBits uint8) (lo, n uint64, ok bool) {
	if leafBits > maxSpanBits || !root.ContainsRange(r) {
		return 0, 0, false
	}
	depth := r.Bits - root.Bits // root.ContainsRange(r) implies r.Bits >= root.Bits
	if depth > leafBits {
		return 0, 0, false
	}
	index := root.ChildIndex(NewRange(r.Prefix, r.Bits).Prefix, depth)
	shift := leafBits - depth
	return index << shift, 1 << shift, true
}

// String returns a readable form of r: the first r.Bits bits of the prefix as
// binary digits, followed by "/" and the number of bits, for example "0110/4".
// The range with Bits == 0, which contains every address, is "/0".
func (r Range) String() string {
	var sb strings.Builder
	sb.Grow(int(r.Bits) + 4)
	p := r.Prefix.Bytes()
	for k := 0; k < int(r.Bits); k++ {
		sb.WriteByte('0' + bitAt(p, k))
	}
	sb.WriteByte('/')
	sb.WriteString(strconv.Itoa(int(r.Bits)))
	return sb.String()
}

// Summary is the fingerprint of a range: the number of elements in it and the
// digest of their LtHash (keccak256 of the LtHash lanes). When Count is 0,
// Digest is the zero value, matching the wire, where it is omitted.
type Summary struct {
	Count  uint64
	Digest [32]byte
}

// prefixLen returns the number of bytes that hold bits bits.
func prefixLen(bits int) int {
	return (bits + 7) / 8
}

// maskTail zeroes the bits of p past the first bits bits.
func maskTail(p []byte, bits int) {
	for i := range p {
		switch {
		case 8*(i+1) <= bits:
		case 8*i >= bits:
			p[i] = 0
		default:
			p[i] &= 0xff << (8 - bits%8)
		}
	}
}

// byteAt returns b[i], or 0 if b is too short.
func byteAt(b []byte, i int) byte {
	if i < len(b) {
		return b[i]
	}
	return 0
}

// bitAt returns bit i of b, most significant bit first, or 0 if b is too
// short.
func bitAt(b []byte, i int) byte {
	return byteAt(b, i/8) >> (7 - i%8) & 1
}

// hasPrefix reports whether the first bits bits of a and prefix are equal.
// Missing bytes of either are taken as zero.
func hasPrefix(a, prefix []byte, bits int) bool {
	full := bits / 8
	for i := 0; i < full; i++ {
		if byteAt(a, i) != byteAt(prefix, i) {
			return false
		}
	}
	if rem := bits % 8; rem != 0 {
		mask := byte(0xff) << (8 - rem)
		return byteAt(a, full)&mask == byteAt(prefix, full)&mask
	}
	return true
}
