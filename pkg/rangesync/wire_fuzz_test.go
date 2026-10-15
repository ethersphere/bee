// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

func FuzzParseRange(f *testing.F) {
	f.Add([]byte{}, uint32(0))
	f.Add([]byte{0xa5}, uint32(8))
	f.Add([]byte{0xa0}, uint32(4))
	f.Add([]byte{0xa8}, uint32(4))
	f.Add([]byte{0xa5, 0}, uint32(8))
	f.Add(bytes.Repeat([]byte{0xff}, 32), uint32(255))
	f.Add(bytes.Repeat([]byte{0xff}, 32), uint32(256))
	f.Add([]byte{}, uint32(1<<32-1))

	f.Fuzz(func(t *testing.T, prefix []byte, bits uint32) {
		in := &pb.Range{Prefix: prefix, Bits: bits}
		r, err := rangesync.ParseRange(in)
		if err != nil {
			if !isInvalid(err) {
				t.Fatalf("error %v does not wrap ErrInvalidMessage", err)
			}
			return
		}
		if bits > fingerprint.MaxBits || len(prefix) != (int(bits)+7)/8 {
			t.Fatalf("accepted %d bits with %d bytes", bits, len(prefix))
		}
		out := rangesync.EncodeRange(r)
		if !bytes.Equal(out.Prefix, prefix) || out.Bits != bits {
			t.Fatalf("re-encoded %v, want %v", out, in)
		}
		if c := fingerprint.NewRange(r.Prefix, r.Bits); !bytes.Equal(c.Prefix.Bytes(), r.Prefix.Bytes()) {
			t.Fatal("range is not canonical")
		}
	})
}

func FuzzParseFingerprint(f *testing.F) {
	d := bytes.Repeat([]byte{7}, 32)
	f.Add([]byte{0xa0}, uint32(4), uint64(0), []byte{})
	f.Add([]byte{0xa0}, uint32(4), uint64(0), d)
	f.Add([]byte{0xa0}, uint32(4), uint64(3), d)
	f.Add([]byte{0xa0}, uint32(4), uint64(3), d[:31])
	f.Add([]byte{}, uint32(0), uint64(1<<64-1), d)

	f.Fuzz(func(t *testing.T, prefix []byte, bits uint32, count uint64, digest []byte) {
		in := &pb.Fingerprint{Range: &pb.Range{Prefix: prefix, Bits: bits}, Count: count, Digest: digest}
		r, s, err := rangesync.ParseFingerprint(in)
		if err != nil {
			if !isInvalid(err) {
				t.Fatalf("error %v does not wrap ErrInvalidMessage", err)
			}
			return
		}
		if s.Count != count || (count == 0) != (len(digest) == 0) || (count > 0 && !bytes.Equal(s.Digest[:], digest)) {
			t.Fatalf("summary %+v from count %d digest %x", s, count, digest)
		}
		gr, gs, err := rangesync.ParseFingerprint(rangesync.EncodeFingerprint(r, s))
		if err != nil || !gr.Equal(r) || gs != s {
			t.Fatalf("round trip: %v %+v %v", gr, gs, err)
		}
	})
}

func FuzzParseItems(f *testing.F) {
	rng := newRand(10)
	r := rangeOf("1010")
	ids := randIDs(rng, r, 3)
	slicesSort(ids)
	for _, it := range []*pb.Items{
		{Range: rangesync.EncodeRange(r)},
		{Range: rangesync.EncodeRange(r), Reply: true},
		rangesync.EncodeItems(rangesync.Items{Range: r, IDs: ids[1:], After: idPtr(ids[0]), More: true, Reply: true}),
		rangesync.EncodeItems(rangesync.Items{Range: r, IDs: []fingerprint.ID{ids[1], ids[0]}}),
		{Range: rangesync.EncodeRange(r), After: ids[0][:63]},
	} {
		data, err := it.Marshal()
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
		f.Add(data[:len(data)/2])
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		in := new(pb.Items)
		if err := in.Unmarshal(data); err != nil {
			return
		}
		it, err := rangesync.ParseItems(in)
		if err != nil {
			if !isInvalid(err) {
				t.Fatalf("error %v does not wrap ErrInvalidMessage", err)
			}
			return
		}
		if len(it.IDs) > rangesync.ItemsPageSize || it.More && len(it.IDs) == 0 {
			t.Fatalf("page of %d IDs with More %v", len(it.IDs), it.More)
		}
		prev := it.After
		if prev != nil && !it.Range.ContainsID(*prev) {
			t.Fatal("After outside the range")
		}
		for _, id := range it.IDs {
			if !it.Range.ContainsID(id) || prev != nil && id.Compare(*prev) <= 0 {
				t.Fatal("ID outside the range or out of order")
			}
			prev = idPtr(id)
		}
		back, err := rangesync.ParseItems(rangesync.EncodeItems(it))
		if err != nil {
			t.Fatal(err)
		}
		if !back.Range.Equal(it.Range) || len(back.IDs) != len(it.IDs) || len(it.IDs) > 0 && !reflect.DeepEqual(back.IDs, it.IDs) ||
			!reflect.DeepEqual(back.After, it.After) || back.More != it.More || back.Reply != it.Reply {
			t.Fatalf("round trip %+v, want %+v", back, it)
		}
	})
}

func FuzzParseStatus(f *testing.F) {
	d := bytes.Repeat([]byte{9}, 32)
	root := &pb.Range{Prefix: []byte{0x80}, Bits: 1}
	for _, st := range []*pb.Status{
		{Root: root, LeafBits: 12, Mapping: 1},
		{Root: root, Count: 5, Digest: d, LeafBits: 12, Mapping: 1},
		{Root: root, Digest: d, LeafBits: 12},
		{Root: root, LeafBits: 1<<32 - 1},
		{Root: root, LeafBits: 254, AuditRound: 3, AuditDigest: d},
	} {
		data, err := st.Marshal()
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		in := new(pb.Status)
		if err := in.Unmarshal(data); err != nil {
			return
		}
		st, err := rangesync.ParseStatus(in)
		if err != nil {
			if !isInvalid(err) {
				t.Fatalf("error %v does not wrap ErrInvalidMessage", err)
			}
			return
		}
		if uint64(in.GetRoot().GetBits())+uint64(in.LeafBits) > fingerprint.MaxBits {
			t.Fatalf("accepted root bits %d with leaf bits %d", in.GetRoot().GetBits(), in.LeafBits)
		}
		back, err := rangesync.ParseStatus(rangesync.EncodeStatus(st))
		if err != nil {
			t.Fatal(err)
		}
		if !back.Root.Equal(st.Root) || back.Summary != st.Summary || back.LeafBits != st.LeafBits || back.Mapping != st.Mapping {
			t.Fatalf("round trip %+v, want %+v", back, st)
		}
	})
}
