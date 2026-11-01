// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzCacheEntryUnmarshal fuzzes the persisted cacheEntry codec.
// (*cacheEntry).Unmarshal decodes a fixed-size record (Address | AccessTimestamp
// = cacheEntrySize) read back from the local LevelDB store, so it must tolerate
// any byte slice — including a truncated or corrupt entry — by returning an
// error, never panicking. Round-trip is asserted only when Marshal succeeds:
// Marshal rejects a zero address or a non-positive timestamp, both of which
// Unmarshal accepts, so a validly-decoded-but-unmarshalable entry is expected.
func FuzzCacheEntryUnmarshal(f *testing.F) {
	valid, err := (&cacheEntry{
		Address:         swarm.NewAddress(bytes.Repeat([]byte{0x01}, swarm.HashSize)),
		AccessTimestamp: 1234567890,
	}).Marshal()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(nil))
	f.Add(make([]byte, cacheEntrySize-1))
	f.Add(make([]byte, cacheEntrySize))
	f.Add(make([]byte, cacheEntrySize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		var e cacheEntry
		if err := e.Unmarshal(data); err != nil { // must not panic on any input
			return
		}
		if len(data) != cacheEntrySize {
			t.Fatalf("decoded buffer of len %d, want %d", len(data), cacheEntrySize)
		}
		out, err := e.Marshal()
		if err != nil {
			return // zero address / non-positive timestamp decode but cannot re-Marshal
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data, out)
		}
	})
}
