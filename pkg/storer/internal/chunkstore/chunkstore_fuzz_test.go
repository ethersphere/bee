// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzRetrievalIndexItemUnmarshal drives chunkstore.RetrievalIndexItem.Unmarshal
// with arbitrary bytes. The type is exported, so this lives in the external
// chunkstore_test package.
func FuzzRetrievalIndexItemUnmarshal(f *testing.F) {
	// Valid seed built the production way.
	item := &chunkstore.RetrievalIndexItem{
		Address:   swarm.NewAddress(bytes.Repeat([]byte{1}, swarm.HashSize)),
		Timestamp: 1,
		Location:  sharky.Location{Shard: 1, Slot: 2, Length: 3},
		RefCnt:    5,
	}
	valid, err := item.Marshal()
	if err != nil {
		f.Fatalf("marshal seed: %v", err)
	}
	f.Add(valid)

	// Benign edge lengths (off-by-one around RetrievalIndexItemSize == 51).
	f.Add([]byte{})
	f.Add(make([]byte, 50))
	f.Add(make([]byte, 52))

	f.Fuzz(func(t *testing.T, data []byte) {
		var it chunkstore.RetrievalIndexItem
		if err := it.Unmarshal(data); err != nil {
			return
		}

		// On success, re-marshal and require round-trip byte equality.
		got, err := it.Marshal()
		if err != nil {
			t.Fatalf("marshal after successful unmarshal: %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("round-trip mismatch:\n got: %x\nwant: %x", got, data)
		}
	})
}
