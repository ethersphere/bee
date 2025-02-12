// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing_test

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/pricing"
	"github.com/ethersphere/bee/v2/pkg/pricing/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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
	t.Parallel()

	logger := log.Noop
	testThreshold := big.NewInt(100000)
	testLightThreshold := big.NewInt(10000)

	observer := &testThresholdObserver{}

	recipient := pricing.New(nil, logger, testThreshold, testLightThreshold, big.NewInt(1000))
	recipient.SetPaymentThresholdObserver(observer)

	peerID := swarm.MustParseHexAddress("9ee7add7")

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	payer := pricing.New(recorder, logger, testThreshold, testLightThreshold, big.NewInt(1000))

	paymentThreshold := big.NewInt(100000)

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
		t.Fatalf("observer called with wrong paymentThreshold. got %v, want %v", observer.paymentThreshold, paymentThreshold)
	}

	if !observer.peer.Equal(peerID) {
		t.Fatalf("observer called with wrong peer. got %v, want %v", observer.peer, peerID)
	}
}

func TestAnnouncePaymentWithInsufficientThreshold(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	testThreshold := big.NewInt(100_000)
	testLightThreshold := big.NewInt(10_000)

	observer := &testThresholdObserver{}

	minThreshold := big.NewInt(1_000_000) // above requested threshold

	recipient := pricing.New(nil, logger, testThreshold, testLightThreshold, minThreshold)
	recipient.SetPaymentThresholdObserver(observer)

	peerID := swarm.MustParseHexAddress("9ee7add7")

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	payer := pricing.New(recorder, logger, testThreshold, testLightThreshold, minThreshold)

	paymentThreshold := big.NewInt(100_000)

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

	if record.Err() == nil {
		t.Fatal("expected error")
	}

	disconnectErr := &p2p.DisconnectError{}
	if !errors.As(record.Err(), &disconnectErr) {
		t.Fatalf("wanted %v, got %v", disconnectErr, record.Err())
	}

	if !errors.Is(record.Err(), pricing.ErrThresholdTooLow) {
		t.Fatalf("wanted error %v, got %v", pricing.ErrThresholdTooLow, record.Err())
	}

	if observer.called {
		t.Fatal("unexpected call to the observer")
	}
}

func TestInitialPaymentThreshold(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	testThreshold := big.NewInt(100000)
	testLightThreshold := big.NewInt(10000)

	observer := &testThresholdObserver{}

	recipient := pricing.New(nil, logger, testThreshold, testLightThreshold, big.NewInt(1000))
	recipient.SetPaymentThresholdObserver(observer)

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID, FullNode: true}

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	payer := pricing.New(recorder, logger, testThreshold, testLightThreshold, big.NewInt(1000))

	err := payer.Init(context.Background(), peer)
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
	if sentPaymentThreshold.Cmp(testThreshold) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentPaymentThreshold, testThreshold)
	}

	if !observer.called {
		t.Fatal("expected observer to be called")
	}

	if observer.paymentThreshold.Cmp(testThreshold) != 0 {
		t.Fatalf("observer called with wrong paymentThreshold, got %v, want %v", observer.paymentThreshold, testThreshold)
	}

	if !observer.peer.Equal(peerID) {
		t.Fatalf("observer called with wrong peer, got %v, want %v", observer.peer, peerID)
	}
}

func TestInitialPaymentThresholdLightNode(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	testThreshold := big.NewInt(100000)
	testLightThreshold := big.NewInt(10000)

	observer := &testThresholdObserver{}

	recipient := pricing.New(nil, logger, testThreshold, testLightThreshold, big.NewInt(1000))
	recipient.SetPaymentThresholdObserver(observer)

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID, FullNode: false}

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	payer := pricing.New(recorder, logger, testThreshold, testLightThreshold, big.NewInt(1000))

	err := payer.Init(context.Background(), peer)
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
	if sentPaymentThreshold.Cmp(testLightThreshold) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentPaymentThreshold, testLightThreshold)
	}

	if !observer.called {
		t.Fatal("expected observer to be called")
	}

	if observer.paymentThreshold.Cmp(testLightThreshold) != 0 {
		t.Fatalf("observer called with wrong paymentThreshold, got %v, want %v", observer.paymentThreshold, testLightThreshold)
	}

	if !observer.peer.Equal(peerID) {
		t.Fatalf("observer called with wrong peer, got %v, want %v", observer.peer, peerID)
	}
}
