// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzPushItemUnmarshal fuzzes the persisted pushItem codec.
// (*pushItem).Unmarshal decodes a fixed-size record (Timestamp|Address|BatchID|
// TagID = pushItemSize) read back from the local LevelDB store, so it must
// tolerate any byte slice — including a truncated or corrupt entry — by
// returning an error, never panicking. On a successful decode with a non-zero
// address the codec must round-trip: re-marshaling reproduces the input.
func FuzzPushItemUnmarshal(f *testing.F) {
	valid, err := (&pushItem{
		Timestamp: 1234567890,
		Address:   swarm.NewAddress(bytes.Repeat([]byte{0x01}, swarm.HashSize)),
		BatchID:   bytes.Repeat([]byte{0xAB}, swarm.HashSize),
		TagID:     42,
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, pushItemSize-1))
	f.Add(make([]byte, pushItemSize))
	f.Add(make([]byte, pushItemSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		var pi pushItem
		if err := pi.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != pushItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), pushItemSize)
		}
		if pi.Address.IsZero() {
			return // zero address decodes but cannot re-Marshal
		}
		out, err := pi.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzTagItemUnmarshal fuzzes the persisted TagItem codec.
// (*TagItem).Unmarshal decodes a fixed-size record (6 counters | Address |
// StartedAt = tagItemSize) read back from disk and must never panic on
// arbitrary bytes. On a successful decode the codec must round-trip; the
// address is decoded via internal.AddressOrZero over exactly 32 bytes, so both
// the zero-address and non-zero-address cases re-marshal to the same bytes.
func FuzzTagItemUnmarshal(f *testing.F) {
	valid, err := (&TagItem{
		TagID:     1,
		Split:     2,
		Seen:      3,
		Stored:    4,
		Sent:      5,
		Synced:    6,
		Address:   swarm.NewAddress(bytes.Repeat([]byte{0x02}, swarm.HashSize)),
		StartedAt: 987654321,
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, tagItemSize-1))
	f.Add(make([]byte, tagItemSize))
	f.Add(make([]byte, tagItemSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		var ti TagItem
		if err := ti.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != tagItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), tagItemSize)
		}
		out, err := ti.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzUploadItemUnmarshal fuzzes the persisted uploadItem value codec.
// (*uploadItem).Unmarshal decodes a fixed-size value (TagID|Uploaded|Synced =
// uploadItemSize); Address and BatchID live in the key, not the marshaled
// value. It must never panic on arbitrary bytes. On a successful decode the
// three value fields must round-trip once valid key-derived Address/BatchID are
// supplied (Marshal requires them set but does not encode them into the value).
func FuzzUploadItemUnmarshal(f *testing.F) {
	valid, err := (&uploadItem{
		Address:  swarm.NewAddress(bytes.Repeat([]byte{0x03}, swarm.HashSize)),
		BatchID:  bytes.Repeat([]byte{0xCD}, swarm.HashSize),
		TagID:    7,
		Uploaded: 111,
		Synced:   222,
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, uploadItemSize-1))
	f.Add(make([]byte, uploadItemSize))
	f.Add(make([]byte, uploadItemSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		var ui uploadItem
		if err := ui.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != uploadItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), uploadItemSize)
		}
		// Address/BatchID are key-derived, not part of the value codec; supply
		// valid dummies so Marshal can produce the value bytes for comparison.
		ui.Address = swarm.NewAddress(bytes.Repeat([]byte{0x03}, swarm.HashSize))
		ui.BatchID = bytes.Repeat([]byte{0xCD}, swarm.HashSize)
		out, err := ui.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("value codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzDirtyTagItemUnmarshal fuzzes the persisted dirtyTagItem codec.
// (*dirtyTagItem).Unmarshal decodes a fixed-size record (TagID|Started =
// dirtyTagItemSize) from disk and must never panic on arbitrary bytes. On a
// successful decode the codec must round-trip.
func FuzzDirtyTagItemUnmarshal(f *testing.F) {
	valid, err := (&dirtyTagItem{TagID: 9, Started: 555}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, dirtyTagItemSize-1))
	f.Add(make([]byte, dirtyTagItemSize))
	f.Add(make([]byte, dirtyTagItemSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		var d dirtyTagItem
		if err := d.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != dirtyTagItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), dirtyTagItemSize)
		}
		out, err := d.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}

// FuzzNextTagIDUnmarshal fuzzes the persisted nextTagID codec.
// (*nextTagID).Unmarshal decodes a single little-endian uint64 (8 bytes) from
// disk and must never panic on arbitrary bytes. On a successful decode the
// codec must round-trip.
func FuzzNextTagIDUnmarshal(f *testing.F) {
	valid, err := nextTagID(42).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, 7))
	f.Add(make([]byte, 8))
	f.Add(make([]byte, 9))

	f.Fuzz(func(t *testing.T, data []byte) {
		var n nextTagID
		if err := n.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != 8 {
			t.Fatalf("decoded buffer of len %d, want 8", len(data))
		}
		out, err := n.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}
