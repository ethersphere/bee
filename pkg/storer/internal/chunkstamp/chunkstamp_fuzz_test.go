// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstamp

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzChunkStampItemUnmarshal drives chunkstamp.Item.Unmarshal with arbitrary
// bytes. Item is unexported, so this fuzz test lives in the internal package.
// The receiver must carry a non-zero address, since Unmarshal rejects a
// zero-address receiver AFTER the size guard but BEFORE the vulnerable slice.
func FuzzChunkStampItemUnmarshal(f *testing.F) {
	addr := swarm.NewAddress(bytes.Repeat([]byte{1}, swarm.HashSize))

	// Valid seed built the production way.
	stamp := postagetesting.MustNewStamp()
	valid, err := (&Item{}).
		WithAddress(addr).
		WithNamespace("scope").
		WithStamp(stamp).
		Marshal()
	if err != nil {
		f.Fatalf("marshal seed: %v", err)
	}
	f.Add(valid)

	// Benign edge lengths the seed corpus tolerates.
	f.Add([]byte{})
	f.Add(make([]byte, 8))
	f.Add(append(make([]byte, 8), []byte("scope")...))

	// Known crasher (finding): a leading uint64 of 0xFFFF... decodes to
	// nsLen = -1, which satisfies the self-referential size guard
	// (len == 8+nsLen+StampSize) at exactly 8-1+StampSize bytes and then panics
	// in make([]byte, 0, nsLen) / bytes[8:8+nsLen]. The dumb-mutation -fuzz run
	// cannot reach this (needs an exact length AND an exact 8-byte prefix), so
	// it is added explicitly here as a regression reproducer.
	neg := make([]byte, 8-1+postage.StampSize)
	binary.LittleEndian.PutUint64(neg, ^uint64(0))
	f.Add(neg)

	f.Fuzz(func(t *testing.T, data []byte) {
		target := (&Item{}).WithAddress(addr.Clone())
		if err := target.Unmarshal(data); err != nil {
			return
		}

		// On success, re-marshal and require round-trip byte equality.
		// Unmarshal accepts some inputs Marshal legitimately rejects (e.g. an
		// empty scope decoded from nsLen == 0); that asymmetry is not a decoder
		// panic bug, so skip the round-trip check when re-marshaling errors.
		got, err := target.Marshal()
		if err != nil {
			return
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("round-trip mismatch:\n got: %x\nwant: %x", got, data)
		}
	})
}
