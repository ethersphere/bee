// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"encoding/binary"
	"hash"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	zerospan    = make([]byte, 8)
	zerosection = make([]byte, 64)
)

// SIMDOptIn is default false, meaning it is disabled by default unless the user turns it on with the
// necessary CLI flag on the supported systems.
var SIMDOptIn = false

// LengthToSpan creates a binary data span size representation.
// It is required for calculating the BMT hash.
func LengthToSpan(length int64) []byte {
	span := make([]byte, SpanSize)
	binary.LittleEndian.PutUint64(span, uint64(length))
	return span
}

// LengthFromSpan returns length from span.
func LengthFromSpan(span []byte) uint64 {
	return binary.LittleEndian.Uint64(span)
}

// calculates the Keccak256 SHA3 hash of the data
func sha3hash(data ...[]byte) ([]byte, error) {
	return doHash(swarm.NewHasher(), data...)
}

// calculates Hash of the data
func doHash(h hash.Hash, data ...[]byte) ([]byte, error) {
	h.Reset()
	for _, v := range data {
		if _, err := h.Write(v); err != nil {
			return nil, err
		}
	}
	return h.Sum(nil), nil
}

// SEGMENT_SIZE is the keccak256 output size in bytes, also the BMT leaf segment size.
const SEGMENT_SIZE = 32

// sizeToParams calculates the depth (number of levels) and segment count in the BMT tree.
func sizeToParams(n int) (c, d int) {
	c = 2
	for ; c < n; c *= 2 {
		d++
	}
	return c, d + 1
}
