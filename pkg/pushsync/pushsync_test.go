// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/accounting"
	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/crypto"
	cryptomock "github.com/ethersphere/bee/pkg/crypto/mock"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/postage"
	bsMock "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	pricermock "github.com/ethersphere/bee/pkg/pricer/mock"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/pushsync/pb"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	mocks "github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/mock"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

const (
	fixedPrice = uint64(10)
)

var blockHash = common.HexToHash("0x1")

type pricerParameters struct {
	price     uint64
	peerPrice uint64
}

var (
	defaultPrices = pricerParameters{price: fixedPrice, peerPrice: fixedPrice}
	defaultSigner = cryptomock.New(cryptomock.WithSignFunc(func([]byte) ([]byte, error) {
		return nil, nil
	}))
	withinRadius = bsMock.New(bsMock.WithIsWithinStorageRadius(true))
)

// TestPushClosest inserts a chunk as uploaded chunk in db. This triggers sending a chunk to the closest node
// and expects a receipt. The message are intercepted in the outgoing stream to check for correctness.
func TestPushClosest(t *testing.T) {
	t.Parallel()
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer, _, _, peerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, _, _, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, nil, mock.WithClosestPeer(closestPeer))

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

// TestReplicateBeforeReceipt tests that a chunk is pushed and a receipt is received.
// Also the storer node initiates a pushsync to N closest nodes of the chunk as it's sending back the receipt.
// The second storer should only store it and not forward it. The balance of all nodes is tested.
func TestReplicateBeforeReceipt(t *testing.T) {
	t.Parallel()
	t.Skip("skipped for now because replication has been removed")

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000") // base 0111

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110
	secondPeer := swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000")  // binary 0100
	emptyPeer := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")   // binary 0101, this peer should not get the chunk

	// node that is connected to secondPeer
	// it's address is closer to the chunk than secondPeer but it will not receive the chunk
	psEmpty, storerEmpty, _, _ := createPushSyncNode(t, emptyPeer, defaultPrices, nil, nil, defaultSigner, nil)

	emptyRecorder := streamtest.New(streamtest.WithProtocols(psEmpty.Protocol()), streamtest.WithBaseAddr(secondPeer))

	// node that is connected to closestPeer
	// will receieve chunk from closestPeer
	psSecond, _, _, secondAccounting := createPushSyncNode(t, secondPeer, defaultPrices, emptyRecorder, nil, defaultSigner, withinRadius, mock.WithPeers(emptyPeer))

	secondRecorder := streamtest.New(streamtest.WithProtocols(psSecond.Protocol()), streamtest.WithBaseAddr(closestPeer))

	psStorer, _, _, storerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, secondRecorder, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf), mock.WithPeers(secondPeer))

	recorder := streamtest.New(streamtest.WithProtocols(psStorer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, _, _, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, nil, mock.WithPeers(closestPeer))

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

	// this intercepts the outgoing delivery message from storer node to second storer node
	waitOnRecordAndTest(t, secondPeer, secondRecorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, secondPeer, secondRecorder, chunk.Address(), nil)

	_, err = storerEmpty.Get(context.Background(), storage.ModeGetSync, chunk.Address())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatal(err)
	}

	// Give some time for accounting goroutines to finish.
	time.Sleep(time.Millisecond * 100)

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

	if balance.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on second storer. want %d got %d", int64(fixedPrice), balance)
	}

	balance, err = storerAccounting.Balance(secondPeer)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on storer node. want %d got %d", -int64(fixedPrice), balance)
	}
}

// PushChunkToClosest tests the sending of chunk to closest peer from the origination source perspective.
// it also checks wether the tags are incremented properly if they are present
func TestPushChunkToClosest(t *testing.T) {
	t.Parallel()
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1
	callbackC := make(chan struct{}, 1)
	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to

	psPeer, _, _, peerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, chanFunc(callbackC), defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, _, pivotTags, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, nil, mock.WithClosestPeer(closestPeer))

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
	t.Parallel()
	t.Skip("flaky test")

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000

	peer1 := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer1, _, _, peerAccounting1 := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer2, _, _, peerAccounting2 := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

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
						stream.Close()
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
	psPivot, _, pivotTags, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, nil, mock.WithPeers(peer1, peer2))

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

	// the write to the first peer might succeed or
	// fail, so it is not guaranteed that two increments
	// are made to Sent. expect >= 1
	if tg := ta2.Get(tags.StateSent); tg == 0 {
		t.Fatalf("tags error got %d want >= 1", tg)
	}

	balance, err := pivotAccounting.Balance(peer1)
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

	balance1, err := peerAccounting1.Balance(peer2)
	if err != nil {
		t.Fatal(err)
	}

	if balance1.Int64() != 0 {
		t.Fatalf("unexpected balance on peer1. want %d got %d", 0, balance1)
	}
}

func TestPushChunkToClosestErrorAttemptRetry(t *testing.T) {
	t.Parallel()

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000

	peer1 := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	peer3 := swarm.MustParseHexAddress("9000000000000000000000000000000000000000000000000000000000000000")
	peer4 := swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000")

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer1, _, _, peerAccounting1 := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer2, _, _, peerAccounting2 := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer3, _, _, peerAccounting3 := createPushSyncNode(t, peer3, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer4, _, _, peerAccounting4 := createPushSyncNode(t, peer4, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(
		streamtest.WithProtocols(
			psPeer1.Protocol(),
			psPeer2.Protocol(),
			psPeer3.Protocol(),
			psPeer4.Protocol(),
		),
		streamtest.WithBaseAddr(pivotNode),
	)

	var pivotAccounting *accountingmock.Service
	pivotAccounting = accountingmock.NewAccounting(
		accountingmock.WithPrepareCreditFunc(func(peer swarm.Address, price uint64, originated bool) (accounting.Action, error) {
			if peer.String() == peer4.String() {
				return pivotAccounting.MakeCreditAction(peer, price), nil
			}
			return nil, errors.New("unable to reserve")
		}),
	)

	psPivot, _, pivotTags := createPushSyncNodeWithAccounting(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, pivotAccounting, nil, mock.WithPeers(peer1, peer2, peer3, peer4))

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
	waitOnRecordAndTest(t, peer4, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, peer4, recorder, chunk.Address(), nil)

	ta2, err := pivotTags.Get(ta.Uid)
	if err != nil {
		t.Fatal(err)
	}
	// out of 4, 3 peers should return accouting error. So we should have effectively
	// sent only 1 msg
	if ta2.Get(tags.StateSent) != 1 {
		t.Fatalf("tags error")
	}

	balance, err := pivotAccounting.Balance(peer4)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance4, err := peerAccounting4.Balance(pivotNode)
	if err != nil {
		t.Fatal(err)
	}

	if balance4.Int64() != int64(fixedPrice) {
		t.Fatalf("unexpected balance on peer4. want %d got %d", int64(fixedPrice), balance4)
	}

	for _, p := range []struct {
		addr swarm.Address
		acct accounting.Interface
	}{
		{peer1, peerAccounting1},
		{peer2, peerAccounting2},
		{peer3, peerAccounting3},
	} {
		bal, err := p.acct.Balance(p.addr)
		if err != nil {
			t.Fatal(err)
		}

		if bal.Int64() != 0 {
			t.Fatalf("unexpected balance on %s. want %d got %d", p.addr, 0, bal)
		}
	}
}

// TestHandler expect a chunk from a node on a stream. It then stores the chunk in the local store and
// sends back a receipt. This is tested by intercepting the incoming stream for proper messages.
// It also sends the chunk to the closest peer and receives a receipt.
//
// Chunk moves from   TriggerPeer -> PivotPeer -> ClosestPeer
func TestHandler(t *testing.T) {
	t.Parallel()
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	triggerPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	pivotPeer := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	// Create the closest peer
	psClosestPeer, _, _, closestAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	closestRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()), streamtest.WithBaseAddr(pivotPeer))

	// creating the pivot peer
	psPivot, _, _, pivotAccounting := createPushSyncNode(t, pivotPeer, defaultPrices, closestRecorder, nil, defaultSigner, nil, mock.WithPeers(closestPeer))

	pivotRecorder := streamtest.New(streamtest.WithProtocols(psPivot.Protocol()), streamtest.WithBaseAddr(triggerPeer))

	// Creating the trigger peer
	psTriggerPeer, _, _, triggerAccounting := createPushSyncNode(t, triggerPeer, defaultPrices, pivotRecorder, nil, defaultSigner, nil, mock.WithPeers(pivotPeer))

	receipt, err := psTriggerPeer.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// Pivot peer will forward the chunk to its closest peer. Intercept the incoming stream from pivot node and check
	// for the correctness of the chunk
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunk.Address(), chunk.Data())

	// Similarly intercept the same incoming stream to see if the closest peer is sending a proper receipt
	waitOnRecordAndTest(t, closestPeer, closestRecorder, chunk.Address(), nil)

	// In the received stream, check if a receipt is sent from pivot peer and check for its correctness.
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunk.Address(), nil)

	// In pivot peer,  intercept the incoming delivery chunk from the trigger peer and check for correctness
	waitOnRecordAndTest(t, pivotPeer, pivotRecorder, chunk.Address(), chunk.Data())

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

func TestSignsReceipt(t *testing.T) {
	t.Parallel()

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	signer := cryptomock.New(cryptomock.WithSignFunc(func([]byte) ([]byte, error) {
		return []byte{1}, nil
	}))

	// create a pivot node and a mocked closest node
	pivotPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	// Create the closest peer
	psClosestPeer, _, _, _ := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, signer, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	closestRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()), streamtest.WithBaseAddr(pivotPeer))

	// creating the pivot peer who will act as a forwarder node with a higher price (17)
	psPivot, _, _, _ := createPushSyncNode(t, pivotPeer, defaultPrices, closestRecorder, nil, signer, nil, mock.WithPeers(closestPeer))

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

	if !bytes.Equal(blockHash.Bytes(), receipt.Nonce) {
		t.Fatal("receipt block hash do not match")
	}
}

func TestMultiplePushesAsForwarder(t *testing.T) {
	t.Parallel()

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000

	peer1 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000")
	peer3 := swarm.MustParseHexAddress("3000000000000000000000000000000000000000000000000000000000000000")

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer1, storerPeer1, _, _ := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))
	psPeer2, storerPeer2, _, _ := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))
	psPeer3, storerPeer3, _, _ := createPushSyncNode(t, peer3, defaultPrices, nil, nil, defaultSigner, withinRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(
		streamtest.WithPeerProtocols(
			map[string]p2p.ProtocolSpec{
				peer1.String(): psPeer1.Protocol(),
				peer2.String(): psPeer2.Protocol(),
				peer3.String(): psPeer3.Protocol(),
			},
		),
		streamtest.WithBaseAddr(pivotNode),
	)

	psPivot, _, _, _ := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner, nil, mock.WithPeers(peer1, peer2, peer3))

	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	waitOnRecordAndTest(t, peer1, recorder, chunk.Address(), chunk.Data())
	waitOnRecordAndTest(t, peer1, recorder, chunk.Address(), nil)
	waitOnRecordAndTest(t, peer2, recorder, chunk.Address(), chunk.Data())
	waitOnRecordAndTest(t, peer2, recorder, chunk.Address(), nil)
	waitOnRecordAndTest(t, peer3, recorder, chunk.Address(), chunk.Data())
	waitOnRecordAndTest(t, peer3, recorder, chunk.Address(), nil)

	want := true

	if got, _ := storerPeer1.Has(context.Background(), chunk.Address()); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	if got, _ := storerPeer2.Has(context.Background(), chunk.Address()); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	if got, _ := storerPeer3.Has(context.Background(), chunk.Address()); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func createPushSyncNode(t *testing.T, addr swarm.Address, prices pricerParameters, recorder *streamtest.Recorder, unwrap func(swarm.Chunk), signer crypto.Signer, bs postage.Storer, mockOpts ...mock.Option) (*pushsync.PushSync, *mocks.MockStorer, *tags.Tags, accounting.Interface) {
	t.Helper()
	mockAccounting := accountingmock.NewAccounting()
	ps, mstorer, ts := createPushSyncNodeWithAccounting(t, addr, prices, recorder, unwrap, signer, mockAccounting, bs, mockOpts...)
	return ps, mstorer, ts, mockAccounting
}

func createPushSyncNodeWithAccounting(t *testing.T, addr swarm.Address, prices pricerParameters, recorder *streamtest.Recorder, unwrap func(swarm.Chunk), signer crypto.Signer, acct accounting.Interface, bs postage.Storer, mockOpts ...mock.Option) (*pushsync.PushSync, *mocks.MockStorer, *tags.Tags) {
	t.Helper()
	logger := log.Noop
	storer := mocks.NewStorer()
	testutil.CleanupCloser(t, storer)

	mockTopology := mock.NewTopologyDriver(mockOpts...)
	mockStatestore := statestore.NewStateStore()
	mtag := tags.NewTags(mockStatestore, logger)

	mockPricer := pricermock.NewMockService(prices.price, prices.peerPrice)

	recorderDisconnecter := streamtest.NewRecorderDisconnecter(recorder)
	if unwrap == nil {
		unwrap = func(swarm.Chunk) {}
	}

	validStamp := func(ch swarm.Chunk, stamp []byte) (swarm.Chunk, error) {
		return ch, nil
	}

	if bs == nil {
		bs = bsMock.New(bsMock.WithIsWithinStorageRadius(false))
	}

	ps := pushsync.New(addr, blockHash.Bytes(), recorderDisconnecter, storer, mockTopology, bs, mtag, true, unwrap, validStamp, logger, acct, mockPricer, signer, nil)
	t.Cleanup(func() { ps.Close() })

	return ps, storer, mtag
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
