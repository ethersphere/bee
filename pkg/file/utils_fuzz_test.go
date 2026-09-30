// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzChunkAddresses drives the exported intermediate-chunk payload parsers
// file.ChunkPayloadSize and file.ChunkAddresses. These decode the reference list
// out of an intermediate chunk's payload bytes: ChunkPayloadSize walks backwards
// in HashSize steps and ChunkAddresses slices data[offset:offset+HashSize] while
// stepping offset by reflen. Because the reference length and parity count are
// derived from the (fuzzed) payload, an offset-driven slice must never read out
// of bounds. The functions must tolerate any input without panicking.
func FuzzChunkAddresses(f *testing.F) {
	// Seed with valid, chunk-shaped payloads: whole multiples of HashSize (32)
	// and of the encrypted reference size (64), with a non-zero trailing
	// reference so ChunkPayloadSize reports the full length.
	mk := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i%255 + 1)
		}
		return b
	}
	f.Add(mk(0), uint8(0), false)
	f.Add(mk(swarm.HashSize), uint8(0), false)
	f.Add(mk(swarm.HashSize*2), uint8(0), false)
	f.Add(mk(swarm.HashSize*3), uint8(1), false)
	f.Add(mk(swarm.HashSize*4), uint8(0), true)
	f.Add(mk(encryption.ReferenceSize*2), uint8(0), true)
	f.Add(mk(swarm.ChunkSize), uint8(2), false)

	f.Fuzz(func(t *testing.T, data []byte, parities uint8, encRef bool) {
		reflen := swarm.HashSize
		if encRef {
			reflen = encryption.ReferenceSize
		}

		pSize, err := file.ChunkPayloadSize(data) // must not panic on any input
		if err != nil {
			return
		}
		payload := data[:pSize]

		// Bound the parity count relative to the payload actually handed to
		// ChunkAddresses (data[:pSize]), not the raw input length. A parity larger
		// than pSize/HashSize would drive shardCnt negative, but that is a harness
		// artifact of feeding an inconsistent parity rather than a defect in the
		// parser (the real payload's parity is always <= its reference count).
		mod := pSize/swarm.HashSize + 1
		parity := int(parities) % mod

		addrs, shardCnt := file.ChunkAddresses(payload, parity, reflen) // must not panic

		if shardCnt < 0 {
			t.Fatalf("negative shardCnt %d", shardCnt)
		}
		if shardCnt > len(payload)/reflen {
			t.Fatalf("shardCnt %d exceeds payload/reflen %d", shardCnt, len(payload)/reflen)
		}
		for _, a := range addrs {
			if len(a.Bytes()) != swarm.HashSize {
				t.Fatalf("address has length %d, want %d", len(a.Bytes()), swarm.HashSize)
			}
		}
	})
}
