// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pricing_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	pricermock "github.com/ethersphere/bee/pkg/pricer/mock"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/pricing/pb"
	"github.com/ethersphere/bee/pkg/swarm"
)

type testThresholdObserver struct {
	called           bool
	peer             swarm.Address
	paymentThreshold *big.Int
}

type testPriceTableObserver struct {
	called     bool
	peer       swarm.Address
	priceTable []uint64
}

func (t *testThresholdObserver) NotifyPaymentThreshold(peerAddr swarm.Address, paymentThreshold *big.Int) error {
	t.called = true
	t.peer = peerAddr
	t.paymentThreshold = paymentThreshold
	return nil
}

func (t *testPriceTableObserver) NotifyPriceTable(peerAddr swarm.Address, priceTable []uint64) error {
	t.called = true
	t.peer = peerAddr
	t.priceTable = priceTable
	return nil
}

func TestAnnouncePaymentThreshold(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	testThreshold := big.NewInt(100000)
	observer := &testThresholdObserver{}

	pricerMockService := pricermock.NewMockService()

	recipient := pricing.New(nil, logger, testThreshold, pricerMockService)
	recipient.SetPaymentThresholdObserver(observer)

	peerID := swarm.MustParseHexAddress("9ee7add7")

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	payer := pricing.New(recorder, logger, testThreshold, pricerMockService)

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

func TestAnnouncePaymentThresholdAndPriceTable(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	testThreshold := big.NewInt(100000)
	observer1 := &testThresholdObserver{}
	observer2 := &testPriceTableObserver{}

	table := []uint64{50, 25, 12, 6}
	priceTableFunc := func() []uint64 {
		return table
	}

	pricerMockService := pricermock.NewMockService(pricermock.WithPriceTableFunc(priceTableFunc))

	recipient := pricing.New(nil, logger, testThreshold, pricerMockService)
	recipient.SetPaymentThresholdObserver(observer1)
	recipient.SetPriceTableObserver(observer2)
	peerID := swarm.MustParseHexAddress("9ee7add7")

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	payer := pricing.New(recorder, logger, testThreshold, pricerMockService)

	paymentThreshold := big.NewInt(10000)

	err := payer.AnnouncePaymentThresholdAndPriceTable(context.Background(), peerID, paymentThreshold)
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

	sentPriceTable := messages[0].(*pb.AnnouncePaymentThreshold).ProximityPrice
	if !reflect.DeepEqual(sentPriceTable, table) {
		t.Fatalf("got message with table %v, want %v", sentPriceTable, table)
	}

	if !observer1.called {
		t.Fatal("expected threshold observer to be called")
	}

	if observer1.paymentThreshold.Cmp(paymentThreshold) != 0 {
		t.Fatalf("observer called with wrong paymentThreshold. got %d, want %d", observer1.paymentThreshold, paymentThreshold)
	}

	if !observer1.peer.Equal(peerID) {
		t.Fatalf("threshold observer called with wrong peer. got %v, want %v", observer1.peer, peerID)
	}

	if !observer2.called {
		t.Fatal("expected table observer to be called")
	}

	if !reflect.DeepEqual(observer2.priceTable, table) {
		t.Fatalf("table observer called with wrong priceTable. got %d, want %d", observer2.priceTable, table)
	}

	if !observer2.peer.Equal(peerID) {
		t.Fatalf("table observer called with wrong peer. got %v, want %v", observer2.peer, peerID)
	}
}
