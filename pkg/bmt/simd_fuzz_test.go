// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package bmt_test

import (
	"bytes"
	"hash"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/bmt/reference"
	"github.com/ethersphere/bee/v2/pkg/keccak"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// fuzzMaxPrefixLen caps the prefix length. A prefix of 72 bytes or more pushes
// the prefix||left||right lanes past the 136-byte keccak rate, so the cap
// covers both single- and multi-block SIMD lanes.
const fuzzMaxPrefixLen = 256

// FuzzSIMDHasher checks that the SIMD BMT hasher, at every batch width the CPU
// supports, produces the same chunk hash as the reference BMT implementation.
func FuzzSIMDHasher(f *testing.F) {
	seed := []byte("swarm bmt simd fuzz seed")
	// countSel selects 1 + countSel%128 segments
	f.Add([]byte{}, uint8(127), []byte{})
	f.Add(bytes.Repeat(seed, 200), uint8(127), []byte{})
	f.Add(seed, uint8(127), []byte{})
	f.Add(seed, uint8(0), []byte{})
	f.Add(seed, uint8(4), []byte{})
	f.Add(bytes.Repeat(seed, 200), uint8(127), []byte("prefix00"))
	// prefixes around the 72-byte boundary where lanes reach the keccak rate
	f.Add(bytes.Repeat(seed, 200), uint8(127), bytes.Repeat([]byte{0x01}, 71))
	f.Add(bytes.Repeat(seed, 200), uint8(127), bytes.Repeat([]byte{0x02}, 72))
	f.Add(bytes.Repeat(seed, 200), uint8(127), bytes.Repeat([]byte{0x03}, 73))

	f.Fuzz(func(t *testing.T, data []byte, countSel uint8, prefix []byte) {
		var widths []int
		if keccak.HasSIMD() {
			widths = append(widths, 4)
		}
		if keccak.HasAVX512() {
			widths = append(widths, 8)
		}
		if len(widths) == 0 {
			t.Skip("AVX2 not available on this CPU")
		}

		count := 1 + int(countSel)%swarm.BmtBranches
		if len(prefix) > fuzzMaxPrefixLen {
			prefix = prefix[:fuzzMaxPrefixLen]
		}

		for _, width := range widths {
			h := bmt.NewSIMDHasher(prefix, count, width)
			if len(data) > h.Capacity() {
				data = data[:h.Capacity()]
			}
			want, err := refPrefixHash(prefix, count, data)
			if err != nil {
				t.Fatal(err)
			}

			// dirty the hasher first: pooled hashers are reused after Reset
			// without clearing their buffer, so bytes left over from a previous
			// chunk must not leak into this one.
			if _, err := syncHash(h, bytes.Repeat([]byte{0xFF}, h.Capacity())); err != nil {
				t.Fatal(err)
			}
			got, err := syncHash(h, data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("width=%d segments=%d prefix_len=%d data_len=%d:\n  simd      %x\n  reference %x",
					width, count, len(prefix), len(data), got, want)
			}
		}
	})
}

// refPrefixHash is refHash with an optional prefix, which the BMT absorbs
// before every section and before the final span||root wrap.
func refPrefixHash(prefix []byte, count int, data []byte) ([]byte, error) {
	newHasher := swarm.NewHasher
	if len(prefix) > 0 {
		newHasher = func() hash.Hash { return swarm.NewPrefixHasher(prefix) }
	}
	root, err := reference.NewRefHasher(newHasher(), count).Hash(data)
	if err != nil {
		return nil, err
	}
	h := newHasher()
	if _, err := h.Write(bmt.LengthToSpan(int64(len(data)))); err != nil {
		return nil, err
	}
	if _, err := h.Write(root); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
