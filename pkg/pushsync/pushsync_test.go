// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/mock"

	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestSencChunk tests that a chunk that is uploaded to localstore is sent to the appropriate closest peer.
func TestSendToClosest(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	// Create a mock connectivity between the peers
	mockTopology := mock.NewTopologyDriver(mock.WithClosestPeer(closestPeer))

	// Set path to empty string so that memory will be used instead of persistent DB
	storer, err := localstore.New("", pivotNode.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	// setup the stream recorder to record stream data
	recorder := streamtest.New(
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return func(context.Context, p2p.Peer, p2p.Stream) error {
				// dont call any handlers
				return nil
			}
		}),
	)

	// instantiate a pushsync instance
	ps := pushsync.New(pushsync.Options{
		Streamer:      recorder,
		Logger:        logger,
		ClosestPeerer: mockTopology,
		Storer:        storer,
	})
	defer ps.Close()
	recorder.SetProtocols(ps.Protocol())

	// upload the chunk to the pivot node
	_, err = storer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(chunkAddress, chunkData))
	if err != nil {
		t.Fatal(err)
	}

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
	chunk := swarm.NewChunk(swarm.NewAddress(delivery.Address), delivery.Data)

	if !bytes.Equal(chunk.Address().Bytes(), chunkAddress.Bytes()) {
		t.Fatalf("chunk address mismatch")
	}

	if !bytes.Equal(chunk.Data(), chunkData) {
		t.Fatalf("chunk data mismatch")
	}
}

// TestForwardChunk tests that when a closer node exists within the topology, we forward a received
// chunk to it.
func TestForwardChunk(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")

	// create a pivot node and a closest mocked closer node address
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // pivot is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110

	// Create a mock connectivity driver
	mockTopology := mock.NewTopologyDriver(mock.WithClosestPeer(closestPeer))
	storer, err := localstore.New("", pivotNode.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	targetCalled := false
	var mtx sync.Mutex

	// setup the stream recorder to record stream data
	recorder := streamtest.New(
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {

			// this is a custom middleware that is needed because of the design of
			// the recorder. since we want to test only one unit, but the first message
			// is supposedly coming from another node, we don't want to execute the handler
			// when the peer address is the peer of `closestPeer`, since this will create an
			// unnecessary entry in the recorder
			return func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
				if p.Address.Equal(closestPeer) {
					mtx.Lock()
					defer mtx.Unlock()
					if targetCalled {
						t.Fatal("target called more than once")
					}
					targetCalled = true
					return nil
				}
				return f(ctx, p, s)
			}
		}),
	)

	ps := pushsync.New(pushsync.Options{
		Streamer:      recorder,
		Logger:        logger,
		ClosestPeerer: mockTopology,
		Storer:        storer,
	})
	defer ps.Close()

	recorder.SetProtocols(ps.Protocol())

	stream, err := recorder.NewStream(context.Background(), pivotNode, nil, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	w := protobuf.NewWriter(stream)

	// this triggers the handler of the pivot with a delivery stream
	err = w.WriteMsg(&pb.Delivery{
		Address: chunkAddress.Bytes(),
		Data:    chunkData,
	})
	if err != nil {
		t.Fatal(err)
	}

	_ = recorder.WaitRecords(t, closestPeer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
	mtx.Lock()
	defer mtx.Unlock()
	if !targetCalled {
		t.Fatal("target not called")
	}
}

// TestNoForwardChunk tests that the closest node to a chunk doesn't forward it to other nodes.
func TestNoForwardChunk(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000") // binary 0111
	chunkData := []byte("1234")

	// create a pivot node and a cluster of nodes
	pivotNode := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // pivot is 0110

	// Create a mock connectivity
	mockTopology := mock.NewTopologyDriver(mock.WithClosestPeerErr(topology.ErrWantSelf))

	storer, err := localstore.New("", pivotNode.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	recorder := streamtest.New(
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return f
		}),
	)

	ps := pushsync.New(pushsync.Options{
		Streamer:      recorder,
		Logger:        logger,
		ClosestPeerer: mockTopology,
		Storer:        storer,
	})
	defer ps.Close()

	recorder.SetProtocols(ps.Protocol())

	stream, err := recorder.NewStream(context.Background(), pivotNode, nil, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	w := protobuf.NewWriter(stream)

	// this triggers the handler of the pivot with a delivery stream
	err = w.WriteMsg(&pb.Delivery{
		Address: chunkAddress.Bytes(),
		Data:    chunkData,
	})
	if err != nil {
		t.Fatal(err)
	}

	_ = recorder.WaitRecords(t, pivotNode, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
}


// TestSendChunkAndGetReceipt send a chunk to the closest node and expects a receipt.
// the received node stores the chunk in the local store and then sends a receipt
func TestSendChunkAndGetReceipt(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")
	chunk := swarm.NewChunk(chunkAddress, chunkData)


	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	recorder := streamtest.New(
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return f
		}),
	)

	storer, err := localstore.New("", pivotNode.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	mockTopology := mock.NewTopologyDriver(mock.WithClosestPeer(closestPeer))

	ps := pushsync.New(pushsync.Options{
		Streamer:      recorder,
		Storer:        storer,
		ClosestPeerer: mockTopology,
		Logger:        logger,
	})
	defer ps.Close()
	recorder.SetProtocols(ps.Protocol())


	var wg sync.WaitGroup
	go func () {
		wg.Add(1)
		defer wg.Done()

		// 1 - Receive chunk from closest node
		records := recorder.WaitRecords(t, closestPeer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
		messages, err := protobuf.ReadMessages(
			bytes.NewReader(records[0].In()),
			func() protobuf.Message { return new(pb.Delivery) },
		)
		if err != nil {
			t.Fatal(err)
		}
		if messages == nil {
			t.Fatal(err)
		}
		if len(messages) > 1 {
			t.Fatal("too many messages")
		}
		delivery := messages[0].(*pb.Delivery)
		rcvdChunk := swarm.NewChunk(swarm.NewAddress(delivery.Address), delivery.Data)

		//  2 - respond with a receipt
		rcvStream, err := recorder.NewStream(context.Background(), pivotNode, nil, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
		if err != nil {
			t.Fatal(err)
		}
		defer rcvStream.Close()
		w := protobuf.NewWriter(rcvStream)

		// this triggers the handler of the pivot with a delivery stream
		err = w.WriteMsg(&pb.Receipt{
			Address: rcvdChunk.Address().Bytes(),
		})
		if err != nil {
			t.Fatal(err)
		}

	}()

	err = ps.SendChunkAndReceiveReceipt(context.Background(), closestPeer, chunk)
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()

}


// TestGetChunkAndSendReceipt receives a chunk from the closest node and sends a receipt.
func TestGetChunkAndSendReceipt(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")
	chunk := swarm.NewChunk(chunkAddress, chunkData)

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	recorder := streamtest.New(
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			return f
		}),
	)


	storer, err := localstore.New("", pivotNode.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	mockTopology := mock.NewTopologyDriver(mock.WithClosestPeer(closestPeer))

	ps := pushsync.New(pushsync.Options{
		Streamer:      recorder,
		Storer:        storer,
		ClosestPeerer: mockTopology,
		Logger:        logger,
	})
	defer ps.Close()
	recorder.SetProtocols(ps.Protocol())


	stream, err := recorder.NewStream(context.Background(), pivotNode, nil, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		defer wg.Done()
		w := protobuf.NewWriter(stream)

		// this triggers the handler of the pivot with a delivery stream
		err = w.WriteMsg(&pb.Delivery{
			Address: chunk.Address().Bytes(),
			Data:    chunk.Data(),
		})
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println("T - Sent chunk delivery - 1")

		records := recorder.WaitRecords(t, pivotNode, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName, 1, 5)
		messages, err := protobuf.ReadMessages(
			bytes.NewReader(records[0].In()),
			func() protobuf.Message { return new(pb.Receipt) },
		)
		if err != nil {
			t.Fatal(err)
		}
		if messages == nil {
			t.Fatal(err)
		}
		if len(messages) > 1 {
			t.Fatal("too many messages")
		}
		receipt := messages[0].(*pb.Receipt)

		if !bytes.Equal(receipt.Address, chunk.Address().Bytes()) {
			t.Fatal(err)
		}
	}()

	rcvdChunk, err := ps.ReceiveChunkAndSendReceipt(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	if !bytes.Equal(rcvdChunk.Address().Bytes(), chunkAddress.Bytes()) {
		t.Fatal(err)
	}

}
