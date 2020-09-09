// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudosettle_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle"
	"github.com/ethersphere/bee/pkg/settlement/pseudosettle/pb"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type testObserver struct {
	called bool
	peer   swarm.Address
	amount uint64
}

func (t *testObserver) NotifyPayment(peer swarm.Address, amount uint64) error {
	t.called = true
	t.peer = peer
	t.amount = amount
	return nil
}

func TestPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	storeRecipient := mock.NewStateStore()
	defer storeRecipient.Close()

	observer := &testObserver{}
	recipient := pseudosettle.New(nil, logger, storeRecipient)
	recipient.SetPaymentObserver(observer)

	recorder := streamtest.New(
		streamtest.WithProtocols(recipient.Protocol()),
	)

	storePayer := mock.NewStateStore()
	defer storePayer.Close()

	payer := pseudosettle.New(recorder, logger, storePayer)

	peerID := swarm.MustParseHexAddress("9ee7add7")
	amount := uint64(10000)

	err := payer.Pay(context.Background(), peerID, amount)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(peerID, "pseudosettle", "1.0.0", "pseudosettle")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	record := records[0]

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
	if sentAmount != amount {
		t.Fatalf("got message with amount %v, want %v", sentAmount, amount)
	}

	if !observer.called {
		t.Fatal("expected observer to be called")
	}

	if observer.amount != amount {
		t.Fatalf("observer called with wrong amount. got %d, want %d", observer.amount, amount)
	}

	if !observer.peer.Equal(peerID) {
		t.Fatalf("observer called with wrong peer. got %v, want %v", observer.peer, peerID)
	}

	totalSent, err := payer.TotalSent(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent != sentAmount {
		t.Fatalf("stored wrong totalSent. got %d, want %d", totalSent, sentAmount)
	}

	totalReceived, err := recipient.TotalReceived(peerID)
	if err != nil {
		t.Fatal(err)
	}

	if totalReceived != sentAmount {
		t.Fatalf("stored wrong totalReceived. got %d, want %d", totalReceived, sentAmount)
	}
}
