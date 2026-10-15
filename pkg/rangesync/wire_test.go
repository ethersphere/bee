// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

func TestParseRange(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		in    *pb.Range
		valid bool
	}{
		{"nil", nil, false},
		{"root", &pb.Range{}, true},
		{"root with prefix", &pb.Range{Prefix: []byte{0}}, false},
		{"byte aligned", &pb.Range{Prefix: []byte{0xa5}, Bits: 8}, true},
		{"partial byte", &pb.Range{Prefix: []byte{0xa0}, Bits: 4}, true},
		{"tail bit set", &pb.Range{Prefix: []byte{0xa8}, Bits: 4}, false},
		{"last tail bit set", &pb.Range{Prefix: []byte{0x01}, Bits: 7}, false},
		{"short prefix", &pb.Range{Prefix: []byte{0xa5}, Bits: 9}, false},
		{"long prefix", &pb.Range{Prefix: []byte{0xa5, 0}, Bits: 8}, false},
		{"max bits", &pb.Range{Prefix: bytes.Repeat([]byte{0xff}, 31), Bits: fingerprint.MaxBits}, false},
		{"max bits canonical", &pb.Range{Prefix: append(bytes.Repeat([]byte{0xff}, 31), 0xfe), Bits: fingerprint.MaxBits}, true},
		{"bits over max", &pb.Range{Prefix: bytes.Repeat([]byte{0}, 32), Bits: 256}, false},
		{"bits wrapping uint8", &pb.Range{Prefix: []byte{}, Bits: 256 + 0}, false},
		{"huge bits", &pb.Range{Prefix: []byte{}, Bits: math.MaxUint32}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, err := rangesync.ParseRange(tc.in)
			if !tc.valid {
				if !isInvalid(err) {
					t.Fatalf("got %v, want invalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !r.Equal(fingerprint.NewRange(r.Prefix, r.Bits)) || len(r.Prefix.Bytes()) != (int(r.Bits)+7)/8 {
				t.Fatalf("range %v is not canonical", r)
			}
			if got := rangesync.EncodeRange(r); !bytes.Equal(got.Prefix, tc.in.Prefix) || got.Bits != tc.in.Bits {
				t.Fatalf("re-encoded %v, want %v", got, tc.in)
			}
		})
	}
}

func TestParseID(t *testing.T) {
	t.Parallel()

	a := bytes.Repeat([]byte{1}, 32)
	s := bytes.Repeat([]byte{2}, 32)
	for _, tc := range []struct {
		name  string
		in    *pb.ID
		valid bool
	}{
		{"nil", nil, false},
		{"valid", &pb.ID{Address: a, StampHash: s}, true},
		{"short address", &pb.ID{Address: a[:31], StampHash: s}, false},
		{"long stamp hash", &pb.ID{Address: a, StampHash: append(bytes.Clone(s), 0)}, false},
		{"empty", &pb.ID{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := rangesync.ParseID(tc.in)
			if !tc.valid {
				if !isInvalid(err) {
					t.Fatalf("got %v, want invalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rangesync.EncodeID(id), tc.in) {
				t.Fatal("id does not round trip")
			}
		})
	}
}

func TestParseFingerprint(t *testing.T) {
	t.Parallel()

	rng := &pb.Range{Prefix: []byte{0xa0}, Bits: 4}
	d := bytes.Repeat([]byte{7}, 32)
	for _, tc := range []struct {
		name  string
		in    *pb.Fingerprint
		valid bool
	}{
		{"nil", nil, false},
		{"nil range", &pb.Fingerprint{Count: 1, Digest: d}, false},
		{"invalid range", &pb.Fingerprint{Range: &pb.Range{Bits: 3}}, false},
		{"empty", &pb.Fingerprint{Range: rng}, true},
		{"empty with digest", &pb.Fingerprint{Range: rng, Digest: d}, false},
		{"empty with short digest", &pb.Fingerprint{Range: rng, Digest: d[:1]}, false},
		{"count", &pb.Fingerprint{Range: rng, Count: 3, Digest: d}, true},
		{"count without digest", &pb.Fingerprint{Range: rng, Count: 3}, false},
		{"count with 31 bytes", &pb.Fingerprint{Range: rng, Count: 3, Digest: d[:31]}, false},
		{"count with 33 bytes", &pb.Fingerprint{Range: rng, Count: 3, Digest: append(bytes.Clone(d), 0)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, s, err := rangesync.ParseFingerprint(tc.in)
			if !tc.valid {
				if !isInvalid(err) {
					t.Fatalf("got %v, want invalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.Count != tc.in.Count || (s.Count > 0 && !bytes.Equal(s.Digest[:], tc.in.Digest)) || (s.Count == 0 && s.Digest != [32]byte{}) {
				t.Fatalf("summary %+v from %v", s, tc.in)
			}
			if !reflect.DeepEqual(rangesync.EncodeFingerprint(r, s), tc.in) {
				t.Fatalf("re-encoded %v, want %v", rangesync.EncodeFingerprint(r, s), tc.in)
			}
		})
	}
}

func TestParseItems(t *testing.T) {
	t.Parallel()

	rng := newRand(1)
	r := rangeOf("1010")
	ids := randIDs(rng, r, 4)
	slicesSort(ids)
	outside := randID(rng, rangeOf("0"))
	enc := func(ids ...fingerprint.ID) []*pb.ID {
		out := make([]*pb.ID, len(ids))
		for i, id := range ids {
			out[i] = rangesync.EncodeID(id)
		}
		return out
	}
	page := func(n int) []*pb.ID {
		out := make([]fingerprint.ID, n)
		for i := range out {
			out[i] = randID(rng, r)
		}
		slicesSort(out)
		return enc(out...)
	}
	pr := rangesync.EncodeRange(r)

	for _, tc := range []struct {
		name  string
		in    *pb.Items
		valid bool
	}{
		{"nil", nil, false},
		{"nil range", &pb.Items{IDs: enc(ids...)}, false},
		{"empty last page", &pb.Items{Range: pr}, true},
		{"reply request", &pb.Items{Range: pr, Reply: true}, true},
		{"empty page with more", &pb.Items{Range: pr, More: true}, false},
		{"first page", &pb.Items{Range: pr, IDs: enc(ids[1:]...), More: true}, true},
		{"after", &pb.Items{Range: pr, IDs: enc(ids[1:]...), After: ids[0][:]}, true},
		{"after not before ids", &pb.Items{Range: pr, IDs: enc(ids[1:]...), After: ids[1][:]}, false},
		{"after short", &pb.Items{Range: pr, IDs: enc(ids[1:]...), After: ids[0][:63]}, false},
		{"after long", &pb.Items{Range: pr, IDs: enc(ids[1:]...), After: append(ids[0][:], 0)}, false},
		{"after outside range", &pb.Items{Range: pr, After: outside[:]}, false},
		{"id outside range", &pb.Items{Range: pr, IDs: enc(outside)}, false},
		{"unsorted", &pb.Items{Range: pr, IDs: enc(ids[1], ids[0])}, false},
		{"duplicate", &pb.Items{Range: pr, IDs: enc(ids[0], ids[0])}, false},
		{"invalid id", &pb.Items{Range: pr, IDs: []*pb.ID{{Address: ids[0][:32]}}}, false},
		{"nil id", &pb.Items{Range: pr, IDs: []*pb.ID{nil}}, false},
		{"full page", &pb.Items{Range: pr, IDs: page(rangesync.ItemsPageSize), More: true}, true},
		{"over page size", &pb.Items{Range: pr, IDs: page(rangesync.ItemsPageSize + 1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			it, err := rangesync.ParseItems(tc.in)
			if !tc.valid {
				if !isInvalid(err) {
					t.Fatalf("got %v, want invalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rangesync.EncodeItems(it), tc.in) {
				t.Fatalf("re-encoded %v, want %v", rangesync.EncodeItems(it), tc.in)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	t.Parallel()

	root := &pb.Range{Prefix: []byte{0x80}, Bits: 1}
	d := bytes.Repeat([]byte{9}, 32)
	for _, tc := range []struct {
		name  string
		in    *pb.Status
		valid bool
	}{
		{"nil", nil, false},
		{"nil root", &pb.Status{LeafBits: 12}, false},
		{"empty", &pb.Status{Root: root, LeafBits: 12, Mapping: 1}, true},
		{"empty with digest", &pb.Status{Root: root, Digest: d, LeafBits: 12}, false},
		{"count", &pb.Status{Root: root, Count: 5, Digest: d, LeafBits: 12, Mapping: 7}, true},
		{"count without digest", &pb.Status{Root: root, Count: 5, LeafBits: 12}, false},
		{"leaf at max bits", &pb.Status{Root: root, LeafBits: fingerprint.MaxBits - 1}, true},
		{"leaf past max bits", &pb.Status{Root: root, LeafBits: fingerprint.MaxBits}, false},
		// 1 + (1<<32 - 1) wraps to 0 in a 32-bit int sum.
		{"leaf bits wrapping int32", &pb.Status{Root: root, LeafBits: math.MaxUint32}, false},
		{"leaf bits wrapping uint8", &pb.Status{Root: root, LeafBits: 256 + 12}, false},
		{"audit fields ignored", &pb.Status{Root: root, LeafBits: 12, AuditRound: 3, AuditDigest: []byte{1}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st, err := rangesync.ParseStatus(tc.in)
			if !tc.valid {
				if !isInvalid(err) {
					t.Fatalf("got %v, want invalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if st.Summary.Count != tc.in.Count || uint32(st.LeafBits) != tc.in.LeafBits || st.Mapping != tc.in.Mapping {
				t.Fatalf("status %+v from %v", st, tc.in)
			}
			back, err := rangesync.ParseStatus(rangesync.EncodeStatus(st))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, st) {
				t.Fatalf("round trip %+v, want %+v", back, st)
			}
		})
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	t.Parallel()

	rng := newRand(2)
	for bits := 0; bits <= fingerprint.MaxBits; bits += 17 {
		var addr fingerprint.ID
		for i := range addr {
			addr[i] = byte(rng.Uint32())
		}
		// a non-canonical range is encoded canonically
		r := fingerprint.Range{Prefix: addr.Address(), Bits: uint8(bits)}
		got, err := rangesync.ParseRange(rangesync.EncodeRange(r))
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(r) {
			t.Fatalf("bits %d: got %v, want %v", bits, got, r)
		}

		s := fingerprint.Summary{Count: uint64(bits), Digest: [32]byte{byte(bits)}}
		if bits == 0 {
			s.Digest = [32]byte{}
		}
		gr, gs, err := rangesync.ParseFingerprint(rangesync.EncodeFingerprint(r, s))
		if err != nil {
			t.Fatal(err)
		}
		if !gr.Equal(r) || gs != s {
			t.Fatalf("fingerprint round trip: %v %+v", gr, gs)
		}
	}

	r := rangeOf("01")
	ids := randIDs(rng, r, 10)
	slicesSort(ids)
	for _, it := range []rangesync.Items{
		{Range: r},
		{Range: r, IDs: ids[1:], After: idPtr(ids[0]), More: true, Reply: true},
		{Range: r, IDs: ids},
	} {
		got, err := rangesync.ParseItems(rangesync.EncodeItems(it))
		if err != nil {
			t.Fatal(err)
		}
		if !got.Range.Equal(it.Range) || !reflect.DeepEqual(got.IDs, it.IDs) && len(it.IDs) > 0 || !reflect.DeepEqual(got.After, it.After) || got.More != it.More || got.Reply != it.Reply {
			t.Fatalf("items round trip: got %+v, want %+v", got, it)
		}
	}
}

func slicesSort(ids []fingerprint.ID) {
	slices.SortFunc(ids, fingerprint.ID.Compare)
}
