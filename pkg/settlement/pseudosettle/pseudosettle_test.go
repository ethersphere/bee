// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle_test

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle/pb"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type testObserver struct {
	receivedCalled chan notifyPaymentReceivedCall
	sentCalled     chan notifyPaymentSentCall
	peerDebts      map[string]*big.Int
}

type notifyPaymentReceivedCall struct {
	peer   swarm.Address
	amount *big.Int
}

type notifyPaymentSentCall struct {
	peer   swarm.Address
	amount *big.Int
	err    error
}

func newTestObserver(debtAmounts map[string]*big.Int) *testObserver {
	return &testObserver{
		receivedCalled: make(chan notifyPaymentReceivedCall, 1),
		sentCalled:     make(chan notifyPaymentSentCall, 1),
		peerDebts:      debtAmounts,
	}
}

func (t *testObserver) setPeerDebt(peer swarm.Address, debt *big.Int) {
	t.peerDebts[peer.String()] = debt
}

func (t *testObserver) PeerDebt(peer swarm.Address) (*big.Int, error) {
	if debt, ok := t.peerDebts[peer.String()]; ok {
		return debt, nil
	}

	return nil, errors.New("Peer not listed")
}

func (t *testObserver) NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error {
	t.receivedCalled <- notifyPaymentReceivedCall{
		peer:   peer,
		amount: amount,
	}
	return nil
}

func (t *testObserver) NotifyPaymentSent(peer swarm.Address, amount *big.Int, err error) {
	t.sentCalled <- notifyPaymentSentCall{
		peer:   peer,
		amount: amount,
		err:    err,
	}
}

var testRefreshRate = int64(10000)

func TestPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID}

	debt := int64(10000)

	observer := newTestObserver(map[string]*big.Int{peerID.String(): big.NewInt(debt)})
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer, big.NewInt(testRefreshRate))
	recipient.SetAccountingAPI(observer)
	recipient.Init(context.Background(), peer)

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	storePayer := mock.NewStateStore()
	defer storePayer.Close()

	observer2 := newTestObserver(map[string]*big.Int{})
	payer := pseudosettle.New(recorder, logger, storePayer, observer2, big.NewInt(testRefreshRate))
	payer.SetAccountingAPI(observer2)

	amount := big.NewInt(debt)

	payer.Pay(context.Background(), peerID, amount)

	records, err := recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	record := records[0]

	if err := record.Err(); err != nil {
		t.Fatalf("record error: %v", err)
	}

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Payment) },
	)
	if err != nil {
		t.Fatal(err)
	}

	receivedMessages, err := protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.PaymentAck) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 || len(receivedMessages) != 1 {
		t.Fatalf("got %v/%v messages, want %v/%v", len(messages), len(receivedMessages), 1, 1)
	}

	sentAmount := big.NewInt(0).SetBytes(messages[0].(*pb.Payment).Amount)
	receivedAmount := big.NewInt(0).SetBytes(receivedMessages[0].(*pb.PaymentAck).Amount)
	if sentAmount.Cmp(amount) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	if sentAmount.Cmp(receivedAmount) != 0 {
		t.Fatalf("wrong settlement amount, got %v, want %v", receivedAmount, sentAmount)
	}

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(amount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount.Cmp(amount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err != nil {
			t.Fatalf("observer called with error. got %v want nil", call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	totalSent, err := payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(sentAmount) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentAmount)
	}

	totalReceived, err := recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(sentAmount) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentAmount)
	}
}

func TestTimeLimitedPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID}

	debt := testRefreshRate

	observer := newTestObserver(map[string]*big.Int{peerID.String(): big.NewInt(debt)})
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer, big.NewInt(testRefreshRate))
	recipient.SetAccountingAPI(observer)
	recipient.Init(context.Background(), peer)

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	storePayer := mock.NewStateStore()
	defer storePayer.Close()

	observer2 := newTestObserver(map[string]*big.Int{})
	payer := pseudosettle.New(recorder, logger, storePayer, observer2, big.NewInt(testRefreshRate))
	payer.SetAccountingAPI(observer2)

	payer.SetTime(int64(10000))
	recipient.SetTime(int64(10000))

	amount := big.NewInt(debt)

	payer.Pay(context.Background(), peerID, amount)

	records, err := recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	record := records[0]

	if err := record.Err(); err != nil {
		t.Fatalf("record error: %v", err)
	}

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Payment) },
	)
	if err != nil {
		t.Fatal(err)
	}

	receivedMessages, err := protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.PaymentAck) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 || len(receivedMessages) != 1 {
		t.Fatalf("got %v/%v messages, want %v/%v", len(messages), len(receivedMessages), 1, 1)
	}

	sentAmount := big.NewInt(0).SetBytes(messages[0].(*pb.Payment).Amount)
	receivedAmount := big.NewInt(0).SetBytes(receivedMessages[0].(*pb.PaymentAck).Amount)
	if sentAmount.Cmp(amount) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	if sentAmount.Cmp(receivedAmount) != 0 {
		t.Fatalf("wrong settlement amount, got %v, want %v", receivedAmount, sentAmount)
	}

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(amount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount.Cmp(amount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err != nil {
			t.Fatalf("observer called with error. got %v want nil", call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	totalSent, err := payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(sentAmount) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentAmount)
	}

	totalReceived, err := recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(sentAmount) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentAmount)
	}

	sentSum := big.NewInt(testRefreshRate)

	// Let 3 seconds pass, attempt settlement below time based refreshment rate

	debt = testRefreshRate * 3 / 2
	amount = big.NewInt(debt)

	payer.SetTime(int64(10003))
	recipient.SetTime(int64(10003))

	observer.setPeerDebt(peerID, amount)

	payer.Pay(context.Background(), peerID, amount)

	sentSum = sentSum.Add(sentSum, amount)

	records, err = recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 2 {
		t.Fatalf("got %v records, want %v", l, 2)
	}
	record = records[1]

	if err := record.Err(); err != nil {
		t.Fatalf("record error: %v", err)
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Payment) },
	)
	if err != nil {
		t.Fatal(err)
	}

	receivedMessages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.PaymentAck) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 || len(receivedMessages) != 1 {
		t.Fatalf("got %v/%v messages, want %v/%v", len(messages), len(receivedMessages), 1, 1)
	}

	sentAmount = big.NewInt(0).SetBytes(messages[0].(*pb.Payment).Amount)
	receivedAmount = big.NewInt(0).SetBytes(receivedMessages[0].(*pb.PaymentAck).Amount)
	if sentAmount.Cmp(amount) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	if sentAmount.Cmp(receivedAmount) != 0 {
		t.Fatalf("wrong settlement amount, got %v, want %v", receivedAmount, sentAmount)
	}

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(receivedAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount.Cmp(receivedAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err != nil {
			t.Fatalf("observer called with error. got %v want nil", call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	totalSent, err = payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentSum)
	}

	totalReceived, err = recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentSum)
	}

	// attempt settlement over the time-based allowed limit 1 seconds later

	debt = 3 * testRefreshRate
	amount = big.NewInt(debt)

	payer.SetTime(int64(10004))
	recipient.SetTime(int64(10004))

	observer.setPeerDebt(peerID, amount)

	payer.Pay(context.Background(), peerID, amount)

	testRefreshRateBigInt := big.NewInt(testRefreshRate)

	sentSum = sentSum.Add(sentSum, testRefreshRateBigInt)

	records, err = recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 3 {
		t.Fatalf("got %v records, want %v", l, 3)
	}

	record = records[2]

	if err := record.Err(); err != nil {
		t.Fatalf("record error: %v", err)
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Payment) },
	)
	if err != nil {
		t.Fatal(err)
	}

	receivedMessages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.PaymentAck) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 || len(receivedMessages) != 1 {
		t.Fatalf("got %v/%v messages, want %v/%v", len(messages), len(receivedMessages), 1, 1)
	}

	sentAmount = big.NewInt(0).SetBytes(messages[0].(*pb.Payment).Amount)
	receivedAmount = big.NewInt(0).SetBytes(receivedMessages[0].(*pb.PaymentAck).Amount)
	if sentAmount.Cmp(amount) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	if receivedAmount.Cmp(testRefreshRateBigInt) != 0 {
		t.Fatalf("wrong settlement amount, got %v, want %v", receivedAmount, testRefreshRateBigInt)
	}

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(testRefreshRateBigInt) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, testRefreshRate)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount.Cmp(testRefreshRateBigInt) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, testRefreshRate)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err != nil {
			t.Fatalf("observer called with error. got %v want nil", call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	totalSent, err = payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentSum)
	}

	totalReceived, err = recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentSum)
	}

	// attempt settle again in the same second without success

	debt = 4 * testRefreshRate
	amount = big.NewInt(debt)

	observer.setPeerDebt(peerID, amount)

	payer.Pay(context.Background(), peerID, amount)

	records, err = recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 3 {
		t.Fatalf("got %v records, want %v", l, 3)
	}

	select {
	case <-observer.receivedCalled:
		t.Fatal("unexpected observer to be called")

	case <-time.After(time.Second):

	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount != nil {
			t.Fatalf("observer called with wrong amount. got %d, want nil", call.amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err == nil {
			t.Fatalf("observer called without error. got nil want err")
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	// attempt again while recipient is still supposed to be blocking based on time

	debt = 2 * testRefreshRate
	amount = big.NewInt(debt)

	payer.SetTime(int64(10005))
	recipient.SetTime(int64(10004))

	observer.setPeerDebt(peerID, amount)

	payer.Pay(context.Background(), peerID, amount)

	records, err = recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 4 {
		t.Fatalf("got %v records, want %v", l, 4)
	}

	select {
	case <-observer.receivedCalled:
		t.Fatal("unexpected observer to be called")

	case <-time.After(time.Second):

	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount != nil {
			t.Fatalf("observer called with wrong amount. got %d, want nil", call.amount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err == nil {
			t.Fatalf("observer called without error. got nil want err")
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	// attempt multiple seconds later with debt over time based allowance

	debt = 9 * testRefreshRate
	amount = big.NewInt(debt)

	payer.SetTime(int64(10010))
	recipient.SetTime(int64(10010))

	observer.setPeerDebt(peerID, amount)

	payer.Pay(context.Background(), peerID, amount)

	sentSum = sentSum.Add(sentSum, big.NewInt(6*testRefreshRate))

	records, err = recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 5 {
		t.Fatalf("got %v records, want %v", l, 5)
	}

	record = records[4]

	if err := record.Err(); err != nil {
		t.Fatalf("record error: %v", err)
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Payment) },
	)
	if err != nil {
		t.Fatal(err)
	}

	receivedMessages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.PaymentAck) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 || len(receivedMessages) != 1 {
		t.Fatalf("got %v/%v messages, want %v/%v", len(messages), len(receivedMessages), 1, 1)
	}

	testAmount := big.NewInt(6 * testRefreshRate)

	sentAmount = big.NewInt(0).SetBytes(messages[0].(*pb.Payment).Amount)
	receivedAmount = big.NewInt(0).SetBytes(receivedMessages[0].(*pb.PaymentAck).Amount)
	if sentAmount.Cmp(amount) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	if receivedAmount.Cmp(testAmount) != 0 {
		t.Fatalf("wrong settlement amount, got %v, want %v", receivedAmount, testAmount)
	}

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(testAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, testAmount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount.Cmp(testAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, testAmount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err != nil {
			t.Fatalf("observer called with error. got %v want nil", call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	totalSent, err = payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentSum)
	}

	totalReceived, err = recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentSum)
	}

	// attempt further settlement with less outstanding debt than time allowance would allow

	debt = 5 * testRefreshRate
	amount = big.NewInt(debt)

	payer.SetTime(int64(10020))
	recipient.SetTime(int64(10020))

	observer.setPeerDebt(peerID, amount)

	payer.Pay(context.Background(), peerID, amount)

	sentSum = sentSum.Add(sentSum, big.NewInt(5*testRefreshRate))

	records, err = recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 6 {
		t.Fatalf("got %v records, want %v", l, 5)
	}

	record = records[5]

	if err := record.Err(); err != nil {
		t.Fatalf("record error: %v", err)
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Payment) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 {
		t.Fatalf("got %v messages, want %v", len(messages), 1)
	}

	testAmount = big.NewInt(5 * testRefreshRate)

	sentAmount = big.NewInt(0).SetBytes(messages[0].(*pb.Payment).Amount)
	if sentAmount.Cmp(testAmount) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentAmount, testAmount)
	}

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(testAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, testAmount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	select {
	case call := <-observer2.sentCalled:
		if call.amount.Cmp(testAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, testAmount)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}
		if call.err != nil {
			t.Fatalf("observer called with error. got %v want nil", call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	totalSent, err = payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentSum)
	}

	totalReceived, err = recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(sentSum) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentSum)
	}
}
