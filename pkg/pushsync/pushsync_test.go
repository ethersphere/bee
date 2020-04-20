// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"github.com/ethersphere/bee/pkg/pushsync"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	mockstate "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/full"
	ma "github.com/multiformats/go-multiaddr"
)

func TestAddChunkToLocalStore(t *testing.T) {
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

	// create a localstore
	dir, err := ioutil.TempDir("", "localstore-stored-gc-size")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	//TODO: need to use memdb after merge with master
	storer, err := localstore.New(dir, pivotNode.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	// instantiate a pushsync protocol
	server := pushsync.New(pushsync.Options{
		Logger:     logger,
		SyncPeerer: fullDriver,
		Storer:     storer,
	})

	// setup the stream recorder to record stream data
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
			if runtime.GOOS == "windows" {
				// windows has a bit lower time resolution
				// so, slow down the handler with a middleware
				// not to get 0s for rtt value
				time.Sleep(100 * time.Millisecond)
			}
			return f
		}),
	)

	// upload the chunk to the pivot node
	_, err = storer.Put(context.Background(), storage.ModePutUpload, swarm.NewChunk(chunkAddress, chunkData))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 5)

	// check if the chunk is there in the closest node
	closestPeer := connectedPeers[2].Address
	records, err := recorder.Records(closestPeer, pushsync.ProtocolName,
		pushsync.ProtocolVersion, pushsync.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]


	chunk := swarm.NewChunk(swarm.NewAddress(record.In()[:20]), record.In()[20:])


	if !bytes.Equal(chunk.Address().Bytes(), chunkAddress.Bytes()) {
		t.Fatalf("chunk address mismatch")
	}

	if !bytes.Equal(chunk.Data(), chunkData) {
		t.Fatalf("chunk data mismatch")
	}



	// also check if the chunks is not present in any other node
}

func TestReceiveChunkFromClosestPeer(t *testing.T) {
	//logger := logging.New(ioutil.Discard, 0)
	//
	//// received chunk
	//chunkAddress := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")
	//
	//// create a receiving node and a cluster of nodes
	//receivingNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000
	//connectedPeers := []p2p.Peer{
	//	{
	//		Address: swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"), // binary 1000 -> po 0
	//	},
	//	{
	//		Address: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // binary 0100 -> po 1
	//	},
	//	{
	//		Address: swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000"), // binary 0110 -> po 1
	//	},
	//}
	//
	//// mock a connectivity between the nodes
	//p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (swarm.Address, error) {
	//	return receivingNode, nil
	//}), p2pmock.WithPeersFunc(func() []p2p.Peer {
	//	return connectedPeers
	//}))
	//
	//// Create a full connectivity between the peers
	//discovery := mock.NewDiscovery()
	//statestore := mockstate.NewStateStore()
	//ab := addressbook.New(statestore)
	//fullDriver := full.New(discovery, ab, p2ps, logger, receivingNode)
	//
	//// instantiate a pushsync protocol
	//server := pushsync.New(pushsync.Options{
	//	Logger:     logger,
	//	SyncPeerer: fullDriver,
	//	Storer:     nil,
	//})
	//
	//// setup the stream recorder to record stream data
	//recorder := streamtest.New(
	//	streamtest.WithProtocols(server.Protocol()),
	//	streamtest.WithMiddlewares(func(f p2p.HandlerFunc) p2p.HandlerFunc {
	//		if runtime.GOOS == "windows" {
	//			// windows has a bit lower time resolution
	//			// so, slow down the handler with a middleware
	//			// not to get 0s for rtt value
	//			time.Sleep(100 * time.Millisecond)
	//		}
	//		return f
	//	}),
	//)
	//
	//stream, err := recorder.NewStream(context.Background(), receivingNode, nil, pushsync.ProtocolName,
	//	pushsync.ProtocolVersion, pushsync.StreamName)
	//defer stream.Close()
	//
	//w, r := protobuf.NewWriterAndReader(stream)
	//w.WriteMsg(&pb.Delivery{
	//	Data: msg,
	//})
	//
	//records, err := recorder.Records(connectedPeers[2].Address, "pushsync", "1.0.0", "pushsync")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//if l := len(records); l != 1 {
	//	t.Fatalf("got %v records, want %v", l, 1)
	//}
	//record := records[0]
}
