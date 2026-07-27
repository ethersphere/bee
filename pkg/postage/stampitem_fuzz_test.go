// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzStampItemUnmarshal fuzzes (*StampItem).Unmarshal, the fixed-offset
// decoder for the persisted stampItem storage value
// (BatchID(32)|chunkAddress(32)|BatchIndex(8)|BatchTimestamp(8)+1). It has an
// explicit exact-size guard, so it is self-bounding; the fuzzer asserts it
// never panics on arbitrary bytes and that any accepted 81-byte record
// round-trips. This lives in the internal package because Marshal requires the
// unexported chunkAddress field to build a valid seed the production way.
func FuzzStampItemUnmarshal(f *testing.F) {
	valid, err := (StampItem{
		BatchID:        make([]byte, swarm.HashSize),
		chunkAddress:   swarm.NewAddress(make([]byte, swarm.HashSize)),
		BatchIndex:     make([]byte, swarm.StampIndexSize),
		BatchTimestamp: make([]byte, swarm.StampTimestampSize),
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte{})
	f.Add(make([]byte, stampItemSize))   // 80: one short of the required size
	f.Add(make([]byte, stampItemSize+2)) // 82: one over the required size

	f.Fuzz(func(t *testing.T, data []byte) {
		it := new(StampItem)
		if err := it.Unmarshal(data); err != nil { // must not panic on any input
			return
		}

		// On success the input is guaranteed to be exactly stampItemSize+1 bytes
		// and Unmarshal populated BatchID and chunkAddress at HashSize each, so
		// Marshal cannot fail. Marshal only ever writes the first stampItemSize
		// bytes; the trailing byte (index stampItemSize) is padding that Marshal
		// leaves 0 and Unmarshal ignores, so a byte-for-byte round-trip only holds
		// when that byte is already canonical (0), matching production data.
		if data[stampItemSize] != 0 {
			return
		}
		out, err := it.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}
