// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reference_test

import (
	"bytes"
	"fmt"
	"hash"
	"testing"

	"github.com/ethersphere/bee/pkg/bmt/reference"
	"gitlab.com/nolash/go-mockbytes"

	"golang.org/x/crypto/sha3"
)

// calculates the hash of the data using hash.Hash
func doSum(h hash.Hash, b []byte, data ...[]byte) ([]byte, error) {
	h.Reset()
	for _, v := range data {
		var err error
		_, err = h.Write(v)
		if err != nil {
			return nil, err
		}
	}
	return h.Sum(b), nil
}

// calculates the Keccak256 SHA3 hash of the data
func sha3hash(t *testing.T, data ...[]byte) []byte {
	t.Helper()
	h := sha3.NewLegacyKeccak256()
	r, err := doSum(h, nil, data...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestRefHasher tests that the RefHasher computes the expected BMT hash for some small data lengths.
func TestRefHasher(t *testing.T) {
	// the test struct is used to specify the expected BMT hash for
	// segment counts between from and to and lengths from 1 to datalength
	for i, x := range []struct {
		from     int
		to       int
		expected func([]byte) []byte
	}{
		{
			// all lengths in [0,64] should be:
			//
			//   sha3hash(data)
			//
			from: 1,
			to:   2,
			expected: func(d []byte) []byte {
				data := make([]byte, 64)
				copy(data, d)
				return sha3hash(t, data)
			},
		}, {
			// all lengths in [3,4] should be:
			//
			//   sha3hash(
			//     sha3hash(data[:64])
			//     sha3hash(data[64:])
			//   )
			//
			from: 3,
			to:   4,
			expected: func(d []byte) []byte {
				data := make([]byte, 128)
				copy(data, d)
				return sha3hash(t, sha3hash(t, data[:64]), sha3hash(t, data[64:]))
			},
		}, {
			// all bmttestutil.SegmentCounts in [5,8] should be:
			//
			//   sha3hash(
			//     sha3hash(
			//       sha3hash(data[:64])
			//       sha3hash(data[64:128])
			//     )
			//     sha3hash(
			//       sha3hash(data[128:192])
			//       sha3hash(data[192:])
			//     )
			//   )
			//
			from: 5,
			to:   8,
			expected: func(d []byte) []byte {
				data := make([]byte, 256)
				copy(data, d)
				return sha3hash(t, sha3hash(t, sha3hash(t, data[:64]), sha3hash(t, data[64:128])), sha3hash(t, sha3hash(t, data[128:192]), sha3hash(t, data[192:])))
			},
		},
	} {
		for segCount := x.from; segCount <= x.to; segCount++ {
			for length := 1; length <= segCount*32; length++ {
				t.Run(fmt.Sprintf("%d_segments_%d_bytes", segCount, length), func(t *testing.T) {
					g := mockbytes.New(i, mockbytes.MockTypeStandard)
					data, err := g.RandomBytes(length)
					if err != nil {
						t.Fatal(err)
					}
					expected := x.expected(data)
					actual, err := reference.NewRefHasher(sha3.NewLegacyKeccak256(), segCount).Hash(data)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(actual, expected) {
						t.Fatalf("expected %x, got %x", expected, actual)
					}
				})
			}
		}
	}
}
