// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/settlement/pseudosettle/pb"
)

func pseudosettleFrame(tb testing.TB, msg protobuf.Message) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// FuzzPaymentRead drives arbitrary bytes through the read path the pseudosettle
// handler uses to parse an incoming payment, including the peer-controlled
// Amount → big.Int conversion the handler performs before accounting.
func FuzzPaymentRead(f *testing.F) {
	f.Add(pseudosettleFrame(f, &pb.Payment{Amount: big.NewInt(1_000_000).Bytes()}))
	f.Add(pseudosettleFrame(f, &pb.Payment{Amount: nil}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var req pb.Payment
		if err := r.ReadMsg(&req); err != nil {
			return
		}
		// mirror the handler: the amount is an unsigned big.Int
		amount := new(big.Int).SetBytes(req.Amount)
		if amount.Sign() < 0 {
			t.Fatalf("SetBytes produced a negative amount: %s", amount)
		}
	})
}

// FuzzPaymentAckRead drives arbitrary bytes through the read path the
// pseudosettle payer uses to parse a payment acknowledgement from a peer.
func FuzzPaymentAckRead(f *testing.F) {
	f.Add(pseudosettleFrame(f, &pb.PaymentAck{Amount: big.NewInt(42).Bytes(), Timestamp: 123}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var ack pb.PaymentAck
		if err := r.ReadMsg(&ack); err != nil {
			return
		}
		_ = new(big.Int).SetBytes(ack.Amount)
	})
}
