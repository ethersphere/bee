// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzDecodeSpan fuzzes the redundancy span decoder. DecodeSpan reads the
// redundancy level out of the most significant byte of a chunk span, and the
// span bytes are peer/persisted-controlled input, so the decoder must tolerate
// any byte slice — empty, truncated (<8 bytes), or oversized — without ever
// panicking (this covers the short-buffer/index-out-of-range class of bug, the
// same class as sharky's Location.UnmarshalBinary). On top of the no-panic
// contract it asserts structural invariants that always hold, and a
// EncodeLevel -> DecodeSpan round-trip over the level byte.
func FuzzDecodeSpan(f *testing.F) {
	// short buffers of every length below the span size
	for i := 0; i < swarm.SpanSize; i++ {
		f.Add(make([]byte, i), byte(0), 0)
	}
	// exactly the span size and larger
	f.Add(make([]byte, swarm.SpanSize), byte(1), swarm.Branches)
	f.Add(make([]byte, swarm.SpanSize+4), byte(0xff), -1)

	// valid encoded spans produced by the real EncodeLevel over a range of levels
	for _, l := range []byte{0, 1, 2, 3, 4, 127, 128, 255} {
		span := make([]byte, swarm.SpanSize)
		redundancy.EncodeLevel(span, redundancy.Level(l))
		f.Add(span, l, int(l))
	}

	f.Fuzz(func(t *testing.T, data []byte, levelByte byte, shards int) {
		// (1) DecodeSpan must not panic on ANY input, however short or long.
		level, span := redundancy.DecodeSpan(data)

		if len(span) != swarm.SpanSize {
			t.Fatalf("decoded span length = %d, want %d", len(span), swarm.SpanSize)
		}
		if level > 127 {
			t.Fatalf("decoded level = %d, must be <= 127", level)
		}

		// exercise the table.go getParities lookup and level.go shard math with a
		// fuzzed shard count; these must not panic and must return non-negatives.
		if p := level.GetParities(shards); p < 0 {
			t.Fatalf("GetParities(%d) = %d, want >= 0", shards, p)
		}
		if p := level.GetEncParities(shards); p < 0 {
			t.Fatalf("GetEncParities(%d) = %d, want >= 0", shards, p)
		}
		_ = level.GetMaxShards()
		_ = level.GetMaxEncShards()

		// (2) Round-trip: EncodeLevel writes level|128 into the last span byte, so
		// the level recovered by DecodeSpan is always the low 7 bits of the byte.
		fresh := make([]byte, swarm.SpanSize)
		redundancy.EncodeLevel(fresh, redundancy.Level(levelByte))
		got, rspan := redundancy.DecodeSpan(fresh)
		if len(rspan) != swarm.SpanSize {
			t.Fatalf("round-trip span length = %d, want %d", len(rspan), swarm.SpanSize)
		}
		want := redundancy.Level(levelByte & 0x7f)
		if got != want {
			t.Fatalf("round-trip level = %d, want %d (encoded byte %d)", got, want, levelByte)
		}
		if redundancy.IsLevelEncoded(fresh) != (want != 0) {
			t.Fatalf("IsLevelEncoded mismatch for encoded byte %d", levelByte)
		}
	})
}
