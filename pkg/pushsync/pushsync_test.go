// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/mock"
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
	psPeer, storerPeer := createPushSyncNode(t, closestPeer, nil, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer.Close()

	recorder := streamtest.New(
		streamtest.WithProtocols(psPeer.Protocol()),
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return f
		}),
	)

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot := createPushSyncNode(t, pivotNode, recorder, mock.WithClosestPeer(closestPeer))
	defer storerPivot.Close()

	// Trigger the sending of chunk to the closest node
	receipt, err := psPivot.SendChunkAndReceiveReceipt(context.Background(), closestPeer, chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(swarm.NewAddress(receipt.Address)) {
		t.Fatal("invalid receipt")
	}

	// this intercepts the outgoing delivery message
	waitOnRecordAndTest(t, closestPeer, recorder, chunkAddress, chunkData)

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunkAddress, nil)

}

// TestHandler expect a chunk from a node on a stream. It then stores the chunk in the local store and
// sends back a receipt. This is tested by intercepting the incoming stream for proper messages.
// It also sends the chunk to the closest peerand receives a receipt.
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

	// Create the closest peer
	psClosestPeer, closestStorerPeerDB := createPushSyncNode(t, closestPeer, nil, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer closestStorerPeerDB.Close()

	closestRecorder := streamtest.New(
		streamtest.WithProtocols(psClosestPeer.Protocol()),
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return f
		}),
	)

	// creating the pivot peer
	psPivot, storerPivotDB := createPushSyncNode(t, pivotPeer, closestRecorder, mock.WithClosestPeer(closestPeer))
	defer storerPivotDB.Close()

	pivotRecorder := streamtest.New(
		streamtest.WithProtocols(psPivot.Protocol()),
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return f
		}),
	)

	// Creating the trigger peer
	psTriggerPeer, triggerStorerDB := createPushSyncNode(t, triggerPeer, pivotRecorder, mock.WithClosestPeer(pivotPeer))
	defer triggerStorerDB.Close()

	receipt, err := psTriggerPeer.SendChunkAndReceiveReceipt(context.Background(), pivotPeer, chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(swarm.NewAddress(receipt.Address)) {
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
}

func createPushSyncNode(t *testing.T, addr swarm.Address, recorder *streamtest.Recorder, mockOpts ...mock.Option) (*pushsync.PushSync, *localstore.DB) {
	logger := logging.New(ioutil.Discard, 0)

	storer, err := localstore.New("", addr.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	mockTopology := mock.NewTopologyDriver(mockOpts...)

	ps := pushsync.New(pushsync.Options{
		Streamer:      recorder,
		Storer:        storer,
		ClosestPeerer: mockTopology,
		Logger:        logger,
	})

	return ps, storer
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
