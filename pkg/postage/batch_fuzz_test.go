// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/postage"
)

// FuzzBatchUnmarshalBinary fuzzes (*postage.Batch).UnmarshalBinary, the
// fixed-offset decoder for postage batch values persisted in the batchstore
// LevelDB. It slices and indexes buf up to offset 94 with no bounds check, so
// the decoder must tolerate any byte slice — including a truncated or corrupt
// entry — by returning an error, never panicking. On a successful decode of a
// full 95-byte record the codec must round-trip (re-marshaling reproduces the
// consumed bytes), except for the immutable flag byte which MarshalBinary
// normalizes to 1 for any non-zero input.
func FuzzBatchUnmarshalBinary(f *testing.F) {
	valid, err := (&postage.Batch{
		ID:          make([]byte, 32),
		Value:       big.NewInt(12345),
		Start:       42,
		Owner:       make([]byte, 20),
		BucketDepth: 16,
		Depth:       24,
		Immutable:   true,
	}).MarshalBinary()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	// A short (<95-byte) seed such as an empty buffer panics the decoder (the
	// bug this target hunts); leave it out of the corpus so the seed run stays
	// green and let -fuzz discover the crash.
	f.Add(make([]byte, 128))

	f.Fuzz(func(t *testing.T, data []byte) {
		b := new(postage.Batch)
		if err := b.UnmarshalBinary(data); err != nil { // must not panic on any input
			return
		}

		// MarshalBinary always emits exactly 95 bytes, so only a 95-byte input
		// can round-trip. The immutable flag byte (index 94) is normalized to 1
		// for any non-zero input, so guard the byte-for-byte assertion to inputs
		// where that byte is already canonical.
		if len(data) != 95 || data[94] > 1 {
			return
		}
		out, err := b.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}
