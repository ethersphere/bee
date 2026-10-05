// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func pullsyncFrame(tb testing.TB, msg protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzGetRead fuzzes the read path the pullsync handler uses to parse an
// incoming range request.
func FuzzGetRead(f *testing.F) {
	f.Add(pullsyncFrame(f, &pb.Get{Bin: 3, Start: 42}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var g pb.Get
		if err := r.ReadMsg(&g); err != nil {
			return
		}
		_ = uint8(g.Bin) // handler narrows Bin to uint8
	})
}

// FuzzOfferRead fuzzes the read path the pullsync client uses to parse an offer
// returned by a peer, mirroring the per-chunk address handling in Sync.
func FuzzOfferRead(f *testing.F) {
	f.Add(pullsyncFrame(f, &pb.Offer{
		Topmost: 10,
		Chunks: []*pb.Chunk{
			{Address: make([]byte, swarm.HashSize), BatchID: make([]byte, swarm.HashSize), StampHash: make([]byte, swarm.HashSize)},
		},
	}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var o pb.Offer
		if err := r.ReadMsg(&o); err != nil {
			return
		}
		for _, c := range o.Chunks {
			if c == nil {
				continue
			}
			_ = swarm.NewAddress(c.Address)
		}
	})
}

// FuzzWantRead fuzzes the read path the pullsync handler uses to parse a want
// bitvector returned by a peer.
func FuzzWantRead(f *testing.F) {
	f.Add(pullsyncFrame(f, &pb.Want{BitVector: []byte{0xff, 0x0f}}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var w pb.Want
		if err := r.ReadMsg(&w); err != nil {
			return
		}
		_ = w.BitVector
	})
}

// FuzzDeliveryRead fuzzes the read path the pullsync client uses to parse a
// chunk delivery, mirroring the chunk construction and stamp decode in Sync.
func FuzzDeliveryRead(f *testing.F) {
	f.Add(pullsyncFrame(f, &pb.Delivery{
		Address: make([]byte, swarm.HashSize),
		Data:    []byte("chunk data"),
		Stamp:   make([]byte, postage.StampSize),
	}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var d pb.Delivery
		if err := r.ReadMsg(&d); err != nil {
			return
		}
		_ = swarm.NewChunk(swarm.NewAddress(d.Address), d.Data)
		_ = new(postage.Stamp).UnmarshalBinary(d.Stamp)
	})
}
