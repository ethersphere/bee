// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/accounting"
	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/crypto"
	cryptomock "github.com/ethersphere/bee/pkg/crypto/mock"
	"github.com/ethersphere/bee/pkg/localstore"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pricer/headerutils"
	pricermock "github.com/ethersphere/bee/pkg/pricer/mock"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/mock"
)

const (
	fixedPrice = uint64(10)
)

type pricerParameters struct {
	price     uint64
	peerPrice uint64
}

var (
	defaultPrices = pricerParameters{price: fixedPrice, peerPrice: fixedPrice}
	defaultSigner = cryptomock.New(cryptomock.WithSignFunc(func([]byte) ([]byte, error) {
		return nil, nil
	}))
)

// TestSendChunkAndGetReceipt inserts a chunk as uploaded chunk in db. This triggers sending a chunk to the closest node
// and expects a receipt. The message are intercepted in the outgoing stream to check for correctness.
func TestSendChunkAndReceiveReceipt(t *testing.T) {
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to

	psPeer, storerPeer, _, peerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer.Close()

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot, _, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, mock.WithClosestPeer(closestPeer))
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
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), nil)
	balance, err := pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = peerAccounting.Balance(pivotNode)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on peer. want %d got %d", int64(fixedPrice), balance)
	}
}

func TestSendChunkAfterPriceUpdate(t *testing.T) {
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to

	serverPrice := uint64(17)
	serverPrices := pricerParameters{price: serverPrice, peerPrice: fixedPrice}

	psPeer, storerPeer, _, peerAccounting := createPushSyncNode(t, closestPeer, serverPrices, nil, nil, defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer.Close()

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot, _, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, mock.WithClosestPeer(closestPeer))
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
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), nil)

	balance, err := pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != -int64(serverPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(serverPrice), balance)
	}

	balance, err = peerAccounting.Balance(pivotNode)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != int64(serverPrice) {
		t.Fatalf("unexpected balance on peer. want %d got %d", int64(serverPrice), balance)
	}
}

// TestReplicateBeforeReceipt tests that a chunk is pushed and a receipt is received.
// Also the storer node initiates a pushsync to N closest nodes of the chunk as it's sending back the receipt.
// The second storer should only store it and not forward it. The balance of all nodes is tested.
func TestReplicateBeforeReceipt(t *testing.T) {

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000") // base 0111

	neighborPrice := pricerParameters{price: 0, peerPrice: 0}

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110
	secondPeer := swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000")  // binary 0100
	emptyPeer := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")   // binary 0101, this peer should not get the chunk

	// node that is connected to secondPeer
	// it's address is closer to the chunk than secondPeer but it will not receive the chunk
	_, storerEmpty, _, _ := createPushSyncNode(t, emptyPeer, defaultPrices, nil, nil, defaultSigner)
	defer storerEmpty.Close()

	// node that is connected to closestPeer
	// will receieve chunk from closestPeer
	psSecond, storerSecond, _, secondAccounting := createPushSyncNode(t, secondPeer, neighborPrice, nil, nil, defaultSigner, mock.WithPeers(emptyPeer))
	defer storerSecond.Close()
	secondRecorder := streamtest.New(streamtest.WithProtocols(psSecond.Protocol()), streamtest.WithBaseAddr(closestPeer))

	psStorer, storerPeer, _, storerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, secondRecorder, nil, defaultSigner, mock.WithPeers(secondPeer), mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer.Close()
	recorder := streamtest.New(streamtest.WithProtocols(psStorer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot, _, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, mock.WithClosestPeer(closestPeer))
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
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), nil)

	// sleep for a bit to allow the second peer to the store replicated chunk
	time.Sleep(time.Millisecond * 500)

	// this intercepts the outgoing delivery message from storer node to second storer node
	waitOnRecordAndTest(t, secondPeer, secondRecorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, secondPeer, secondRecorder, chunk.Address(), nil)

	_, err = storerEmpty.Get(context.Background(), storage.ModeGetSync, chunk.Address())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}

	balance, err := pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on storer node. want %d got %d", int64(fixedPrice), balance)
	}

	balance, err = storerAccounting.Balance(pivotNode)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on storer node. want %d got %d", int64(fixedPrice), balance)
	}

	balance, err = secondAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != 0 {
		t.Fatalf("unexpected balance on second storer. want %d got %d", 0, balance)
	}

	balance, err = storerAccounting.Balance(secondPeer)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != 0 {
		t.Fatalf("unexpected balance on storer node. want %d got %d", 0, balance)
	}
}

// PushChunkToClosest tests the sending of chunk to closest peer from the origination source perspective.
// it also checks wether the tags are incremented properly if they are present
func TestPushChunkToClosest(t *testing.T) {
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1
	callbackC := make(chan struct{}, 1)
	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to

	psPeer, storerPeer, _, peerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, chanFunc(callbackC), defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer.Close()

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot, pivotTags, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, mock.WithClosestPeer(closestPeer))
	defer storerPivot.Close()

	ta, err := pivotTags.Create(1)
	if err != nil {
		t.Fatal(err)
	}
	chunk = chunk.WithTagID(ta.Uid)

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
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), nil)

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

	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = peerAccounting.Balance(pivotNode)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on peer. want %d got %d", int64(fixedPrice), balance)
	}

	// check if the pss delivery hook is called
	select {
	case <-callbackC:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("delivery hook was not called")
	}
}

func TestPushChunkToNextClosest(t *testing.T) {

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000

	peer1 := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer1, storerPeer1, _, peerAccounting1 := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer1.Close()

	psPeer2, storerPeer2, _, peerAccounting2 := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer storerPeer2.Close()

	var fail = true
	var lock sync.Mutex

	recorder := streamtest.New(
		streamtest.WithProtocols(
			psPeer1.Protocol(),
			psPeer2.Protocol(),
		),
		streamtest.WithMiddlewares(
			func(h p2p.HandlerFunc) p2p.HandlerFunc {
				return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
					// this hack is required to simulate first storer node failing
					lock.Lock()
					defer lock.Unlock()
					if fail {
						fail = false
						return errors.New("peer not reachable")
					}

					if err := h(ctx, peer, stream); err != nil {
						return err
					}
					// close stream after all previous middlewares wrote to it
					// so that the receiving peer can get all the post messages
					return stream.Close()
				}
			},
		),
		streamtest.WithBaseAddr(pivotNode),
	)

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, storerPivot, pivotTags, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, mock.WithPeers(peer1, peer2))
	defer storerPivot.Close()

	ta, err := pivotTags.Create(1)
	if err != nil {
		t.Fatal(err)
	}
	chunk = chunk.WithTagID(ta.Uid)

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
	waitOnRecordAndTest(t, peer2, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, peer2, recorder, chunk.Address(), nil)

	ta2, err := pivotTags.Get(ta.Uid)
	if err != nil {
		t.Fatal(err)
	}
	if ta2.Get(tags.StateSent) != 2 {
		t.Fatalf("tags error")
	}

	balance, err := pivotAccounting.Balance(peer2)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance2, err := peerAccounting2.Balance(pivotNode)
	if err != nil {
		t.Fatal(err)
	}

	if balance2.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on peer2. want %d got %d", int64(fixedPrice), balance2)
	}

	balance1, err := peerAccounting1.Balance(peer1)
	if err != nil {
		t.Fatal(err)
	}

	if balance1.Int64() != 0 {
		t.Fatalf("unexpected balance on peer1. want %d got %d", 0, balance1)
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
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	triggerPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	pivotPeer := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	// Create the closest peer
	psClosestPeer, closestStorerPeerDB, _, closestAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer closestStorerPeerDB.Close()

	closestRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()), streamtest.WithBaseAddr(pivotPeer))

	// creating the pivot peer
	psPivot, storerPivotDB, _, pivotAccounting := createPushSyncNode(t, pivotPeer, defaultPrices, closestRecorder, nil, defaultSigner, mock.WithClosestPeer(closestPeer))
	defer storerPivotDB.Close()

	pivotRecorder := streamtest.New(streamtest.WithProtocols(psPivot.Protocol()), streamtest.WithBaseAddr(triggerPeer))

	// Creating the trigger peer
	psTriggerPeer, triggerStorerDB, _, triggerAccounting := createPushSyncNode(t, triggerPeer, defaultPrices, pivotRecorder, nil, defaultSigner, mock.WithClosestPeer(pivotPeer))
	defer triggerStorerDB.Close()

	receipt, err := psTriggerPeer.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// In pivot peer,  intercept the incoming delivery chunk from the trigger peer and check for correctness
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunk.Address(), chunk.Data())

	// Pivot peer will forward the chunk to its closest peer. Intercept the incoming stream from pivot node and check
	// for the correctness of the chunk
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunk.Address(), chunk.Data())

	// Similarly intercept the same incoming stream to see if the closest peer is sending a proper receipt
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunk.Address(), nil)

	// In the received stream, check if a receipt is sent from pivot peer and check for its correctness.
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunk.Address(), nil)

	balance, err := triggerAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on trigger. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = pivotAccounting.Balance(triggerPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", int64(fixedPrice), balance)
	}

	balance, err = pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = closestAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on closest. want %d got %d", int64(fixedPrice), balance)
	}
}

func TestHandlerWithUpdate(t *testing.T) {
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	serverPrice := uint64(17)
	serverPrices := pricerParameters{price: serverPrice, peerPrice: fixedPrice}

	// create a pivot node and a mocked closest node
	triggerPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	pivotPeer := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	// Create the closest peer with default prices (10)
	psClosestPeer, closestStorerPeerDB, _, closestAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer closestStorerPeerDB.Close()

	closestRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()), streamtest.WithBaseAddr(pivotPeer))

	// creating the pivot peer who will act as a forwarder node with a higher price (17)
	psPivot, storerPivotDB, _, pivotAccounting := createPushSyncNode(t, pivotPeer, serverPrices, closestRecorder, nil, defaultSigner, mock.WithClosestPeer(closestPeer))
	defer storerPivotDB.Close()

	pivotRecorder := streamtest.New(streamtest.WithProtocols(psPivot.Protocol()), streamtest.WithBaseAddr(triggerPeer))

	// Creating the trigger peer with default price (10)
	psTriggerPeer, triggerStorerDB, _, triggerAccounting := createPushSyncNode(t, triggerPeer, defaultPrices, pivotRecorder, nil, defaultSigner, mock.WithClosestPeer(pivotPeer))
	defer triggerStorerDB.Close()

	receipt, err := psTriggerPeer.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// In pivot peer,  intercept the incoming delivery chunk from the trigger peer and check for correctness
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunk.Address(), chunk.Data())

	// Pivot peer will forward the chunk to its closest peer. Intercept the incoming stream from pivot node and check
	// for the correctness of the chunk
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunk.Address(), chunk.Data())

	// Similarly intercept the same incoming stream to see if the closest peer is sending a proper receipt
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunk.Address(), nil)

	// In the received stream, check if a receipt is sent from pivot peer and check for its correctness.
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunk.Address(), nil)

	balance, err := triggerAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}

	// balance on triggering peer towards the forwarder should show negative serverPrice (17)
	if balance.Int64() != -int64(serverPrice) {
		t.Fatalf("unexpected balance on trigger. want %d got %d", -int64(serverPrice), balance)
	}

	// we need to check here for pivotPeer instead of triggerPeer because during streamtest the peer in the handler is actually the receiver
	// balance on forwarding peer for the triggering peer should show serverPrice (17)
	balance, err = pivotAccounting.Balance(triggerPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != int64(serverPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", int64(serverPrice), balance)
	}

	balance, err = pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}
	// balance of the forwarder peer for the closest peer should show negative default price (10)
	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = closestAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}
	// balance of the closest peer for the forwarder peer should show the default price (10)
	if balance.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on closest. want %d got %d", int64(fixedPrice), balance)
	}
}

func TestSignsReceipt(t *testing.T) {

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	signer := cryptomock.New(cryptomock.WithSignFunc(func([]byte) ([]byte, error) {
		return []byte{1}, nil
	}))

	// create a pivot node and a mocked closest node
	pivotPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	// Create the closest peer
	psClosestPeer, closestStorerPeerDB, _, _ := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, signer, mock.WithClosestPeerErr(topology.ErrWantSelf))
	defer closestStorerPeerDB.Close()

	closestRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()), streamtest.WithBaseAddr(pivotPeer))

	// creating the pivot peer who will act as a forwarder node with a higher price (17)
	psPivot, storerPivotDB, _, _ := createPushSyncNode(t, pivotPeer, defaultPrices, closestRecorder, nil, signer, mock.WithClosestPeer(closestPeer))
	defer storerPivotDB.Close()

	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	if !bytes.Equal(chunk.Address().Bytes(), receipt.Address.Bytes()) {
		t.Fatal("chunk address do not match")
	}

	if !bytes.Equal([]byte{1}, receipt.Signature) {
		t.Fatal("receipt signature is not present")
	}
}

func createPushSyncNode(t *testing.T, addr swarm.Address, prices pricerParameters, recorder *streamtest.Recorder, unwrap func(swarm.Chunk), signer crypto.Signer, mockOpts ...mock.Option) (*pushsync.PushSync, *localstore.DB, *tags.Tags, accounting.Interface) {
	t.Helper()
	logger := logging.New(ioutil.Discard, 0)

	storer, err := localstore.New("", addr.Bytes(), nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	mockTopology := mock.NewTopologyDriver(mockOpts...)
	mockStatestore := statestore.NewStateStore()
	mtag := tags.NewTags(mockStatestore, logger)
	mockAccounting := accountingmock.NewAccounting()

	headlerFunc := func(h p2p.Headers, a swarm.Address) p2p.Headers {
		target, _ := headerutils.ParseTargetHeader(h)
		headers, _ := headerutils.MakePricingResponseHeaders(prices.price, target, 0)
		return headers
	}

	mockPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithPrice(prices.price), pricermock.WithPeerPrice(prices.peerPrice))

	recorderDisconnecter := streamtest.NewRecorderDisconnecter(recorder)
	if unwrap == nil {
		unwrap = func(swarm.Chunk) {}
	}

	return pushsync.New(addr, recorderDisconnecter, storer, mockTopology, mtag, unwrap, logger, mockAccounting, mockPricer, signer, nil), storer, mtag, mockAccounting
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

func chanFunc(c chan<- struct{}) func(swarm.Chunk) {
	return func(_ swarm.Chunk) {
		c <- struct{}{}
	}
}
