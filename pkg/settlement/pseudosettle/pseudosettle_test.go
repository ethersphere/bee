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
var testRefreshRateLight = int64(1000)

func testCaseNotAccepted(t *testing.T, recorder *streamtest.Recorder, observer *testObserver, payer, recipient *pseudosettle.Service, peerID swarm.Address, payerTime, recipientTime int64, recordsLength int, debtAmount, amount *big.Int, expectedError error) {

	payer.SetTime(payerTime)
	recipient.SetTime(recipientTime)
	observer.setPeerDebt(peerID, debtAmount)

	_, _, err := payer.Pay(context.Background(), peerID, amount, amount)
	if !errors.Is(err, expectedError) {
		t.Fatalf("expected error %v, got %v", expectedError, err)
	}

	records, err := recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != recordsLength {
		t.Fatalf("got %v records, want %v", l, recordsLength)
	}

	select {
	case <-observer.receivedCalled:
		t.Fatal("unexpected observer to be called")

	case <-time.After(1 * time.Second):

	}
}

func testCaseAccepted(t *testing.T, recorder *streamtest.Recorder, observer *testObserver, payer, recipient *pseudosettle.Service, peerID swarm.Address, payerTime, recipientTime int64, recordsLength, msgLength, recMsgLength int, debtAmount, amount, accepted, totalAmount *big.Int) {

	payer.SetTime(payerTime)
	recipient.SetTime(recipientTime)

	// set debt shown by accounting (observer)
	observer.setPeerDebt(peerID, debtAmount)

	acceptedAmount, _, err := payer.Pay(context.Background(), peerID, amount, amount)
	if err != nil {
		t.Fatal(err)
	}

	if acceptedAmount.Cmp(accepted) != 0 {
		t.Fatalf("expected amount not accepted. wanted %d, got %d", acceptedAmount, accepted)
	}

	records, err := recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != recordsLength {
		t.Fatalf("got %v records, want %v", l, recordsLength)
	}

	record := records[recordsLength-1]

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

	if len(messages) != msgLength || len(receivedMessages) != recMsgLength {
		t.Fatalf("got %v/%v messages, want %v/%v", len(messages), len(receivedMessages), msgLength, recMsgLength)
	}

	sentAmount := big.NewInt(0).SetBytes(messages[0].(*pb.Payment).Amount)
	receivedAmount := big.NewInt(0).SetBytes(receivedMessages[0].(*pb.PaymentAck).Amount)
	if sentAmount.Cmp(amount) != 0 {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	if receivedAmount.Cmp(accepted) != 0 {
		t.Fatalf("wrong settlement amount, got %v, want %v", receivedAmount, accepted)
	}

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(accepted) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, accepted)
		}

		if !call.peer.Equal(peerID) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peerID)
		}

	case <-time.After(1 * time.Second):
		t.Fatal("expected observer to be called")
	}

	totalSent, err := payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(totalAmount) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentAmount)
	}

	totalReceived, err := recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(totalAmount) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentAmount)
	}
}

func TestPayment(t *testing.T) {
	logger := logging.New(io.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID, FullNode: true}

	debt := int64(10000)

	observer := newTestObserver(map[string]*big.Int{peerID.String(): big.NewInt(debt)}, map[string]*big.Int{})
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer, big.NewInt(testRefreshRate), big.NewInt(testRefreshRateLight), mockp2p.New())
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
	payer := pseudosettle.New(recorder, logger, storePayer, observer2, big.NewInt(testRefreshRate), big.NewInt(testRefreshRateLight), mockp2p.New())
	payer.SetAccounting(observer2)
	// set time to non-zero, attempt payment based on debt, expect full amount to be accepted
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 30, 30, 1, 1, 1, big.NewInt(debt), big.NewInt(debt), big.NewInt(debt), big.NewInt(debt))
}

func TestTimeLimitedPayment(t *testing.T) {
	logger := logging.New(io.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID, FullNode: true}

	debt := testRefreshRate

	observer := newTestObserver(map[string]*big.Int{peerID.String(): big.NewInt(debt)}, map[string]*big.Int{})
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer, big.NewInt(testRefreshRate), big.NewInt(testRefreshRateLight), mockp2p.New())
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
	payer := pseudosettle.New(recorder, logger, storePayer, observer2, big.NewInt(testRefreshRate), big.NewInt(testRefreshRateLight), mockp2p.New())
	payer.SetAccounting(observer2)

	// Set time to 10000, attempt payment based on debt, expect full amount accepted
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10000, 10000, 1, 1, 1, big.NewInt(debt), big.NewInt(debt), big.NewInt(debt), big.NewInt(debt))

	// Set time 3 seconds later, attempt settlement below time based refreshment rate, expect full amount accepted
	sentSum := big.NewInt(debt + testRefreshRate*3/2)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10003, 10003, 2, 1, 1, big.NewInt(testRefreshRate*3/2), big.NewInt(testRefreshRate*3/2), big.NewInt(testRefreshRate*3/2), sentSum)

	// set time 1 seconds later, attempt settlement over the time-based allowed limit, expect partial amount accepted
	sentSum = big.NewInt(debt + testRefreshRate*3/2 + testRefreshRate)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10004, 10004, 3, 1, 1, big.NewInt(testRefreshRate*3), big.NewInt(testRefreshRate*3), big.NewInt(testRefreshRate), sentSum)

	// set time to same second as previous case, attempt settlement, expect error too soon
	testCaseNotAccepted(t, recorder, observer, payer, recipient, peerID, 10004, 10004, 3, big.NewInt(4*testRefreshRate), big.NewInt(4*testRefreshRate), pseudosettle.ErrSettlementTooSoon)

	// set time to same second as previous case on recipient, 1 second later on payer, attempt settlement, expect sent but failed
	testCaseNotAccepted(t, recorder, observer, payer, recipient, peerID, 10005, 10004, 4, big.NewInt(2*testRefreshRate), big.NewInt(2*testRefreshRate), io.EOF)

	// set time 6 seconds later, attempt with debt over time based allowance, expect partial accept
	sentSum = big.NewInt(debt + testRefreshRate*3/2 + testRefreshRate + 6*testRefreshRate)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10010, 10010, 5, 1, 1, big.NewInt(9*testRefreshRate), big.NewInt(9*testRefreshRate), big.NewInt(6*testRefreshRate), sentSum)

	// set time 10 seconds later, attempt with debt below time based allowance, expect full amount accepted
	sentSum = big.NewInt(debt + testRefreshRate*3/2 + testRefreshRate + 6*testRefreshRate + 5*testRefreshRate)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10020, 10020, 6, 1, 1, big.NewInt(5*testRefreshRate), big.NewInt(5*testRefreshRate), big.NewInt(5*testRefreshRate), sentSum)
}

func TestTimeLimitedPaymentLight(t *testing.T) {
	logger := logging.New(io.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	peer := p2p.Peer{Address: peerID, FullNode: false}

	debt := testRefreshRate

	observer := newTestObserver(map[string]*big.Int{peerID.String(): big.NewInt(debt)}, map[string]*big.Int{})
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer, big.NewInt(testRefreshRate), big.NewInt(testRefreshRateLight), mockp2p.New())
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
	payer := pseudosettle.New(recorder, logger, storePayer, observer2, big.NewInt(testRefreshRateLight), big.NewInt(testRefreshRateLight), mockp2p.New())
	payer.SetAccounting(observer2)

	// Set time to 10000, attempt payment based on debt, expect full amount accepted
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10000, 10000, 1, 1, 1, big.NewInt(debt), big.NewInt(debt), big.NewInt(debt), big.NewInt(debt))
	// Set time 3 seconds later, attempt settlement below time based light refreshment rate, expect full amount accepted
	sentSum := big.NewInt(debt + testRefreshRateLight*3)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10003, 10003, 2, 1, 1, big.NewInt(testRefreshRate*3/2), big.NewInt(testRefreshRate*3/2), big.NewInt(testRefreshRateLight*3), sentSum)
	// set time 1 seconds later, attempt settlement over the time-based light allowed limit, expect partial amount accepted
	sentSum = big.NewInt(debt + testRefreshRateLight*3 + testRefreshRateLight)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10004, 10004, 3, 1, 1, big.NewInt(testRefreshRate*3), big.NewInt(testRefreshRate*3), big.NewInt(testRefreshRateLight), sentSum)
	// set time to same second as previous case, attempt settlement, expect error too soon
	testCaseNotAccepted(t, recorder, observer, payer, recipient, peerID, 10004, 10004, 3, big.NewInt(4*testRefreshRate), big.NewInt(4*testRefreshRate), pseudosettle.ErrSettlementTooSoon)
	// set time to same second as previous case on recipient, 1 second later on payer, attempt settlement, expect sent but failed
	testCaseNotAccepted(t, recorder, observer, payer, recipient, peerID, 10005, 10004, 4, big.NewInt(2*testRefreshRate), big.NewInt(2*testRefreshRate), io.EOF)
	// set time 6 seconds later, attempt with debt over time based light allowance, expect partial accept
	sentSum = big.NewInt(debt + testRefreshRateLight*3 + testRefreshRateLight + 6*testRefreshRateLight)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10010, 10010, 5, 1, 1, big.NewInt(9*testRefreshRate), big.NewInt(9*testRefreshRate), big.NewInt(6*testRefreshRateLight), sentSum)
	// set time 100 seconds later, attempt with debt below time based allowance, expect full amount accepted
	sentSum = big.NewInt(debt + testRefreshRateLight*3 + testRefreshRateLight + 6*testRefreshRateLight + 50*testRefreshRateLight)
	testCaseAccepted(t, recorder, observer, payer, recipient, peerID, 10110, 10110, 6, 1, 1, big.NewInt(50*testRefreshRateLight), big.NewInt(50*testRefreshRateLight), big.NewInt(50*testRefreshRateLight), sentSum)
}
