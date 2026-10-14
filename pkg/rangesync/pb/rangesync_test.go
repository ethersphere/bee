// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pb_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/gogo/protobuf/proto"
)

// message is the subset of the generated methods the round trip needs.
type message interface {
	proto.Message
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	Size() int
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	b32 := func(v byte) []byte { return bytes.Repeat([]byte{v}, 32) }
	rng := &pb.Range{Prefix: []byte{0xa5, 0x80}, Bits: 9}
	id := &pb.ID{Address: b32(1), StampHash: b32(2)}

	for _, tc := range []struct {
		name string
		msg  message
		new  func() message
	}{
		{"empty range", &pb.Range{}, func() message { return new(pb.Range) }},
		{"range", rng, func() message { return new(pb.Range) }},
		{"empty status", &pb.Status{}, func() message { return new(pb.Status) }},
		{"status", &pb.Status{
			Root:        rng,
			Count:       4_194_304,
			Digest:      b32(3),
			LeafBits:    12,
			AuditRound:  77,
			AuditDigest: b32(4),
			Mapping:     1,
		}, func() message { return new(pb.Status) }},
		{"status mapping only", &pb.Status{Mapping: 2}, func() message { return new(pb.Status) }},
		{"empty fingerprint", &pb.Fingerprint{Range: rng}, func() message { return new(pb.Fingerprint) }},
		{"fingerprint", &pb.Fingerprint{Range: rng, Count: 1000, Digest: b32(5)}, func() message { return new(pb.Fingerprint) }},
		{"symbols", &pb.Symbols{
			Range:     rng,
			Count:     1 << 40,
			Start:     4096,
			Data:      bytes.Repeat([]byte{6}, 2*82),
			Exhausted: true,
		}, func() message { return new(pb.Symbols) }},
		{"more", &pb.More{Range: rng, UpTo: 512}, func() message { return new(pb.More) }},
		{"id", id, func() message { return new(pb.ID) }},
		{"items", &pb.Items{
			Range: rng,
			IDs:   []*pb.ID{id, {Address: b32(7), StampHash: b32(8)}},
			After: append(b32(9), b32(10)...),
			More:  true,
			Reply: true,
		}, func() message { return new(pb.Items) }},
		{"items ask", &pb.Items{Range: rng, Reply: true}, func() message { return new(pb.Items) }},
		{"part fingerprint", &pb.Part{Kind: &pb.Part_FP{FP: &pb.Fingerprint{Range: rng, Count: 1, Digest: b32(11)}}}, func() message { return new(pb.Part) }},
		{"part symbols", &pb.Part{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rng, Count: 3, Data: []byte{1, 2, 3}}}}, func() message { return new(pb.Part) }},
		{"part more", &pb.Part{Kind: &pb.Part_More{More: &pb.More{Range: rng, UpTo: 16}}}, func() message { return new(pb.Part) }},
		{"part items", &pb.Part{Kind: &pb.Part_Items{Items: &pb.Items{Range: rng, IDs: []*pb.ID{id}}}}, func() message { return new(pb.Part) }},
		{"empty message", &pb.Message{}, func() message { return new(pb.Message) }},
		{"busy message", &pb.Message{Busy: true}, func() message { return new(pb.Message) }},
		{"message", &pb.Message{
			Parts: []*pb.Part{
				{Kind: &pb.Part_FP{FP: &pb.Fingerprint{Range: rng, Count: 2, Digest: b32(12)}}},
				{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rng, Start: 4, Data: []byte{4}}}},
				{Kind: &pb.Part_More{More: &pb.More{Range: rng, UpTo: 64}}},
				{Kind: &pb.Part_Items{Items: &pb.Items{Range: rng, IDs: []*pb.ID{id}, More: true}}},
			},
			AuditRound: 42,
		}, func() message { return new(pb.Message) }},
		{"empty want", &pb.Want{}, func() message { return new(pb.Want) }},
		{"want", &pb.Want{IDs: []*pb.ID{id, id}}, func() message { return new(pb.Want) }},
		{"delivery", &pb.Delivery{Address: b32(13), Data: bytes.Repeat([]byte{14}, 4104), Stamp: bytes.Repeat([]byte{15}, 113)}, func() message { return new(pb.Delivery) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := tc.msg.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			if len(data) != tc.msg.Size() {
				t.Fatalf("Size: got %d, encoded %d bytes", tc.msg.Size(), len(data))
			}
			got := tc.new()
			if err := got.Unmarshal(data); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.msg) {
				t.Fatalf("got %v, want %v", got, tc.msg)
			}
			// the generic proto path agrees with the generated one
			generic, err := proto.Marshal(tc.msg)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(generic, data) {
				t.Fatal("proto.Marshal differs from Marshal")
			}
		})
	}
}

// tag returns the protobuf key byte of a field number and wire type
// (0 varint, 2 length-delimited) for field numbers below 16.
func tag(field, wireType byte) byte {
	return field<<3 | wireType
}

// TestFieldNumbers pins the wire numbers of the fields reserved for later
// phases (Symbols, More, Status.Mapping, Message.AuditRound), so a schema edit
// that renumbers them fails here.
func TestFieldNumbers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		msg  message
		want []byte
	}{
		{"Status.Mapping", &pb.Status{Mapping: 1}, []byte{tag(7, 0), 1}},
		{"Status.AuditRound", &pb.Status{AuditRound: 1}, []byte{tag(5, 0), 1}},
		{"Status.AuditDigest", &pb.Status{AuditDigest: []byte{1}}, []byte{tag(6, 2), 1, 1}},
		{"Message.Busy", &pb.Message{Busy: true}, []byte{tag(2, 0), 1}},
		{"Message.AuditRound", &pb.Message{AuditRound: 1}, []byte{tag(3, 0), 1}},
		{"Part.FP", &pb.Part{Kind: &pb.Part_FP{FP: &pb.Fingerprint{}}}, []byte{tag(1, 2), 0}},
		{"Part.Symbols", &pb.Part{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{}}}, []byte{tag(2, 2), 0}},
		{"Part.More", &pb.Part{Kind: &pb.Part_More{More: &pb.More{}}}, []byte{tag(3, 2), 0}},
		{"Part.Items", &pb.Part{Kind: &pb.Part_Items{Items: &pb.Items{}}}, []byte{tag(4, 2), 0}},
		{"Symbols.Exhausted", &pb.Symbols{Exhausted: true}, []byte{tag(5, 0), 1}},
		{"More.UpTo", &pb.More{UpTo: 1}, []byte{tag(2, 0), 1}},
		{"Items.Reply", &pb.Items{Reply: true}, []byte{tag(5, 0), 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := tc.msg.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, tc.want) {
				t.Fatalf("got %x, want %x", data, tc.want)
			}
		})
	}
}
