// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestNewHasher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		plaintext []byte
		hash      []byte
	}{
		{
			plaintext: []byte("Digital Freedom Now."),
			hash:      []byte{43, 108, 13, 242, 182, 16, 111, 176, 64, 234, 1, 180, 231, 199, 55, 85, 89, 149, 188, 70, 54, 111, 149, 1, 187, 76, 74, 232, 251, 194, 192, 190},
		},
		{
			plaintext: []byte("If Ethereum is the world's CPU, Swarm is the world's Hard Drive"),
			hash:      []byte{189, 23, 172, 191, 253, 137, 130, 94, 251, 161, 91, 101, 97, 229, 100, 172, 122, 47, 152, 84, 63, 116, 108, 216, 0, 66, 111, 10, 247, 85, 13, 210},
		},
	}

	for _, tc := range tests {
		h := swarm.NewHasher()

		_, err := h.Write(tc.plaintext)
		if err != nil {
			t.Fatal(err)
		}

		hash := h.Sum(nil)

		if !bytes.Equal(hash, tc.hash) {
			t.Fatalf("unexpected hash value")
		}
	}
}

func TestNewTrHasher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		plaintext []byte
		prefix    []byte
		hash      []byte
	}{
		{
			plaintext: []byte("Digital Freedom Now."),
			prefix:    []byte("swarm"),
			hash:      []byte{143, 34, 52, 99, 113, 222, 205, 101, 157, 242, 66, 64, 18, 236, 79, 127, 187, 2, 169, 109, 173, 96, 35, 38, 43, 254, 218, 153, 189, 177, 119, 115},
		},
		{
			plaintext: []byte("If Ethereum is the world's CPU, Swarm is the world's Hard Drive"),
			prefix:    []byte("swarm"),
			hash:      []byte{27, 253, 128, 158, 74, 63, 27, 178, 245, 169, 125, 189, 181, 37, 94, 163, 150, 155, 16, 175, 5, 215, 62, 90, 249, 72, 38, 80, 78, 119, 44, 206},
		},
	}

	// Run tests cases against TrHasher
	for _, tc := range tests {
		h := swarm.NewPrefixHasher(tc.prefix)

		_, err := h.Write(tc.plaintext)
		if err != nil {
			t.Fatal(err)
		}

		hash := h.Sum(nil)

		if !bytes.Equal(hash, tc.hash) {
			t.Fatalf("unexpected hash value")
		}
	}

	// Run tests against NewHasher
	// This time prefix needs to be manually included in hash
	for _, tc := range tests {
		h := swarm.NewHasher()

		_, err := h.Write(tc.prefix)
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.Write(tc.plaintext)
		if err != nil {
			t.Fatal(err)
		}

		hash := h.Sum(nil)

		if !bytes.Equal(hash, tc.hash) {
			t.Fatalf("unexpected hash value")
		}
	}
}
