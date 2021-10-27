// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	mockp2p "github.com/ethersphere/bee/pkg/p2p/mock"
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

func newTestObserver(debtAmounts, shadowBalanceAmounts map[string]*big.Int) *testObserver {
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

func (t *testObserver) Connect(peer swarm.Address, full bool) {

}

func (t *testObserver) Disconnect(peer swarm.Address) {

}

func (t *testObserver) NotifyRefreshmentReceived(peer swarm.Address, amount *big.Int) error {
	t.receivedCalled <- notifyPaymentReceivedCall{
		peer:   peer,
		amount: amount,
	}
	return nil
}

func (t *testObserver) NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error {
	return nil
}

func (t *testObserver) NotifyPaymentSent(peer swarm.Address, amount *big.Int, err error) {
	t.sentCalled <- notifyPaymentSentCall{
		peer:   peer,
		amount: amount,
		err:    err,
	}
}

func (t *testObserver) Reserve(ctx context.Context, peer swarm.Address, amount uint64) error {
	return nil
}

func (t *testObserver) Release(peer swarm.Address, amount uint64) {
}

var testRefreshRate = int64(10000)

func TestPayment(t *testing.T) {
	logger := logging.New(io.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID}

	debt := int64(10000)

	observer := newTestObserver(map[string]*big.Int{peerID.String(): big.NewInt(debt)}, map[string]*big.Int{})
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer, big.NewInt(testRefreshRate), mockp2p.New())
	recipient.SetAccounting(observer)
	err := recipient.Init(context.Background(), peer)
	if err != nil {
		t.Fatal(err)
	}

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	storePayer := mock.NewStateStore()
	defer storePayer.Close()

	observer2 := newTestObserver(map[string]*big.Int{}, map[string]*big.Int{peerID.String(): big.NewInt(debt)})
	payer := pseudosettle.New(recorder, logger, storePayer, observer2, big.NewInt(testRefreshRate), mockp2p.New())
	payer.SetAccounting(observer2)

	amount := big.NewInt(debt)

	acceptedAmount, _, err := payer.Pay(context.Background(), peerID, amount, amount)
	if err != nil {
		t.Fatal(err)
	}

	if acceptedAmount.Cmp(amount) != 0 {
		t.Fatalf("full amount not accepted. wanted %d, got %d", amount, acceptedAmount)
	}

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
	logger := logging.New(io.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID}

	debt := testRefreshRate

	observer := newTestObserver(map[string]*big.Int{peerID.String(): big.NewInt(debt)}, map[string]*big.Int{})
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer, big.NewInt(testRefreshRate), mockp2p.New())
	recipient.SetAccounting(observer)
	err := recipient.Init(context.Background(), peer)
	if err != nil {
		t.Fatal(err)
	}

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	storePayer := mock.NewStateStore()
	defer storePayer.Close()

	observer2 := newTestObserver(map[string]*big.Int{}, map[string]*big.Int{peerID.String(): big.NewInt(debt)})
	payer := pseudosettle.New(recorder, logger, storePayer, observer2, big.NewInt(testRefreshRate), mockp2p.New())
	payer.SetAccounting(observer2)

	payer.SetTime(int64(10000))
	recipient.SetTime(int64(10000))

	amount := big.NewInt(debt)

	acceptedAmount, _, err := payer.Pay(context.Background(), peerID, amount, amount)
	if err != nil {
		t.Fatal(err)
	}

	if acceptedAmount.Cmp(amount) != 0 {
		t.Fatalf("full amount not accepted. wanted %d, got %d", amount, acceptedAmount)
	}

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

	acceptedAmount, _, err = payer.Pay(context.Background(), peerID, amount, amount)
	if err != nil {
		t.Fatal(err)
	}

	if acceptedAmount.Cmp(amount) != 0 {
		t.Fatalf("full amount not accepted. wanted %d, got %d", amount, acceptedAmount)
	}

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

	acceptedAmount, _, err = payer.Pay(context.Background(), peerID, amount, amount)
	if err != nil {
		t.Fatal(err)
	}

	testRefreshRateBigInt := big.NewInt(testRefreshRate)
	if acceptedAmount.Cmp(testRefreshRateBigInt) != 0 {
		t.Fatalf("full amount not accepted. wanted %d, got %d", amount, testRefreshRateBigInt)
	}

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

	_, _, err = payer.Pay(context.Background(), peerID, amount, amount)
	if !errors.Is(err, pseudosettle.ErrSettlementTooSoon) {
		t.Fatal("sent settlement too soon")
	}

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

	// attempt again while recipient is still supposed to be blocking based on time

	debt = 2 * testRefreshRate
	amount = big.NewInt(debt)

	payer.SetTime(int64(10005))
	recipient.SetTime(int64(10004))

	observer.setPeerDebt(peerID, amount)

	_, _, err = payer.Pay(context.Background(), peerID, amount, amount)
	if err == nil {
		t.Fatal("expected error")
	}

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

	// attempt multiple seconds later with debt over time based allowance

	debt = 9 * testRefreshRate
	amount = big.NewInt(debt)

	payer.SetTime(int64(10010))
	recipient.SetTime(int64(10010))

	observer.setPeerDebt(peerID, amount)

	acceptedAmount, _, err = payer.Pay(context.Background(), peerID, amount, amount)
	if err != nil {
		t.Fatal(err)
	}

	testAmount := big.NewInt(6 * testRefreshRate)

	if acceptedAmount.Cmp(testAmount) != 0 {
		t.Fatalf("incorrect amount accepted. wanted %d, got %d", testAmount, acceptedAmount)
	}

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

	acceptedAmount, _, err = payer.Pay(context.Background(), peerID, amount, amount)
	if err != nil {
		t.Fatal(err)
	}

	testAmount = big.NewInt(5 * testRefreshRate)

	if acceptedAmount.Cmp(testAmount) != 0 {
		t.Fatalf("incorrect amount accepted. wanted %d, got %d", testAmount, acceptedAmount)
	}

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
