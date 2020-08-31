// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/accounting"
	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/mock"
)

const (
	fixedPrice = uint64(10)
)

// TestSendChunkAndGetReceipt inserts a chunk as uploaded chunk in db. This triggers sending a chunk to the closest node
// and expects a receipt. The message are intercepted in the outgoing stream to check for correctness.
func TestSendChunkAndReceiveReceipt(t *testing.T) {
	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")
	chunk := swarm.NewChunk(chunkAddress, chunkData)

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer, storerPeer, _, peerAccounting := createPushSyncNode(t, closestPeer, nil, nil, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer.Close()

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot, _, pivotAccounting := createPushSyncNode(t, pivotNode, recorder, nil, mock.WithClosestPeer(closestPeer))
	defer storerPivot.Close()

	// Trigger the sending of chunk to the closest node
	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// this intercepts the outgoing delivery message
	waitOnRecordAndTest(t, closestPeer, recorder, chunkAddress, chunkData)

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunkAddress, nil)

	balance, err := pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = peerAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != int64(fixedPrice) {
		t.Fatalf("unexpected balance on peer. want %d got %d", int64(fixedPrice), balance)
	}
}

// PushChunkToClosest tests the sending of chunk to closest peer from the origination source perspective.
// it also checks wether the tags are incremented properly if they are present
func TestPushChunkToClosest(t *testing.T) {
	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer, storerPeer, _, peerAccounting := createPushSyncNode(t, closestPeer, nil, nil, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer.Close()

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot, pivotTags, pivotAccounting := createPushSyncNode(t, pivotNode, recorder, nil, mock.WithClosestPeer(closestPeer))
	defer storerPivot.Close()

	ta, err := pivotTags.Create("test", 1)
	if err != nil {
		t.Fatal(err)
	}
	chunk := swarm.NewChunk(chunkAddress, chunkData).WithTagID(ta.Uid)

	ta1, err := pivotTags.Get(ta.Uid)
	if err != nil {
		t.Fatal(err)
	}

	if ta1.Get(tags.StateSent) != 0 || ta1.Get(tags.StateSynced) != 0 {
		t.Fatalf("tags initialization error")
	}

	// Trigger the sending of chunk to the closest node
	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// this intercepts the outgoing delivery message
	waitOnRecordAndTest(t, closestPeer, recorder, chunkAddress, chunkData)

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunkAddress, nil)

	ta2, err := pivotTags.Get(ta.Uid)
	if err != nil {
		t.Fatal(err)
	}
	if ta2.Get(tags.StateSent) != 1 {
		t.Fatalf("tags error")
	}

	balance, err := pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = peerAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != int64(fixedPrice) {
		t.Fatalf("unexpected balance on peer. want %d got %d", int64(fixedPrice), balance)
	}
}

// TestHandler expect a chunk from a node on a stream. It then stores the chunk in the local store and
// sends back a receipt. This is tested by intercepting the incoming stream for proper messages.
// It also sends the chunk to the closest peer and receives a receipt.
//
// Chunk moves from   TriggerPeer -> PivotPeer -> ClosestPeer
//
func TestHandler(t *testing.T) {
	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")
	chunk := swarm.NewChunk(chunkAddress, chunkData)

	// create a pivot node and a mocked closest node
	pivotPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	triggerPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("f000000000000000000000000000000000000000000000000000000000000000")

	// mock call back function to see if pss message is delivered when it is received in the destination (closestPeer in this testcase)
	hookWasCalled := make(chan bool, 1) // channel to check if hook is called
	pssDeliver := func(ctx context.Context, ch swarm.Chunk) error {
		hookWasCalled <- true
		return nil
	}

	// Create the closest peer
	psClosestPeer, closestStorerPeerDB, _, closestAccounting := createPushSyncNode(t, closestPeer, nil, pssDeliver, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer closestStorerPeerDB.Close()

	closestRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()))

	// creating the pivot peer
	psPivot, storerPivotDB, _, pivotAccounting := createPushSyncNode(t, pivotPeer, closestRecorder, nil, mock.WithClosestPeer(closestPeer))
	defer storerPivotDB.Close()

	pivotRecorder := streamtest.New(streamtest.WithProtocols(psPivot.Protocol()))

	// Creating the trigger peer
	psTriggerPeer, triggerStorerDB, _, triggerAccounting := createPushSyncNode(t, triggerPeer, pivotRecorder, nil, mock.WithClosestPeer(pivotPeer))
	defer triggerStorerDB.Close()

	receipt, err := psTriggerPeer.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// In pivot peer,  intercept the incoming delivery chunk from the trigger peer and check for correctness
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunkAddress, chunkData)

	// Pivot peer will forward the chunk to its closest peer. Intercept the incoming stream from pivot node and check
	// for the correctness of the chunk
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunkAddress, chunkData)

	// Similarly intercept the same incoming stream to see if the closest peer is sending a proper receipt
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunkAddress, nil)

	// In the received stream, check if a receipt is sent from pivot peer and check for its correctness.
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunkAddress, nil)

	// check if the pss delivery hook is called
	select {
	case <-hookWasCalled:
		break
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recovery hook was not called")
	}

	balance, err := triggerAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on trigger. want %d got %d", -int64(fixedPrice), balance)
	}

	// we need to check here for pivotPeer instead of triggerPeer because during streamtest the peer in the handler is actually the receiver
	balance, err = pivotAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", int64(fixedPrice), balance)
	}

	balance, err = pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = closestAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance != int64(fixedPrice) {
		t.Fatalf("unexpected balance on closest. want %d got %d", int64(fixedPrice), balance)
	}
}

func createPushSyncNode(t *testing.T, addr swarm.Address, recorder *streamtest.Recorder, pssDeliver func(context.Context, swarm.Chunk) error, mockOpts ...mock.Option) (*pushsync.PushSync, *localstore.DB, *tags.Tags, accounting.Interface) {
	logger := logging.New(ioutil.Discard, 0)

	storer, err := localstore.New("", addr.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	mockTopology := mock.NewTopologyDriver(mockOpts...)
	mockStatestore := statestore.NewStateStore()
	mtag := tags.NewTags(mockStatestore, logger)

	mockAccounting := accountingmock.NewAccounting()
	mockPricer := accountingmock.NewPricer(fixedPrice, fixedPrice)

	return pushsync.New(recorder, storer, mockTopology, mtag, pssDeliver, logger, mockAccounting, mockPricer), storer, mtag, mockAccounting
}

func waitOnRecordAndTest(t *testing.T, peer swarm.Address, recorder *streamtest.Recorder, add swarm.Address, data []byte) {
	t.Helper()
	records := recorder.WaitRecords(t, peer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)

	if data != nil {
		messages, err := protobuf.ReadMessages(
			bytes.NewReader(records[0].In()),
			func() protobuf.Message { return new(pb.Delivery) },
		)
		if err != nil {
			t.Fatal(err)
		}
		if messages == nil {
			t.Fatal("nil rcvd. for message")
		}
		if len(messages) > 1 {
			t.Fatal("too many messages")
		}
		delivery := messages[0].(*pb.Delivery)

		if !bytes.Equal(delivery.Address, add.Bytes()) {
			t.Fatalf("chunk address mismatch")
		}

		if !bytes.Equal(delivery.Data, data) {
			t.Fatalf("chunk data mismatch")
		}
	} else {
		messages, err := protobuf.ReadMessages(
			bytes.NewReader(records[0].In()),
			func() protobuf.Message { return new(pb.Receipt) },
		)
		if err != nil {
			t.Fatal(err)
		}
		if messages == nil {
			t.Fatal("nil rcvd. for message")
		}
		if len(messages) > 1 {
			t.Fatal("too many messages")
		}
		receipt := messages[0].(*pb.Receipt)
		receiptAddress := swarm.NewAddress(receipt.Address)

		if !receiptAddress.Equal(add) {
			t.Fatalf("receipt address mismatch")
		}
	}
}
