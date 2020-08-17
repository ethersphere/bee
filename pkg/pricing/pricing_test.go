// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/pricing/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestAnnouncePaymentThreshold(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	recipient := pricing.New(pricing.Options{
		Logger: logger,
	})

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
	)

	payer := pricing.New(pricing.Options{
		Streamer: recorder,
		Logger:   logger,
	})

	peerID := swarm.MustParseHexAddress("9ee7add7")
	paymentThreshold := uint64(10000)

	err := payer.AnnouncePaymentThreshold(context.Background(), peerID, paymentThreshold)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(peerID, "pricing", "1.0.0", "pricing")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	record := records[0]

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.AnnouncePaymentThreshold) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 {
		t.Fatalf("got %v messages, want %v", len(messages), 1)
	}

	sentPaymentThreshold := messages[0].(*pb.AnnouncePaymentThreshold).PaymentThreshold
	if sentPaymentThreshold != paymentThreshold {
		t.Fatalf("got message with amount %v, want %v", sentPaymentThreshold, paymentThreshold)
	}
}
