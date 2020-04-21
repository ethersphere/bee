// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/pushsync/pb"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	mockstate "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/full"
	ma "github.com/multiformats/go-multiaddr"
)

// TestSencChunk tests that a chunk that is uploaded to localstore is sent to the appropriate closest peer.
func TestSendChunk(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	// chunk data to upload
	chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	chunkData := []byte("1234")

	// create a pivot node and a cluster of nodes
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000
	connectedPeers := []p2p.Peer{
		{
			Address: swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"), // binary 1000 -> po 0
		},
		{
			Address: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // binary 0100 -> po 1
		},
		{
			Address: swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000"), // binary 0110 -> po 1
		},
	}
	closestPeer := connectedPeers[2].Address

	// mock a connectivity between the nodes
	p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
		return pivotNode, nil
	}), p2pmock.WithPeersFunc(func() []p2p.Peer {
		return connectedPeers
	}))

	// Create a full connectivity between the peers
	discovery := mock.NewDiscovery()
	statestore := mockstate.NewStateStore()
	ab := addressbook.New(statestore)
	fullDriver := full.New(discovery, ab, p2ps, logger, pivotNode)

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
		ClosestPeerer: fullDriver,
		Storer:        storer,
	})
	defer ps.Close()
	recorder.SetProtocols(ps.Protocol())

	// upload the chunk to the pivot node
	_, err = storer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(chunkAddress, chunkData))
	if err != nil {
		t.Fatal(err)
	}

	var records []*streamtest.Record

LOOP:
	for i := 0; i < 10; i++ {
		records, _ = recorder.Records(closestPeer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
		switch l := len(records); l {
		case 1:
			break LOOP
		case 0:
			time.Sleep(time.Millisecond * 10)
		default:
			t.Fatal("too many records!")
		}
	}

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

	// create a pivot node and a cluster of nodes
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // pivot is 0000
	connectedPeers := []p2p.Peer{
		{
			Address: swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"), // binary 1000 -> po 0
		},
		{
			Address: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // binary 0100 -> po 1
		},
		{
			Address: swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000"), // binary 0110 -> po 1 want this one
		},
	}
	closestPeer := connectedPeers[2].Address

	// mock a connectivity between the nodes
	p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
		return pivotNode, nil
	}), p2pmock.WithPeersFunc(func() []p2p.Peer {
		return connectedPeers
	}))

	// Create a full connectivity driver
	fullDriver := full.New(mock.NewDiscovery(), addressbook.New(mockstate.NewStateStore()), p2ps, logger, pivotNode)
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
				if p.Address.Equal(connectedPeers[2].Address) {
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
		ClosestPeerer: fullDriver,
		Storer:        storer,
	})
	defer ps.Close()

	// TODO: this needs to go away once we have a simpler recorder
	recorder.SetProtocols(ps.Protocol())

	stream, err := recorder.NewStream(context.Background(), pivotNode, nil, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	w, _ := protobuf.NewWriterAndReader(stream)

	// this triggers the handler of the pivot with a delivery stream
	err = w.WriteMsg(&pb.Delivery{
		Address: chunkAddress.Bytes(),
		Data:    chunkData,
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		mtx.Lock()
		called := targetCalled
		mtx.Unlock()

		if !called {
			time.Sleep(10 * time.Millisecond)
		}
	}

	records, err := recorder.Records(closestPeer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
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
	connectedPeers := []p2p.Peer{
		{
			Address: swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"), // binary 1000 -> po 0
		},
		{
			Address: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // binary 0100 -> po 1
		},
		{
			Address: swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000"), // binary 0000 -> po 0
		},
	}

	// mock a connectivity between the nodes
	p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
		return pivotNode, nil
	}), p2pmock.WithPeersFunc(func() []p2p.Peer {
		return connectedPeers
	}))

	// Create a full connectivity driver
	fullDriver := full.New(mock.NewDiscovery(), addressbook.New(mockstate.NewStateStore()), p2ps, logger, pivotNode)
	storer, err := localstore.New("", pivotNode.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	// how many calls to the handler we've handled
	calls := 0

	// setup the stream recorder to record stream data
	recorder := streamtest.New(
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			calls++
			if runtime.GOOS == "windows" {
				// windows has a bit lower time resolution
				// so, slow down the handler with a middleware
				// not to get 0s for rtt value
				time.Sleep(100 * time.Millisecond)
			}
			return f
		}),
	)

	ps := pushsync.New(pushsync.Options{
		Streamer:      recorder,
		Logger:        logger,
		ClosestPeerer: fullDriver,
		Storer:        storer,
	})
	defer ps.Close()

	// TODO: this needs to go away once we have a simpler recorder
	recorder.SetProtocols(ps.Protocol())

	stream, err := recorder.NewStream(context.Background(), pivotNode, nil, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	w, _ := protobuf.NewWriterAndReader(stream)

	// this triggers the handler of the pivot with a delivery stream
	err = w.WriteMsg(&pb.Delivery{
		Address: chunkAddress.Bytes(),
		Data:    chunkData,
	})
	if err != nil {
		t.Fatal(err)
	}

	// wait for some time and verify there are no entries here
	for i := 0; i < 20; i++ {
		if calls <= 1 {
			time.Sleep(10 * time.Millisecond)
		} else {
			t.Fatal("too many calls")
		}
	}
}
