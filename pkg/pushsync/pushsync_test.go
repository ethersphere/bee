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
	"github.com/ethersphere/bee/pkg/storage"
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
	defer psPeer.Close()

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
	defer psPivot.Close()

	// upload the chunk to the pivot node
	_, err := storerPivot.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// this intercepts the outgoing delivery message
	records := recorder.WaitRecords(t, closestPeer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(records[0].In()),
		func() protobuf.Message { return new(pb.Delivery) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) > 1 {
		t.Fatal("too many messages")
	}
	delivery := messages[0].(*pb.Delivery)
	rcvdChunk := swarm.NewChunk(swarm.NewAddress(delivery.Address), delivery.Data)

	if !bytes.Equal(rcvdChunk.Address().Bytes(), chunkAddress.Bytes()) {
		t.Fatalf("chunk address mismatch")
	}

	if !bytes.Equal(rcvdChunk.Data(), chunkData) {
		t.Fatalf("chunk data mismatch")
	}



	// this intercepts the incoming receipt message
	records = recorder.WaitRecords(t, closestPeer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
	messages, err = protobuf.ReadMessages(
		bytes.NewReader(records[0].In()),
		func() protobuf.Message { return new(pb.Receipt) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) > 1 {
		t.Fatal("too many messages")
	}
	receipt := messages[0].(*pb.Receipt)
	receiptAddress := swarm.NewAddress(receipt.Address)

	if !bytes.Equal(receiptAddress.Bytes(), chunkAddress.Bytes()) {
		t.Fatalf("receipt address mismatch")
	}
}

// TestHandler expect a chunk from a node on a stream. It then stores the chunk in the local store and
// sends back a receipt. This is tested by intercepting the incoming stream for proper messages.
func TestHandler(t *testing.T) {
	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")
	chunk := swarm.NewChunk(chunkAddress, chunkData)

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1


	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot := createPushSyncNode(t, pivotNode, nil, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPivot.Close()
	defer psPivot.Close()


	recorder := streamtest.New(
		streamtest.WithProtocols(psPivot.Protocol()),
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return f
		}),
	)

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's noone to forward to
	psPeer, storerPeer := createPushSyncNode(t, closestPeer, recorder, mock.WithClosestPeer(pivotNode))
	defer storerPeer.Close()
	defer psPeer.Close()


	// upload the chunk to the pivot node
	_, err := storerPeer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// this intercepts the incoming delivery message from a peer
	records := recorder.WaitRecords(t, pivotNode, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(records[0].In()),
		func() protobuf.Message { return new(pb.Delivery) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) > 1 {
		t.Fatal("too many messages")
	}
	delivery := messages[0].(*pb.Delivery)
	rcvdChunk := swarm.NewChunk(swarm.NewAddress(delivery.Address), delivery.Data)

	if !bytes.Equal(rcvdChunk.Address().Bytes(), chunkAddress.Bytes()) {
		t.Fatalf("chunk address mismatch")
	}

	if !bytes.Equal(rcvdChunk.Data(), chunkData) {
		t.Fatalf("chunk data mismatch")
	}

	// this intercepts the outgoing receipt message on the same stream
	records = recorder.WaitRecords(t, pivotNode, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
	messages, err = protobuf.ReadMessages(
		bytes.NewReader(records[0].In()),
		func() protobuf.Message { return new(pb.Receipt) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) > 1 {
		t.Fatal("too many messages")
	}
	receipt := messages[0].(*pb.Receipt)
	receiptAddress := swarm.NewAddress(receipt.Address)

	if !bytes.Equal(receiptAddress.Bytes(), chunkAddress.Bytes()) {
		t.Fatalf("receipt address mismatch")
	}
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
