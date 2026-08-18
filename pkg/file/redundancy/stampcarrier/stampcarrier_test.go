// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampcarrier_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy/stampcarrier"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func randStamp(t *testing.T, batchID []byte) []byte {
	t.Helper()
	s := make([]byte, postage.StampSize)
	if _, err := rand.Read(s); err != nil {
		t.Fatal(err)
	}
	copy(s[:32], batchID)
	return s
}

func TestConstants(t *testing.T) {
	t.Parallel()
	if stampcarrier.HeaderSize != 34 || stampcarrier.EntrySize != 83 {
		t.Fatalf("format sizes changed: header %d entry %d", stampcarrier.HeaderSize, stampcarrier.EntrySize)
	}
	// capacity per spec §4: floor((4096-34)/83) = 48
	if stampcarrier.MaxEntries != 48 {
		t.Fatalf("expected 48 entries per carrier, got %d", stampcarrier.MaxEntries)
	}
}

func TestCount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ children, want int }{
		{0, 0}, {1, 1}, {48, 1}, {49, 2}, {96, 2}, {97, 3}, {123, 3},
	} {
		if got := stampcarrier.Count(tc.children); got != tc.want {
			t.Errorf("Count(%d) = %d, want %d", tc.children, got, tc.want)
		}
	}
}

func TestPackUnpackRoundtrip(t *testing.T) {
	t.Parallel()
	batchID := make([]byte, 32)
	if _, err := rand.Read(batchID); err != nil {
		t.Fatal(err)
	}
	const children = 123 // full MEDIUM parent: 114 data + 9 parity
	stamps := make([][]byte, children)
	for i := range stamps {
		stamps[i] = randStamp(t, batchID)
	}
	// one missing stamp must be tolerated and simply omitted
	stamps[5] = nil

	payloads, err := stampcarrier.Pack(stamps)
	if err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 3 {
		t.Fatalf("expected 3 payloads, got %d", len(payloads))
	}
	for _, p := range payloads {
		if len(p) != swarm.ChunkSize {
			t.Fatalf("payload not padded to chunk size: %d", len(p))
		}
	}

	got := make(map[uint16][]byte)
	for _, p := range payloads {
		m, err := stampcarrier.Unpack(p)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range m {
			got[k] = v
		}
	}
	if len(got) != children-1 {
		t.Fatalf("expected %d entries, got %d", children-1, len(got))
	}
	for i, s := range stamps {
		if s == nil {
			if _, ok := got[uint16(i)]; ok {
				t.Fatalf("slot %d should be absent", i)
			}
			continue
		}
		if !bytes.Equal(got[uint16(i)], s) {
			t.Fatalf("slot %d stamp mismatch", i)
		}
	}
}

func TestPackSlotMapping(t *testing.T) {
	t.Parallel()
	// the stamp of slot i must be in payload i/48 (spec §4)
	batchID := make([]byte, 32)
	stamps := make([][]byte, 100)
	for i := range stamps {
		stamps[i] = randStamp(t, batchID)
	}
	payloads, err := stampcarrier.Pack(stamps)
	if err != nil {
		t.Fatal(err)
	}
	for i := range stamps {
		m, err := stampcarrier.Unpack(payloads[i/stampcarrier.MaxEntries])
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := m[uint16(i)]; !ok {
			t.Fatalf("slot %d not found in payload %d", i, i/stampcarrier.MaxEntries)
		}
	}
}

func TestGroupReconstruct(t *testing.T) {
	t.Parallel()
	for _, c := range []int{1, 2, 3} {
		batchID := make([]byte, 32)
		stamps := make([][]byte, c*stampcarrier.MaxEntries)
		for i := range stamps {
			stamps[i] = randStamp(t, batchID)
		}
		payloads, err := stampcarrier.Pack(stamps)
		if err != nil {
			t.Fatal(err)
		}
		parities, err := stampcarrier.EncodeGroup(payloads)
		if err != nil {
			t.Fatal(err)
		}
		if len(parities) != stampcarrier.GroupParities {
			t.Fatalf("expected %d parities, got %d", stampcarrier.GroupParities, len(parities))
		}

		// lose any 2 of the c+2 group members; recovery must succeed
		shards := make([][]byte, c+stampcarrier.GroupParities)
		copy(shards, payloads)
		copy(shards[c:], parities)
		shards[0] = nil
		shards[c] = nil // one carrier + one parity

		if err := stampcarrier.ReconstructGroup(shards, c); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(shards[0], payloads[0]) {
			t.Fatalf("c=%d: reconstructed payload differs", c)
		}
	}
}

func TestGroupReconstructTooManyLost(t *testing.T) {
	t.Parallel()
	stamps := make([][]byte, 3*stampcarrier.MaxEntries)
	for i := range stamps {
		stamps[i] = randStamp(t, make([]byte, 32))
	}
	payloads, _ := stampcarrier.Pack(stamps)
	parities, _ := stampcarrier.EncodeGroup(payloads)
	shards := make([][]byte, 5)
	copy(shards, payloads)
	copy(shards[3:], parities)
	shards[0], shards[1], shards[2] = nil, nil, nil // 3 of 5 lost
	if err := stampcarrier.ReconstructGroup(shards, 3); err == nil {
		t.Fatal("expected reconstruction error with 3 of 5 lost")
	}
}

func TestPackShortStampReturnsError(t *testing.T) {
	t.Parallel()
	// A short first stamp (less than 113 bytes) must return an error,
	// not panic with "slice bounds out of range" during batch ID discovery.
	stamps := make([][]byte, 2)
	stamps[0] = make([]byte, 31) // short: 31 bytes instead of 113
	stamps[1] = randStamp(t, make([]byte, 32))

	_, err := stampcarrier.Pack(stamps)
	if err == nil {
		t.Fatal("expected error for short stamp, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("size")) {
		t.Fatalf("expected size-related error, got: %v", err)
	}
}
