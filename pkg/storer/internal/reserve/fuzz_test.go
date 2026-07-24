// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const chunkBinItemSize = 1 + 8 + swarm.HashSize + swarm.HashSize + 1 + swarm.HashSize + storage.ChunkSumSize

// FuzzChunkBinItemUnmarshal feeds arbitrary bytes to the hand-rolled
// ChunkBinItem codec: it must never panic, must reject any length other than
// the canonical one, and every accepted value must survive a marshal
// round-trip byte for byte.
func FuzzChunkBinItemUnmarshal(f *testing.F) {
	valid := &reserve.ChunkBinItem{
		Bin:       1,
		BinID:     42,
		Address:   swarm.NewAddress(bytes.Repeat([]byte{0xaa}, swarm.HashSize)),
		BatchID:   bytes.Repeat([]byte{0xbb}, swarm.HashSize),
		StampHash: bytes.Repeat([]byte{0xcc}, swarm.HashSize),
		ChunkType: swarm.ChunkTypeSingleOwner,
		Sum:       bytes.Repeat([]byte{0xdd}, storage.ChunkSumSize),
	}
	buf, err := valid.Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(buf)
	f.Add([]byte{})
	f.Add(make([]byte, chunkBinItemSize-storage.ChunkSumSize)) // pre-Sum legacy size

	f.Fuzz(func(t *testing.T, data []byte) {
		item := &reserve.ChunkBinItem{}
		if err := item.Unmarshal(data); err != nil {
			return
		}
		if len(data) != chunkBinItemSize {
			t.Fatalf("accepted value of length %d, want only %d", len(data), chunkBinItemSize)
		}
		out, err := item.Marshal()
		if err != nil {
			t.Fatalf("unmarshaled value failed to marshal: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatal("marshal round-trip changed the value")
		}
	})
}

// FuzzParseChunkBinID feeds arbitrary key IDs to the raw-key parser used for
// key-only deletion of undecodable records: it must never panic, and every
// accepted ID must reproduce itself through the item's ID construction.
func FuzzParseChunkBinID(f *testing.F) {
	f.Add([]byte((&reserve.ChunkBinItem{Bin: 3, BinID: 7}).ID()))
	f.Add([]byte{})
	f.Add(make([]byte, 9))

	f.Fuzz(func(t *testing.T, id []byte) {
		bin, binID, err := reserve.ParseChunkBinID(string(id))
		if err != nil {
			return
		}
		if got := (&reserve.ChunkBinItem{Bin: bin, BinID: binID}).ID(); got != string(id) {
			t.Fatalf("parse/construct round-trip mismatch: %x -> %x", id, got)
		}
	})
}
