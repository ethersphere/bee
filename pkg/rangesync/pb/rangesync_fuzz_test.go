// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pb_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
)

// FuzzUnmarshal decodes arbitrary bytes as each top-level message of the
// rangesync streams (Status, Message, Want, Delivery). Whatever is accepted
// must re-encode stably: encode(decode(encode(m))) == encode(m).
func FuzzUnmarshal(f *testing.F) {
	newMessage := []func() message{
		func() message { return new(pb.Status) },
		func() message { return new(pb.Message) },
		func() message { return new(pb.Want) },
		func() message { return new(pb.Delivery) },
	}

	rng := &pb.Range{Prefix: []byte{0xa5}, Bits: 8}
	id := &pb.ID{Address: bytes.Repeat([]byte{1}, 32), StampHash: bytes.Repeat([]byte{2}, 32)}
	for i, m := range []message{
		&pb.Status{Root: rng, Count: 3, Digest: bytes.Repeat([]byte{3}, 32), LeafBits: 12, Mapping: 1},
		&pb.Message{Parts: []*pb.Part{
			{Kind: &pb.Part_FP{FP: &pb.Fingerprint{Range: rng, Count: 1, Digest: bytes.Repeat([]byte{4}, 32)}}},
			{Kind: &pb.Part_Symbols{Symbols: &pb.Symbols{Range: rng, Count: 1, Data: []byte{5}}}},
			{Kind: &pb.Part_More{More: &pb.More{Range: rng, UpTo: 8}}},
			{Kind: &pb.Part_Items{Items: &pb.Items{Range: rng, IDs: []*pb.ID{id}, More: true, Reply: true}}},
		}, Busy: true, AuditRound: 1},
		&pb.Want{IDs: []*pb.ID{id}},
		&pb.Delivery{Address: id.Address, Data: []byte{6}, Stamp: []byte{7}},
	} {
		data, err := m.Marshal()
		if err != nil {
			f.Fatal(err)
		}
		f.Add(uint8(i), data)
		f.Add(uint8(i), []byte{})
		f.Add(uint8(i), data[:len(data)/2])
	}

	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		newMsg := newMessage[int(kind)%len(newMessage)]
		m := newMsg()
		if err := m.Unmarshal(data); err != nil {
			return
		}
		first, err := m.Marshal()
		if err != nil {
			t.Fatalf("re-encode accepted message: %v", err)
		}
		if len(first) != m.Size() {
			t.Fatalf("Size %d, encoded %d bytes", m.Size(), len(first))
		}
		again := newMsg()
		if err := again.Unmarshal(first); err != nil {
			t.Fatalf("decode re-encoded message: %v", err)
		}
		second, err := again.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("unstable encoding: %x then %x", first, second)
		}
	})
}
