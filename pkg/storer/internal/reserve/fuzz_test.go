// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
)

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
