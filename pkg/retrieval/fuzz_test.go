// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	pb "github.com/ethersphere/bee/v2/pkg/retrieval/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func retrievalFrame(tb testing.TB, msg protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzRequestRead drives arbitrary bytes through the read path the retrieval
// handler uses to parse an incoming chunk request, mirroring its address
// construction and validity check.
func FuzzRequestRead(f *testing.F) {
	f.Add(retrievalFrame(f, &pb.Request{Addr: make([]byte, swarm.HashSize)}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var req pb.Request
		if err := r.ReadMsg(&req); err != nil {
			return
		}
		addr := swarm.NewAddress(req.Addr)
		_ = addr.IsZero()
		_ = addr.IsEmpty()
		_ = addr.IsValidLength()
	})
}

// FuzzDeliveryRead drives arbitrary bytes through the read path the retrieval
// client uses to parse a delivery returned by a peer.
func FuzzDeliveryRead(f *testing.F) {
	f.Add(retrievalFrame(f, &pb.Delivery{Data: []byte("chunk data")}))
	f.Add(retrievalFrame(f, &pb.Delivery{Err: "not found"}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var d pb.Delivery
		if err := r.ReadMsg(&d); err != nil {
			return
		}
		_ = d.Err
		_ = d.Data
	})
}
