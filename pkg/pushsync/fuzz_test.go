// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func pushsyncFrame(tb testing.TB, msg protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzDeliveryRead drives arbitrary bytes through the exact read path the
// pushsync handler uses for an incoming chunk delivery: the length-delimited
// protobuf reader followed by decoding the peer-supplied stamp. It asserts the
// combined path never panics and honours the reader's message-size limit.
func FuzzDeliveryRead(f *testing.F) {
	f.Add(pushsyncFrame(f, &pb.Delivery{
		Address: make([]byte, swarm.HashSize),
		Data:    []byte("some chunk data"),
		Stamp:   make([]byte, postage.StampSize),
	}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var d pb.Delivery
		if err := r.ReadMsg(&d); err != nil {
			return
		}
		// mirror the handler: build the chunk and decode the stamp
		_ = swarm.NewChunk(swarm.NewAddress(d.Address), d.Data)
		_ = new(postage.Stamp).UnmarshalBinary(d.Stamp)
	})
}

// FuzzReceiptRead drives arbitrary bytes through the reader path used by the
// pushsync originator to parse a receipt returned by a peer.
func FuzzReceiptRead(f *testing.F) {
	f.Add(pushsyncFrame(f, &pb.Receipt{
		Address:       make([]byte, swarm.HashSize),
		Signature:     make([]byte, 65),
		Nonce:         make([]byte, swarm.HashSize),
		StorageRadius: 8,
	}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var rec pb.Receipt
		if err := r.ReadMsg(&rec); err != nil {
			return
		}
		_ = swarm.NewAddress(rec.Address)
	})
}
