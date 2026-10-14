// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"encoding/binary"
	"fmt"
	"hash"
	"sync"

	"golang.org/x/crypto/sha3"
)

const (
	// LtHashLanes is the number of 16-bit lanes of an LtHash.
	LtHashLanes = 1024
	// LtHashSize is the size of an LtHash in bytes: the length of an element
	// expansion and of the input of Digest.
	LtHashSize = 2 * LtHashLanes

	// expandDomain is the domain separation prefix of the element expansion.
	expandDomain = "rangesync/v1"

	// ltHashWords is the number of uint64 words the lanes are packed in.
	ltHashWords = LtHashSize / 8
	// laneHigh has the top bit of each of the four lanes of a word set.
	laneHigh = 0x8000_8000_8000_8000
)

// LtHash is a lattice hash of a multiset of elements (proposal §4.3.3, §5.2):
// the lane-wise sum, modulo 2^16, of the 1024-lane expansions of its elements.
// It is a commutative group, so elements can be added and removed in any
// order and the result depends only on the multiset, and removal is exact.
//
// The zero value is the hash of the empty set. LtHash values are comparable
// with ==. The methods work in place, take their operands by pointer and do
// not allocate; the receiver and the argument may be the same value.
//
// Lane i is the little-endian uint16 at bytes 2i, 2i+1 of the expansion and of
// the Digest input. Internally four lanes are packed in a uint64 (lane i in
// bits 16·(i mod 4) of word i/4) so that additions run four lanes at a time;
// Lane reads a single lane.
type LtHash struct {
	w [ltHashWords]uint64
}

// Expand returns the element vector of id salted with salt:
// SHAKE128("rangesync/v1" ‖ salt ‖ id), 2048 bytes read as 1024
// little-endian uint16 lanes. The unsalted expansion used by Add and Sub is
// Expand(nil, id); an empty non-nil salt is the same unsalted form, so a
// caller that needs a salted value must reject an empty salt itself. The
// encoding is injective: the prefix is fixed and id has a fixed size, so salt
// and id are always recovered from the input.
//
// The salted form is the per-round audit of proposal §7.9, salted with the
// redistribution anchor; sum salted elements with AddHash.
func Expand(salt []byte, id ID) LtHash {
	var h LtHash
	e := getExpander(salt, id)
	for i := range h.w {
		h.w[i] = binary.LittleEndian.Uint64(e.out[8*i:])
	}
	expanders.Put(e)
	return h
}

// Add adds the element id to h.
func (h *LtHash) Add(id ID) {
	e := getExpander(nil, id)
	for i := range h.w {
		h.w[i] = add16(h.w[i], binary.LittleEndian.Uint64(e.out[8*i:]))
	}
	expanders.Put(e)
}

// Sub removes the element id from h. Removing an element that was never added
// is allowed: the result is the group difference.
func (h *LtHash) Sub(id ID) {
	e := getExpander(nil, id)
	for i := range h.w {
		h.w[i] = sub16(h.w[i], binary.LittleEndian.Uint64(e.out[8*i:]))
	}
	expanders.Put(e)
}

// AddHash sets h to h + o, the hash of the union of both multisets.
func (h *LtHash) AddHash(o *LtHash) {
	for i := range h.w {
		h.w[i] = add16(h.w[i], o.w[i])
	}
}

// SubHash sets h to h − o, the hash of the multiset difference.
func (h *LtHash) SubHash(o *LtHash) {
	for i := range h.w {
		h.w[i] = sub16(h.w[i], o.w[i])
	}
}

// Lane returns lane i of h. It panics unless 0 <= i < LtHashLanes.
func (h *LtHash) Lane(i int) uint16 {
	if i < 0 || i >= LtHashLanes {
		panic(fmt.Sprintf("fingerprint: lthash lane %d out of range", i))
	}
	return uint16(h.w[i/4] >> (16 * (i % 4)))
}

// Digest returns keccak256 of the 1024 lanes of h, each lane as two bytes
// little-endian, lane 0 first (2048 bytes in all). This is the 32-byte value
// sent on the wire.
//
// The digest of the zero value (the empty set) is keccak256 of 2048 zero
// bytes, not the zero [32]byte; Summary.Digest is defined to be zero when
// Count == 0, so callers special-case empty ranges.
func (h *LtHash) Digest() [32]byte {
	d := digesters.Get().(*digester)
	for i, w := range h.w {
		binary.LittleEndian.PutUint64(d.in[8*i:], w)
	}
	d.keccak.Reset()
	_, _ = d.keccak.Write(d.in[:])
	var out [32]byte
	copy(out[:], d.keccak.Sum(d.sum[:0]))
	digesters.Put(d)
	return out
}

// add16 adds x and y lane-wise modulo 2^16, four 16-bit lanes at a time: the
// low 15 bits of every lane are added with the top bits cleared, so no carry
// crosses a lane boundary, and the top bit is the XOR of both top bits and
// the carry out of the low bits.
func add16(x, y uint64) uint64 {
	return ((x &^ laneHigh) + (y &^ laneHigh)) ^ ((x ^ y) & laneHigh)
}

// sub16 subtracts y from x lane-wise modulo 2^16, four lanes at a time: with
// the top bit of every lane of x set and of y cleared, the low 15 bits never
// borrow across a lane boundary; the top bit is then corrected to
// x15 ⊕ y15 ⊕ borrow.
func sub16(x, y uint64) uint64 {
	return ((x | laneHigh) - (y &^ laneHigh)) ^ ((x ^ ^y) & laneHigh)
}

// expander holds the reusable state of one element expansion, so that Add,
// Sub and Expand do not allocate.
type expander struct {
	shake sha3.ShakeHash
	// in is expandDomain followed by the ID being expanded.
	in  [len(expandDomain) + IDSize]byte
	out [LtHashSize]byte
}

var expanders = sync.Pool{New: func() any {
	e := &expander{shake: sha3.NewShake128()}
	copy(e.in[:], expandDomain)
	return e
}}

// getExpander returns an expander from the pool whose out holds
// SHAKE128(expandDomain ‖ salt ‖ id). The caller returns it to the pool.
func getExpander(salt []byte, id ID) *expander {
	e := expanders.Get().(*expander)
	copy(e.in[len(expandDomain):], id[:])
	e.shake.Reset()
	_, _ = e.shake.Write(e.in[:len(expandDomain)])
	_, _ = e.shake.Write(salt)
	_, _ = e.shake.Write(e.in[len(expandDomain):])
	_, _ = e.shake.Read(e.out[:])
	return e
}

// digester holds the reusable state of Digest.
type digester struct {
	keccak hash.Hash
	in     [LtHashSize]byte
	sum    [32]byte
}

var digesters = sync.Pool{New: func() any {
	return &digester{keccak: sha3.NewLegacyKeccak256()}
}}
