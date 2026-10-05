// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzBatchRadiusItemUnmarshal fuzzes the persisted BatchRadiusItem codec.
// (*BatchRadiusItem).Unmarshal decodes a fixed-size record read back from the
// local LevelDB store, so it must tolerate any byte slice — including a
// truncated or corrupt entry — by returning an error, never panicking. On a
// successful decode with a non-zero address the codec must round-trip:
// re-marshaling reproduces the consumed bytes.
func FuzzBatchRadiusItemUnmarshal(f *testing.F) {
	valid, err := (&BatchRadiusItem{
		Bin:       3,
		BatchID:   bytes.Repeat([]byte{0xAB}, swarm.HashSize),
		Address:   swarm.NewAddress(bytes.Repeat([]byte{0x01}, swarm.HashSize)),
		BinID:     42,
		StampHash: bytes.Repeat([]byte{0xCD}, swarm.HashSize),
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, 10))
	f.Add(make([]byte, batchRadiusItemSize))

	f.Fuzz(func(t *testing.T, data []byte) {
		item := &BatchRadiusItem{}
		if err := item.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != batchRadiusItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), batchRadiusItemSize)
		}
		if item.Address.IsZero() {
			return // zero address decodes but cannot re-Marshal
		}
		out, err := item.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzChunkBinItemUnmarshal fuzzes the persisted ChunkBinItem codec.
// (*ChunkBinItem).Unmarshal decodes a fixed-size record from disk and must
// never panic on arbitrary bytes. On a successful decode with a non-zero
// address the codec must round-trip.
func FuzzChunkBinItemUnmarshal(f *testing.F) {
	valid, err := (&ChunkBinItem{
		Bin:       2,
		BinID:     7,
		Address:   swarm.NewAddress(bytes.Repeat([]byte{0x02}, swarm.HashSize)),
		BatchID:   bytes.Repeat([]byte{0xEF}, swarm.HashSize),
		ChunkType: swarm.ChunkTypeContentAddressed,
		StampHash: bytes.Repeat([]byte{0x12}, swarm.HashSize),
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, 10))
	f.Add(make([]byte, chunkBinItemSize))

	f.Fuzz(func(t *testing.T, data []byte) {
		item := &ChunkBinItem{}
		if err := item.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != chunkBinItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), chunkBinItemSize)
		}
		if item.Address.IsZero() {
			return // zero address decodes but cannot re-Marshal
		}
		out, err := item.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzBinItemUnmarshal fuzzes the persisted BinItem codec.
// (*BinItem).Unmarshal decodes an 8-byte big-endian binID from disk and must
// never panic on arbitrary bytes; on success the codec round-trips.
func FuzzBinItemUnmarshal(f *testing.F) {
	valid, err := (&BinItem{Bin: 4, BinID: 99}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, 4))

	f.Fuzz(func(t *testing.T, data []byte) {
		item := &BinItem{}
		if err := item.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != binItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), binItemSize)
		}
		out, err := item.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzEpochItemUnmarshal fuzzes the persisted EpochItem codec.
// (*EpochItem).Unmarshal decodes an 8-byte big-endian timestamp from disk and
// must never panic on arbitrary bytes; on success the codec round-trips.
func FuzzEpochItemUnmarshal(f *testing.F) {
	valid, err := (&EpochItem{Timestamp: 1234567890}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, 4))

	f.Fuzz(func(t *testing.T, data []byte) {
		item := &EpochItem{}
		if err := item.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != epochItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), epochItemSize)
		}
		out, err := item.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzRadiusItemUnmarshal fuzzes the persisted radiusItem codec.
// (*radiusItem).Unmarshal decodes a single storage-radius byte from disk and
// must never panic on arbitrary bytes; on success the codec round-trips.
func FuzzRadiusItemUnmarshal(f *testing.F) {
	valid, err := (&radiusItem{Radius: 8}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, 2))

	f.Fuzz(func(t *testing.T, data []byte) {
		item := &radiusItem{}
		if err := item.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != 1 {
			t.Fatalf("decoded buffer of len %d, want 1", len(data))
		}
		out, err := item.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}
