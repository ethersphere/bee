// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle/pb"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type testObserver struct {
	called chan struct{}
	peer   swarm.Address
	amount *big.Int
}

func newTestObserver() *testObserver {
	return &testObserver{
		called: make(chan struct{}),
	}
}

func (t *testObserver) PeerDebt(peer swarm.Address) (*big.Int, error) {
	return nil, nil
}
func (t *testObserver) NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error {
	close(t.called)
	t.peer = peer
	t.amount = amount
	return nil
}

func (t *testObserver) NotifyPaymentSent(peer swarm.Address, amount *big.Int, err error) {}

func TestPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	observer := newTestObserver()
	recipient := pseudosettle.New(nil, logger, storeRecipient, observer)

	peerID := swarm.MustParseHexAddress("9ee7add7")

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	storePayer := mock.NewStateStore()
	defer storePayer.Close()

	observer2 := newTestObserver()
	payer := pseudosettle.New(recorder, logger, storePayer, observer2)
	payer.SetAccountingAPI(observer2)

	amount := big.NewInt(10000)

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

	if len(messages) != 1 {
		t.Fatalf("got %v messages, want %v", len(messages), 1)
	}

	sentAmount := messages[0].(*pb.Payment).Amount
	if sentAmount != amount.Uint64() {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	select {
	case <-observer.called:
	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	if observer.amount.Cmp(amount) != 0 {
		t.Fatalf("observer called with wrong amount. got %d, want %d", observer.amount, amount)
	}

	if !observer.peer.Equal(peerID) {
		t.Fatalf("observer called with wrong peer. got %v, want %v", observer.peer, peerID)
	}

	totalSent, err := payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(new(big.Int).SetUint64(sentAmount)) != 0 {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentAmount)
	}

	totalReceived, err := recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived.Cmp(new(big.Int).SetUint64(sentAmount)) != 0 {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentAmount)
	}
}
