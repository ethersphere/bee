// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sharky_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
)

// FuzzLocationCodec fuzzes the Location binary codec. LocationFromBinary /
// (*Location).UnmarshalBinary decode the persisted <shard,slot,length> reference
// that the local store reads back from disk, so the decoder must tolerate any
// byte slice — including a truncated or corrupt entry — by returning an error,
// never panicking (its signature promises an error return). On a successful
// decode the codec must round-trip: re-marshaling reproduces the consumed bytes.
func FuzzLocationCodec(f *testing.F) {
	valid, err := (&sharky.Location{Shard: 3, Slot: 42, Length: 17}).MarshalBinary()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add(make([]byte, sharky.LocationSize))

	f.Fuzz(func(t *testing.T, data []byte) {
		loc, err := sharky.LocationFromBinary(data) // must not panic on any input
		if err != nil {
			return
		}
		out, err := loc.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal after successful decode: %v", err)
		}
		if !bytes.Equal(out, data[:sharky.LocationSize]) {
			t.Fatalf("codec not round-trip stable: decoded %x, re-marshaled %x", data[:sharky.LocationSize], out)
		}
	})
}

// FuzzWriteRead drives fuzzed blobs through the real sharky Store write→read→release
// cycle (slot allocation, per-shard offset arithmetic, and read-back). It asserts
// the store never panics, that Write rejects oversized blobs with ErrTooLong rather
// than corrupting a shard, and that any blob it accepts reads back byte-for-byte.
func FuzzWriteRead(f *testing.F) {
	const (
		shardCnt    = 2
		maxDataSize = 64
	)

	dir := f.TempDir()
	s, err := sharky.New(&dirFS{basedir: dir}, shardCnt, maxDataSize)
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	f.Add([]byte("hello sharky"))
	f.Add([]byte{})
	f.Add(make([]byte, maxDataSize))
	f.Add(make([]byte, maxDataSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		loc, err := s.Write(ctx, data)
		if err != nil {
			if len(data) <= maxDataSize {
				t.Fatalf("write of %d bytes (<= max %d) failed: %v", len(data), maxDataSize, err)
			}
			return // oversized blobs are correctly rejected
		}
		if len(data) > maxDataSize {
			t.Fatalf("write accepted %d bytes exceeding max %d", len(data), maxDataSize)
		}

		buf := make([]byte, loc.Length)
		if err := s.Read(ctx, loc, buf); err != nil {
			t.Fatalf("read back accepted blob: %v", err)
		}
		if !bytes.Equal(buf, data) {
			t.Fatalf("round-trip mismatch: wrote %x, read %x", data, buf)
		}

		// release so the slot is reused and disk usage stays bounded across execs
		if err := s.Release(ctx, loc); err != nil {
			t.Fatalf("release: %v", err)
		}
	})
}
