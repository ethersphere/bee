// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pricing/pb"
)

// FuzzAnnouncePaymentThresholdRead drives arbitrary bytes through the read path
// the pricing handler uses to parse a peer's payment-threshold announcement,
// including the peer-controlled PaymentThreshold → big.Int conversion and the
// minimum-threshold comparison the handler performs on it.
func FuzzAnnouncePaymentThresholdRead(f *testing.F) {
	minThreshold := big.NewInt(1000)

	writeFrame := func(tb testing.TB, msg protobuf.Message) []byte {
		tb.Helper()
		var buf bytes.Buffer
		if err := protobuf.NewWriter(&buf).WriteMsg(msg); err != nil {
			tb.Fatal(err)
		}
		return buf.Bytes()
	}

	f.Add(writeFrame(f, &pb.AnnouncePaymentThreshold{PaymentThreshold: big.NewInt(100_000).Bytes()}))
	f.Add(writeFrame(f, &pb.AnnouncePaymentThreshold{PaymentThreshold: nil}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := protobuf.NewReader(bytes.NewReader(data))
		var req pb.AnnouncePaymentThreshold
		if err := r.ReadMsg(&req); err != nil {
			return
		}
		// mirror the handler's threshold handling
		threshold := new(big.Int).SetBytes(req.PaymentThreshold)
		if threshold.Sign() < 0 {
			t.Fatalf("SetBytes produced a negative threshold: %s", threshold)
		}
		_ = threshold.Cmp(minThreshold)
	})
}
