// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampindex

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzStampIndexItemUnmarshal drives stampindex.Item.Unmarshal with arbitrary
// bytes. Item is unexported, so this fuzz test lives in the internal package.
func FuzzStampIndexItemUnmarshal(f *testing.F) {
	// Valid seed built the production way via the export_test helper.
	batchTimestamp := make([]byte, swarm.StampTimestampSize)
	chunkAddress := swarm.NewAddress(bytes.Repeat([]byte{1}, swarm.HashSize))
	valid, err := NewItemWithValues(batchTimestamp, chunkAddress).Marshal()
	if err != nil {
		f.Fatalf("marshal seed: %v", err)
	}
	f.Add(valid)

	// Benign edge lengths the seed corpus tolerates.
	f.Add([]byte{})
	f.Add(make([]byte, 8))
	f.Add(append(make([]byte, 8), []byte("test_namespace")...))

	// Known crasher (finding): a leading uint64 of 0xFFFF... decodes to
	// nsLen = -1, which satisfies the self-referential size guard and then panics
	// in make([]byte, 0, nsLen). The dumb-mutation -fuzz run cannot reach this
	// (exact length AND exact 8-byte prefix), so it is added as a regression
	// reproducer.
	fixed := swarm.HashSize + swarm.StampIndexSize + swarm.StampTimestampSize + swarm.HashSize + swarm.HashSize
	neg := make([]byte, 8-1+fixed)
	binary.LittleEndian.PutUint64(neg, ^uint64(0))
	f.Add(neg)

	f.Fuzz(func(t *testing.T, data []byte) {
		var it Item
		if err := it.Unmarshal(data); err != nil {
			return
		}

		// On success, re-marshal and require round-trip byte equality.
		// Unmarshal accepts some inputs Marshal legitimately rejects (e.g. an
		// empty scope decoded from nsLen == 0); that asymmetry is not a decoder
		// panic bug, so skip the round-trip check when re-marshaling errors.
		got, err := it.Marshal()
		if err != nil {
			return
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("round-trip mismatch:\n got: %x\nwant: %x", got, data)
		}
	})
}
