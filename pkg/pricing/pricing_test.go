// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/pricing/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

type testThresholdObserver struct {
	called           bool
	peer             swarm.Address
	paymentThreshold *big.Int
}

func (t *testThresholdObserver) NotifyPaymentThreshold(peerAddr swarm.Address, paymentThreshold *big.Int) error {
	t.called = true
	t.peer = peerAddr
	t.paymentThreshold = paymentThreshold
	return nil
}

func TestAnnouncePaymentThreshold(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	testThreshold := big.NewInt(100000)
	observer := &testThresholdObserver{}

	recipient := pricing.New(nil, logger, testThreshold)
	recipient.SetPaymentThresholdObserver(observer)

	peerID := swarm.MustParseHexAddress("9ee7add7")

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	payer := pricing.New(recorder, logger, testThreshold)

	paymentThreshold := big.NewInt(10000)

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

	sentPaymentThreshold := big.NewInt(0).SetBytes(messages[0].(*pb.AnnouncePaymentThreshold).PaymentThreshold)
	if sentPaymentThreshold.Cmp(paymentThreshold) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentPaymentThreshold, paymentThreshold)
	}

	if !observer.called {
		t.Fatal("expected observer to be called")
	}

	if observer.paymentThreshold.Cmp(paymentThreshold) != 0 {
		t.Fatalf("observer called with wrong paymentThreshold. got %d, want %d", observer.paymentThreshold, paymentThreshold)
	}

	if !observer.peer.Equal(peerID) {
		t.Fatalf("observer called with wrong peer. got %v, want %v", observer.peer, peerID)
	}
}
