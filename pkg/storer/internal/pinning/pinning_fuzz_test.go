// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzPinCollectionItemUnmarshal fuzzes the persisted pinCollectionItem codec.
// (*pinCollectionItem).Unmarshal decodes a fixed-size record (Addr | UUID |
// Stat.Total | Stat.DupInCollection = pinCollectionItemSize) read back from the
// local LevelDB store. The address field is 64 bytes but is interpreted as
// either a 32-byte plain hash or a 64-byte encrypted reference depending on
// whether the interior bytes buf[32:64] are all zero, so the decoder branches
// on interior bytes and slices at several offsets — it must tolerate any byte
// slice by returning an error, never panicking. On a successful decode with a
// non-zero address the codec must round-trip on both branches.
func FuzzPinCollectionItemUnmarshal(f *testing.F) {
	plain, err := (&pinCollectionItem{
		Addr: swarm.NewAddress(bytes.Repeat([]byte{0x01}, swarm.HashSize)),
		UUID: bytes.Repeat([]byte{0x02}, uuidSize),
		Stat: CollectionStat{Total: 10, DupInCollection: 3},
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	enc, err := (&pinCollectionItem{
		Addr: swarm.NewAddress(bytes.Repeat([]byte{0x03}, 64)),
		UUID: bytes.Repeat([]byte{0x04}, uuidSize),
		Stat: CollectionStat{Total: 99, DupInCollection: 7},
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(plain)
	f.Add(enc)
	f.Add([]byte(nil))
	f.Add(make([]byte, pinCollectionItemSize-1))
	f.Add(make([]byte, pinCollectionItemSize))
	f.Add(make([]byte, pinCollectionItemSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		var p pinCollectionItem
		if err := p.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != pinCollectionItemSize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), pinCollectionItemSize)
		}
		if p.Addr.IsZero() {
			return // zero address decodes but cannot re-Marshal
		}
		out, err := p.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}
