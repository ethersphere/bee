// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accounting"
	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	cryptomock "github.com/ethersphere/bee/v2/pkg/crypto/mock"
	"github.com/ethersphere/bee/v2/pkg/depth"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/pushsync/pb"
	"github.com/ethersphere/bee/v2/pkg/soc"
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	fixedPrice                  = uint64(10)
	pushNextClosestSpinlockWait = 5 * time.Second
)

var blockHash = common.HexToHash("0x1")

type pricerParameters struct {
	price     uint64
	peerPrice uint64
}

var (
	defaultPrices = pricerParameters{price: fixedPrice, peerPrice: fixedPrice}
	defaultSigner = func(ch swarm.Chunk) crypto.Signer {
		return cryptomock.New(cryptomock.WithSignFunc(func([]byte) ([]byte, error) {
			key, _ := crypto.GenerateSecp256k1Key()
			signer := crypto.NewDefaultSigner(key)
			signature, _ := signer.Sign(ch.Address().Bytes())

			return signature, nil
		}))
	}
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
	psPeer, _, peerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, pivotStorer, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(chunk), mock.WithClosestPeer(closestPeer))

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

	found, _ := pivotStorer.hasReported(t, chunk.Address())
	if found {
		t.Fatalf("chunk %s reported", chunk.Address())
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
}

// TestSocListener listens all payload of a SOC. This triggers sending a chunk to the closest node
// and expects a receipt. The message is intercepted in the outgoing stream to check for correctness.
func TestSocListener(t *testing.T) {
	t.Parallel()
	defaultSigner := cryptomock.New(cryptomock.WithSignFunc(func(addr []byte) ([]byte, error) {
		key, _ := crypto.GenerateSecp256k1Key()
		signer := crypto.NewDefaultSigner(key)
		signature, _ := signer.Sign(addr)

		return signature, nil
	}))

	// chunk data to upload
	privKey, err := crypto.DecodeSecp256k1PrivateKey(swarm.MustParseHexAddress("b0baf37700000000000000000000000000000000000000000000000000000000").Bytes())
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	chunk1 := testingc.FixtureChunk("7000")
	chunk2 := testingc.FixtureChunk("0033")
	id := make([]byte, swarm.HashSize)
	s1 := soc.New(id, chunk1)
	s2 := soc.New(id, chunk2)
	sch1, err := s1.Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	sch1 = sch1.WithStamp(chunk1.Stamp())
	sch2, err := s2.Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	sch2 = sch2.WithStamp(chunk2.Stamp())
	expectedPayload := chunk1.Data()
	gsocListener := func(soc *soc.SOC) {
		if !bytes.Equal(soc.WrappedChunk().Data(), expectedPayload) {
			t.Fatalf("unexpected SOC payload on GSOC listener. got %s, want %s", soc.WrappedChunk().Data(), expectedPayload)
		}
	}

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000") // binary 1000 -> po 1

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer, _, _ := createGsocPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, _, _ := createGsocPushSyncNode(t, pivotNode, defaultPrices, recorder, gsocListener, defaultSigner, mock.WithClosestPeer(closestPeer))

	// Trigger the sending of chunk to the closest node
	receipt, err := psPivot.PushChunkToClosest(context.Background(), sch1)
	if err != nil {
		t.Fatal(err)
	}

	if !sch1.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// this intercepts the outgoing delivery message
	waitOnRecordAndTest(t, closestPeer, recorder, sch1.Address(), sch1.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, sch1.Address(), nil)

	recorder.Reset()
	expectedPayload = chunk2.Data()

	// Trigger the sending of chunk to the closest node
	receipt, err = psPivot.PushChunkToClosest(context.Background(), sch2)
	if err != nil {
		t.Fatal(err)
	}

	if !sch2.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// this intercepts the outgoing delivery message
	waitOnRecordAndTest(t, closestPeer, recorder, sch2.Address(), sch2.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, sch2.Address(), nil)
}

// TestShallowReceipt forces the peer to send back a shallow receipt to a pushsync request. In return, the origin node returns the error along with the received receipt.
func TestShallowReceipt(t *testing.T) {
	t.Parallel()
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	var highPO uint8 = 31

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer, _ := createPushSyncNodeWithRadius(t, closestPeer, defaultPrices, nil, nil, defaultSigner(chunk), highPO, 0, mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, _ := createPushSyncNodeWithRadius(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(chunk), highPO, 0, mock.WithClosestPeer(closestPeer))

	// Trigger the sending of chunk to the closest node
	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if !errors.Is(err, pushsync.ErrShallowReceipt) {
		t.Fatalf("got %v, want %v", err, pushsync.ErrShallowReceipt)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// this intercepts the outgoing delivery message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), nil)
}

// TestShallowReceiptTolerance sends back a shallow receipt but because of the tolerance level, the origin node accepts the receipts.
func TestShallowReceiptTolerance(t *testing.T) {
	t.Parallel()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	signer := crypto.NewDefaultSigner(key)

	pubKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	closestPeer, err := crypto.NewOverlayAddress(*pubKey, 1, blockHash.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	storerRadius := 2
	chunkProximity := 2

	pivotRadius := 4
	pivotTolerance := uint8(2)

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")

	chunk := testingc.GenerateValidRandomChunkAt(t, closestPeer, chunkProximity)

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer, _ := createPushSyncNodeWithRadius(t, closestPeer, defaultPrices, nil, nil, signer, uint8(storerRadius), 0, mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, _ := createPushSyncNodeWithRadius(t, pivotNode, defaultPrices, recorder, nil, nil, uint8(pivotRadius), pivotTolerance, mock.WithClosestPeer(closestPeer))

	// Trigger the sending of chunk to the closest node
	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}
	if err != nil {
		t.Fatalf("got %v, want %v", err, nil)
	}
	if got := swarm.Proximity(receipt.Address.Bytes(), closestPeer.Bytes()); got < uint8(chunkProximity) {
		t.Fatalf("got %v, want at least %v", got, chunkProximity)
	}

	// this intercepts the outgoing delivery message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), chunk.Data())

	// this intercepts the incoming receipt message
	waitOnRecordAndTest(t, closestPeer, recorder, chunk.Address(), nil)
}

// TestPushChunkToClosest tests the sending of chunk to closest peer from the origination source perspective.
// it also checks whether the tags are incremented properly if they are present
func TestPushChunkToClosest(t *testing.T) {
	t.Parallel()
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000").WithTagID(1)

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")   // base is 0000
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000") // binary 0110 -> po 1
	callbackC := make(chan struct{}, 1)
	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to

	psPeer, _, peerAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, chanFunc(callbackC), defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))

	// pivot node needs the streamer since the chunk is intercepted by
	// the chunk worker, then gets sent by opening a new stream
	psPivot, pivotStorer, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(chunk), mock.WithClosestPeer(closestPeer))

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

	found, count := pivotStorer.hasReported(t, chunk.Address())
	if !found {
		t.Fatalf("chunk %s not reported", chunk.Address())
	}

	if count != 1 {
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

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000").WithTagID(1)

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000

	peer1 := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer1, _, peerAccounting1 := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer2, _, peerAccounting2 := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	fail := true
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
	psPivot, pivotStorer, pivotAccounting := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(chunk), mock.WithPeers(peer1, peer2))

	// Trigger the sending of chunk to the closest node
	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	found, count := pivotStorer.hasReported(t, chunk.Address())
	if !found {
		t.Fatalf("chunk %s not reported", chunk.Address())
	}

	// the write to the first peer might succeed or
	// fail, so it is not guaranteed that two increments
	// are made to Sent. expect >= 1
	if count == 0 {
		t.Fatalf("tags error got %d want >= 1", count)
	}

	waitPushStreamsToBothClosestPeers(t, recorder, peer1, peer2)

	// The storer that applied PrepareDebit books +price toward the pivot.
	peerThatEarnedFromPush(t, peer1, peerAccounting1, peer2, peerAccounting2, pivotNode)
	if !anyPeerHasValidPushStream(t, recorder, []swarm.Address{peer1, peer2}, chunk) {
		t.Fatal("expected at least one closest peer with a full delivery+receipt stream record")
	}
	b1, err := pivotAccounting.Balance(peer1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := pivotAccounting.Balance(peer2)
	if err != nil {
		t.Fatal(err)
	}
	sum := b1.Int64() + b2.Int64()
	if sum != -int64(fixedPrice) {
		t.Fatalf("pivot combined balance toward closest peers: want %d got %d (%s=%d %s=%d)",
			-int64(fixedPrice), sum, peer1, b1.Int64(), peer2, b2.Int64())
	}
}

// peerThatEarnedFromPush asserts exactly one closest peer has balance +fixedPrice toward the pivot (successful storer debit).
func peerThatEarnedFromPush(
	t *testing.T,
	peer1 swarm.Address, peerAcct1 accounting.Interface,
	peer2 swarm.Address, peerAcct2 accounting.Interface,
	pivot swarm.Address,
) {
	t.Helper()
	want := int64(fixedPrice)
	n := 0
	for _, p := range []struct {
		addr swarm.Address
		acct accounting.Interface
	}{{peer1, peerAcct1}, {peer2, peerAcct2}} {
		b, err := p.acct.Balance(pivot)
		if err != nil {
			t.Fatal(err)
		}
		if b.Int64() == want {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one peer with balance +%d toward pivot, got %d", fixedPrice, n)
	}
}

func anyPeerHasValidPushStream(t *testing.T, recorder *streamtest.Recorder, peers []swarm.Address, chunk swarm.Chunk) bool {
	t.Helper()
	for _, p := range peers {
		recs, err := recorder.Records(p, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
		if err != nil {
			continue
		}
		for _, rec := range recs {
			if pushStreamRecordValid(rec, chunk) {
				return true
			}
		}
	}
	return false
}

// waitPushStreamsToBothClosestPeers waits until the recorder shows at least one push stream
// opened to each peer, i.e. the pivot attempted another closest after the first path.
func waitPushStreamsToBothClosestPeers(t *testing.T, recorder *streamtest.Recorder, peer1, peer2 swarm.Address) {
	t.Helper()

	timeout := time.After(pushNextClosestSpinlockWait)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		r1, e1 := recorder.Records(peer1, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
		r2, e2 := recorder.Records(peer2, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
		if e1 == nil && len(r1) > 0 && e2 == nil && len(r2) > 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-timeout:
			t.Fatal("timed out waiting for push streams to both closest peers")
			return
		}
	}
}

func pushStreamRecordValid(rec *streamtest.Record, ch swarm.Chunk) bool {
	in := rec.In()
	if len(in) == 0 {
		return false
	}
	dmsgs, err := protobuf.ReadMessages(
		bytes.NewReader(in),
		func() protobuf.Message { return new(pb.Delivery) },
	)
	if err != nil || len(dmsgs) != 1 {
		return false
	}
	d := dmsgs[0].(*pb.Delivery)
	if !bytes.Equal(d.Address, ch.Address().Bytes()) || !bytes.Equal(d.Data, ch.Data()) {
		return false
	}
	out := rec.Out()
	if len(out) == 0 {
		return false
	}
	rmsgs, err := protobuf.ReadMessages(
		bytes.NewReader(out),
		func() protobuf.Message { return new(pb.Receipt) },
	)
	if err != nil || len(rmsgs) != 1 {
		return false
	}
	rcpt := rmsgs[0].(*pb.Receipt)
	return swarm.NewAddress(rcpt.Address).Equal(ch.Address())
}

func TestPushChunkToClosestErrorAttemptRetry(t *testing.T) {
	t.Parallel()

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000").WithTagID(1)

	// create a pivot node and a mocked closest node
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000

	peer1 := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	peer3 := swarm.MustParseHexAddress("9000000000000000000000000000000000000000000000000000000000000000")
	peer4 := swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000")

	// peer is the node responding to the chunk receipt message
	// mock should return ErrWantSelf since there's no one to forward to
	psPeer1, _, peerAccounting1 := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer2, _, peerAccounting2 := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer3, _, peerAccounting3 := createPushSyncNode(t, peer3, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	psPeer4, _, peerAccounting4 := createPushSyncNode(t, peer4, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

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

	psPivot, pivotStorer := createPushSyncNodeWithAccounting(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(chunk), pivotAccounting, log.Noop, func(*soc.SOC) {}, mock.WithPeers(peer1, peer2, peer3, peer4))

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

	// out of 4, 3 peers should return accounting error. So we should have effectively
	// sent only 1 msg
	found, count := pivotStorer.hasReported(t, chunk.Address())
	if !found {
		t.Fatalf("chunk %s not reported", chunk.Address())
	}
	if count != 1 {
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
	psClosestPeer, _, closestAccounting := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

	// creating the pivot peer
	psPivot, _, pivotAccounting := createPushSyncNode(t, pivotPeer, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithPeers(closestPeer))

	combinedRecorder := streamtest.New(streamtest.WithProtocols(psPivot.Protocol(), psClosestPeer.Protocol()), streamtest.WithBaseAddr(triggerPeer))

	// Creating the trigger peer
	psTriggerPeer, _, triggerAccounting := createPushSyncNode(t, triggerPeer, defaultPrices, combinedRecorder, nil, defaultSigner(chunk), mock.WithPeers(pivotPeer, closestPeer))

	receipt, err := psTriggerPeer.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	// Pivot peer will forward the chunk to its closest peer. Intercept the incoming stream from pivot node and check
	// for the correctness of the chunk
	waitOnRecordAndTest(t, closestPeer, combinedRecorder, chunk.Address(), chunk.Data())

	// Similarly intercept the same incoming stream to see if the closest peer is sending a proper receipt
	waitOnRecordAndTest(t, closestPeer, combinedRecorder, chunk.Address(), nil)

	// In the received stream, check if a receipt is sent from pivot peer and check for its correctness.
	waitOnRecordAndTest(t, pivotPeer, combinedRecorder, chunk.Address(), nil)

	// In pivot peer,  intercept the incoming delivery chunk from the trigger peer and check for correctness
	waitOnRecordAndTest(t, pivotPeer, combinedRecorder, chunk.Address(), chunk.Data())

	balance, err := triggerAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != -int64(fixedPrice) {
		t.Fatalf("unexpected balance on trigger. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = triggerAccounting.Balance(closestPeer)
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

	if balance.Int64() != 0 {
		t.Fatalf("unexpected balance on pivot. want %d got %d", int64(fixedPrice), balance)
	}

	balance, err = pivotAccounting.Balance(closestPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != 0 {
		t.Fatalf("unexpected balance on pivot. want %d got %d", -int64(fixedPrice), balance)
	}

	balance, err = closestAccounting.Balance(pivotPeer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != 0 {
		t.Fatalf("unexpected balance on closest. want %d got %d", int64(fixedPrice), balance)
	}
}

// TestHandlerRejectsInvalidSOC checks that a chunk carrying a well-formed and
// correctly signed SOC payload, which nevertheless does not pass soc.Valid, is
// rejected by the handler and is neither stored nor handed to the gsoc listener.
func TestHandlerRejectsInvalidSOC(t *testing.T) {
	t.Parallel()

	for _, tc := range invalidSOCs(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if soc.Valid(tc.chunk) {
				t.Fatal("test chunk must not be a valid soc")
			}
			if _, err := soc.FromChunk(tc.chunk); err != nil {
				t.Fatalf("test chunk must parse as soc: %v", err)
			}

			pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
			closestPeer := swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000")

			var gsocCalls atomic.Int32
			gsocListener := func(*soc.SOC) { gsocCalls.Add(1) }

			psPeer, peerStorer, _ := createGsocPushSyncNode(t, closestPeer, defaultPrices, nil, gsocListener, defaultSigner(tc.chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))
			recorder := streamtest.New(streamtest.WithProtocols(psPeer.Protocol()), streamtest.WithBaseAddr(pivotNode))
			psPivot, _, _ := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(tc.chunk), mock.WithClosestPeer(closestPeer))

			if _, err := psPivot.PushChunkToClosest(context.Background(), tc.chunk); err == nil {
				t.Error("expected push of invalid soc to be rejected")
			}

			if peerStorer.hasChunk(t, tc.chunk.Address()) {
				t.Error("invalid soc stored in reserve")
			}
			if n := gsocCalls.Load(); n != 0 {
				t.Errorf("gsoc listener called %d times for invalid soc", n)
			}
		})
	}
}

type invalidSOC struct {
	name  string
	chunk swarm.Chunk
}

// invalidSOCs returns chunks whose data is a correctly signed SOC that
// soc.FromChunk accepts but soc.Valid rejects.
func invalidSOCs(t *testing.T) []invalidSOC {
	t.Helper()

	cac := testingc.GenerateTestRandomChunk()

	// a regular SOC sent under an address other than keccak(id, owner)
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, swarm.HashSize)
	sch, err := soc.New(id, cac).Sign(crypto.NewDefaultSigner(key))
	if err != nil {
		t.Fatal(err)
	}
	wrongAddress := swarm.NewChunk(cac.Address(), sch.Data()).WithStamp(postagetesting.MustNewStamp())

	// a replica SOC with the correct address, but whose id does not match the
	// wrapped chunk address
	replicasKey, err := crypto.DecodeSecp256k1PrivateKey(append([]byte{1}, make([]byte, 31)...))
	if err != nil {
		t.Fatal(err)
	}
	replicaID := bytes.Repeat([]byte{0xff}, swarm.HashSize)
	rch, err := soc.New(replicaID, cac).Sign(crypto.NewDefaultSigner(replicasKey))
	if err != nil {
		t.Fatal(err)
	}
	badReplica := rch.WithStamp(postagetesting.MustNewStamp())

	return []invalidSOC{
		{name: "mismatched address", chunk: wrongAddress},
		{name: "replica with mismatched id", chunk: badReplica},
	}
}

func TestPropagateErrMsg(t *testing.T) {
	t.Parallel()
	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	// create a pivot node and a mocked closest node
	triggerPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	pivotPeer := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000")

	faultySigner := cryptomock.New(cryptomock.WithSignFunc(func([]byte) ([]byte, error) {
		return nil, errors.New("simulated error")
	}))

	buf := new(bytes.Buffer)
	captureLogger := log.NewLogger("test", log.WithSink(buf))

	// Create the closest peer
	psClosestPeer, _ := createPushSyncNodeWithAccounting(t, closestPeer, defaultPrices, nil, nil, faultySigner, accountingmock.NewAccounting(), log.Noop, func(*soc.SOC) {}, mock.WithClosestPeerErr(topology.ErrWantSelf))

	// creating the pivot peer
	psPivot, _ := createPushSyncNodeWithAccounting(t, pivotPeer, defaultPrices, nil, nil, defaultSigner(chunk), accountingmock.NewAccounting(), log.Noop, func(*soc.SOC) {}, mock.WithPeers(closestPeer))

	combinedRecorder := streamtest.New(streamtest.WithProtocols(psPivot.Protocol(), psClosestPeer.Protocol()), streamtest.WithBaseAddr(triggerPeer))

	// Creating the trigger peer
	psTriggerPeer, _ := createPushSyncNodeWithAccounting(t, triggerPeer, defaultPrices, combinedRecorder, nil, defaultSigner(chunk), accountingmock.NewAccounting(), captureLogger, func(*soc.SOC) {}, mock.WithPeers(pivotPeer))

	_, err := psTriggerPeer.PushChunkToClosest(context.Background(), chunk)
	if err == nil {
		t.Fatal("should received error")
	}

	want := p2p.NewChunkDeliveryError("receipt signature: simulated error")
	if got := buf.String(); !strings.Contains(got, want.Error()) {
		t.Fatalf("got log %s, want %s", got, want)
	}
}

func TestSignsReceipt(t *testing.T) {
	t.Parallel()

	// chunk data to upload
	chunk := testingc.FixtureChunk("7000")

	invalidSigner := cryptomock.New(cryptomock.WithSignFunc(func([]byte) ([]byte, error) {
		return []byte{1}, nil
	}))

	// create a pivot node and a mocked closest node
	pivotPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
	closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

	// Create the closest peer
	psClosestPeer, _, _ := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, invalidSigner, mock.WithClosestPeerErr(topology.ErrWantSelf))

	closestRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()), streamtest.WithBaseAddr(pivotPeer))

	// creating the pivot peer who will act as a forwarder node with a higher price (17)
	psPivot, _, _ := createPushSyncNode(t, pivotPeer, defaultPrices, closestRecorder, nil, invalidSigner, mock.WithPeers(closestPeer))

	_, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if !errors.Is(err, topology.ErrWantSelf) { // because the receipt are invalid, the node will eventually return it self
		t.Fatal(err)
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
	psPeer1, storerPeer1, _ := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))
	psPeer2, storerPeer2, _ := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))
	psPeer3, storerPeer3, _ := createPushSyncNode(t, peer3, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

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

	psPivot, _, _ := createPushSyncNode(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(chunk), mock.WithPeers(peer1, peer2, peer3))

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

	if got := storerPeer1.hasChunk(t, chunk.Address()); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	if got := storerPeer2.hasChunk(t, chunk.Address()); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	if got := storerPeer3.hasChunk(t, chunk.Address()); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestShallowReceiptUnknownDepth checks that an origin node with an unknown
// network depth checks a receipt only against the radius the storer reports.
func TestShallowReceiptUnknownDepth(t *testing.T) {
	t.Parallel()

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	pubKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	storerAddr, err := crypto.NewOverlayAddress(*pubKey, 1, blockHash.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	chunk := testingc.GenerateValidRandomChunkAt(t, storerAddr, 2)
	po := swarm.Proximity(chunk.Address().Bytes(), storerAddr.Bytes())
	if po >= swarm.MaxPO {
		t.Fatalf("chunk proximity %d leaves no room below the network radius %d", po, swarm.MaxPO)
	}
	signature, err := signer.Sign(chunk.Address().Bytes())
	if err != nil {
		t.Fatal(err)
	}
	receipt := func(storageRadius uint32) *pb.Receipt {
		return &pb.Receipt{
			Address:       chunk.Address().Bytes(),
			Signature:     signature,
			Nonce:         blockHash.Bytes(),
			StorageRadius: storageRadius,
		}
	}

	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")

	psKnown, _ := createPushSyncNodeWithRadius(t, pivotNode, defaultPrices, nil, nil, nil, swarm.MaxPO, 0, mock.WithClosestPeerErr(topology.ErrWantSelf))
	if err := psKnown.CheckReceipt(receipt(uint32(po))); !errors.Is(err, pushsync.ErrShallowReceipt) {
		t.Fatalf("known radius: got %v, want %v", err, pushsync.ErrShallowReceipt)
	}

	unknownRadius := func(context.Context) (uint8, error) { return 0, depth.ErrUnknown }
	psPivot, _ := createPushSyncNodeWithRadiusFunc(t, pivotNode, defaultPrices, nil, nil, nil, unknownRadius, 0, mock.WithClosestPeerErr(topology.ErrWantSelf))

	// the storer reports a radius deeper than the chunk's proximity
	if err := psPivot.CheckReceipt(receipt(uint32(po) + 1)); !errors.Is(err, pushsync.ErrShallowReceipt) {
		t.Fatalf("storer radius above proximity: got %v, want %v", err, pushsync.ErrShallowReceipt)
	}

	// the network radius would reject this receipt, the storer radius accepts it
	if err := psPivot.CheckReceipt(receipt(uint32(po))); err != nil {
		t.Fatalf("storer radius at proximity: got %v, want nil", err)
	}

	if got := counterValue(t, psPivot.ReceiptNetworkCheckSkipped()); got != 2 {
		t.Fatalf("receipt_network_check_skipped_total: got %v, want 2", got)
	}
	if got := counterValue(t, psKnown.ReceiptNetworkCheckSkipped()); got != 0 {
		t.Fatalf("receipt_network_check_skipped_total with a known radius: got %v, want 0", got)
	}
}

// TestMultiplexOffWhenDepthUnknown checks that an origin node with an unknown
// network depth pushes to the closest peer only. TestMultiplePushesAsForwarder
// is the same setup with a known radius, where all three peers receive the chunk.
func TestMultiplexOffWhenDepthUnknown(t *testing.T) {
	t.Parallel()

	chunk := testingc.FixtureChunk("7000")

	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")

	peer1 := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000")
	peer3 := swarm.MustParseHexAddress("3000000000000000000000000000000000000000000000000000000000000000")

	psPeer1, storerPeer1, _ := createPushSyncNode(t, peer1, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))
	psPeer2, storerPeer2, _ := createPushSyncNode(t, peer2, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))
	psPeer3, storerPeer3, _ := createPushSyncNode(t, peer3, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

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

	unknownRadius := func(context.Context) (uint8, error) { return 0, depth.ErrUnknown }
	psPivot, _ := createPushSyncNodeWithRadiusFunc(t, pivotNode, defaultPrices, recorder, nil, defaultSigner(chunk), unknownRadius, 0, mock.WithPeers(peer1, peer2, peer3))

	receipt, err := psPivot.PushChunkToClosest(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}
	if !chunk.Address().Equal(receipt.Address) {
		t.Fatal("invalid receipt")
	}

	waitOnRecordAndTest(t, peer1, recorder, chunk.Address(), chunk.Data())
	waitOnRecordAndTest(t, peer1, recorder, chunk.Address(), nil)

	// without the multiplex retries no other push was started before the
	// receipt returned, and the canceled push context starts none after it
	for _, peer := range []swarm.Address{peer2, peer3} {
		if _, err := recorder.Records(peer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName); !errors.Is(err, streamtest.ErrRecordsNotFound) {
			t.Fatalf("peer %s: got %v, want %v", peer, err, streamtest.ErrRecordsNotFound)
		}
	}

	if !storerPeer1.hasChunk(t, chunk.Address()) {
		t.Fatal("closest peer did not store the chunk")
	}
	if storerPeer2.hasChunk(t, chunk.Address()) || storerPeer3.hasChunk(t, chunk.Address()) {
		t.Fatal("chunk stored by a peer other than the closest")
	}
}

// TestHandlerStoreGateUnknownDepth checks that a forwarder with an unknown
// network depth does not store a chunk it receives as within its area of
// responsibility and forwards it instead. With a known radius of 0 the same
// forwarder stores the chunk itself.
func TestHandlerStoreGateUnknownDepth(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		radiusFunc    func(context.Context) (uint8, error)
		wantForwarded bool
	}{
		{
			name:       "known",
			radiusFunc: func(context.Context) (uint8, error) { return 0, nil },
		},
		{
			name:          "unknown",
			radiusFunc:    func(context.Context) (uint8, error) { return 0, depth.ErrUnknown },
			wantForwarded: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			chunk := testingc.FixtureChunk("7000")

			triggerPeer := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000")
			pivotPeer := swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")
			closestPeer := swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

			psClosestPeer, storerClosestPeer, _ := createPushSyncNode(t, closestPeer, defaultPrices, nil, nil, defaultSigner(chunk), mock.WithClosestPeerErr(topology.ErrWantSelf))

			pivotRecorder := streamtest.New(streamtest.WithProtocols(psClosestPeer.Protocol()), streamtest.WithBaseAddr(pivotPeer))
			psPivot, storerPivot := createPushSyncNodeWithRadiusFunc(t, pivotPeer, defaultPrices, pivotRecorder, nil, defaultSigner(chunk), tc.radiusFunc, 0, mock.WithPeers(closestPeer))

			triggerRecorder := streamtest.New(streamtest.WithProtocols(psPivot.Protocol()), streamtest.WithBaseAddr(triggerPeer))
			psTriggerPeer, _, _ := createPushSyncNode(t, triggerPeer, defaultPrices, triggerRecorder, nil, defaultSigner(chunk), mock.WithPeers(pivotPeer))

			receipt, err := psTriggerPeer.PushChunkToClosest(context.Background(), chunk)
			if err != nil {
				t.Fatal(err)
			}
			if !chunk.Address().Equal(receipt.Address) {
				t.Fatal("invalid receipt")
			}

			// the pivot received the chunk from the trigger and sent back a receipt
			waitOnRecordAndTest(t, pivotPeer, triggerRecorder, chunk.Address(), chunk.Data())
			waitOnRecordAndTest(t, pivotPeer, triggerRecorder, chunk.Address(), nil)

			if got, want := storerPivot.hasChunk(t, chunk.Address()), !tc.wantForwarded; got != want {
				t.Fatalf("pivot stored the chunk: got %v, want %v", got, want)
			}

			if !tc.wantForwarded {
				if _, err := pivotRecorder.Records(closestPeer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName); !errors.Is(err, streamtest.ErrRecordsNotFound) {
					t.Fatalf("closest peer: got %v, want %v", err, streamtest.ErrRecordsNotFound)
				}
				return
			}

			waitOnRecordAndTest(t, closestPeer, pivotRecorder, chunk.Address(), chunk.Data())
			waitOnRecordAndTest(t, closestPeer, pivotRecorder, chunk.Address(), nil)
			if !storerClosestPeer.hasChunk(t, chunk.Address()) {
				t.Fatal("closest peer did not store the forwarded chunk")
			}
		})
	}
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()

	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}

type testStorer struct {
	chunksMu       sync.Mutex
	chunksPut      map[string]swarm.Chunk
	chunksReported map[string]int
}

func (ts *testStorer) ReservePutter() storage.Putter {
	return storage.PutterFunc(
		func(ctx context.Context, chunk swarm.Chunk) error {
			ts.chunksMu.Lock()
			defer ts.chunksMu.Unlock()
			ts.chunksPut[chunk.Address().ByteString()] = chunk
			return nil
		},
	)
}

func (ts *testStorer) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {
	if state != storage.ChunkSent {
		return errors.New("incorrect state")
	}

	ts.chunksMu.Lock()
	defer ts.chunksMu.Unlock()

	count, exists := ts.chunksReported[chunk.Address().ByteString()]
	if exists {
		count++
		ts.chunksReported[chunk.Address().ByteString()] = count
		return nil
	}

	ts.chunksReported[chunk.Address().ByteString()] = 1

	return nil
}

func (ts *testStorer) IsWithinStorageRadius(address swarm.Address) bool { return true }

func (ts *testStorer) StorageRadius() uint8 { return 0 }

func (ts *testStorer) hasChunk(t *testing.T, address swarm.Address) bool {
	t.Helper()

	ts.chunksMu.Lock()
	defer ts.chunksMu.Unlock()

	_, found := ts.chunksPut[address.ByteString()]
	return found
}

func (ts *testStorer) hasReported(t *testing.T, address swarm.Address) (bool, int) {
	t.Helper()

	ts.chunksMu.Lock()
	defer ts.chunksMu.Unlock()

	count, found := ts.chunksReported[address.ByteString()]
	return found, count
}

func createPushSyncNode(
	t *testing.T,
	addr swarm.Address,
	prices pricerParameters,
	recorder *streamtest.Recorder,
	unwrap func(swarm.Chunk),
	signer crypto.Signer,
	mockOpts ...mock.Option,
) (*pushsync.PushSync, *testStorer, accounting.Interface) {
	t.Helper()
	mockAccounting := accountingmock.NewAccounting()
	ps, mstorer := createPushSyncNodeWithAccounting(t, addr, prices, recorder, unwrap, signer, mockAccounting, log.Noop, func(*soc.SOC) {}, mockOpts...)
	return ps, mstorer, mockAccounting
}

func createGsocPushSyncNode(
	t *testing.T,
	addr swarm.Address,
	prices pricerParameters,
	recorder *streamtest.Recorder,
	gsocListener func(*soc.SOC),
	signer crypto.Signer,
	mockOpts ...mock.Option,
) (*pushsync.PushSync, *testStorer, accounting.Interface) {
	t.Helper()
	mockAccounting := accountingmock.NewAccounting()
	ps, mstorer := createPushSyncNodeWithAccounting(t, addr, prices, recorder, nil, signer, mockAccounting, log.Noop, gsocListener, mockOpts...)
	return ps, mstorer, mockAccounting
}

func createPushSyncNodeWithRadius(
	t *testing.T,
	addr swarm.Address,
	prices pricerParameters,
	recorder *streamtest.Recorder,
	unwrap func(swarm.Chunk),
	signer crypto.Signer,
	radius uint8,
	shallowReceiptTolerance uint8,
	mockOpts ...mock.Option,
) (*pushsync.PushSync, *testStorer) {
	t.Helper()
	radiusFunc := func(context.Context) (uint8, error) { return radius, nil }
	return createPushSyncNodeWithRadiusFunc(t, addr, prices, recorder, unwrap, signer, radiusFunc, shallowReceiptTolerance, mockOpts...)
}

func createPushSyncNodeWithRadiusFunc(
	t *testing.T,
	addr swarm.Address,
	prices pricerParameters,
	recorder *streamtest.Recorder,
	unwrap func(swarm.Chunk),
	signer crypto.Signer,
	radiusFunc func(context.Context) (uint8, error),
	shallowReceiptTolerance uint8,
	mockOpts ...mock.Option,
) (*pushsync.PushSync, *testStorer) {
	t.Helper()
	storer := &testStorer{
		chunksPut:      make(map[string]swarm.Chunk),
		chunksReported: make(map[string]int),
	}

	mockTopology := mock.NewTopologyDriver(mockOpts...)
	mockPricer := pricermock.NewMockService(prices.price, prices.peerPrice)

	recorderDisconnecter := streamtest.NewRecorderDisconnecter(recorder)
	if unwrap == nil {
		unwrap = func(swarm.Chunk) {}
	}

	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) {
		return ch, nil
	}

	ps := pushsync.New(addr, 1, blockHash.Bytes(), recorderDisconnecter, storer, radiusFunc, mockTopology, true, unwrap, func(*soc.SOC) {}, validStamp, log.Noop, accountingmock.NewAccounting(), mockPricer, signer, nil, stabilmock.NewSubscriber(true), shallowReceiptTolerance)
	t.Cleanup(func() { ps.Close() })

	return ps, storer
}

func createPushSyncNodeWithAccounting(
	t *testing.T,
	addr swarm.Address,
	prices pricerParameters,
	recorder *streamtest.Recorder,
	unwrap func(swarm.Chunk),
	signer crypto.Signer,
	acct accounting.Interface,
	logger log.Logger,
	gsocListener func(*soc.SOC),
	mockOpts ...mock.Option,
) (*pushsync.PushSync, *testStorer) {
	t.Helper()
	storer := &testStorer{
		chunksPut:      make(map[string]swarm.Chunk),
		chunksReported: make(map[string]int),
	}

	mockTopology := mock.NewTopologyDriver(mockOpts...)
	mockPricer := pricermock.NewMockService(prices.price, prices.peerPrice)

	recorderDisconnecter := streamtest.NewRecorderDisconnecter(recorder)
	if unwrap == nil {
		unwrap = func(swarm.Chunk) {}
	}
	if gsocListener == nil {
		gsocListener = func(*soc.SOC) {}
	}

	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) {
		return ch, nil
	}

	radiusFunc := func(context.Context) (uint8, error) { return 0, nil }

	ps := pushsync.New(addr, 1, blockHash.Bytes(), recorderDisconnecter, storer, radiusFunc, mockTopology, true, unwrap, gsocListener, validStamp, logger, acct, mockPricer, signer, nil, stabilmock.NewSubscriber(true), 0)
	t.Cleanup(func() { ps.Close() })

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

func chanFunc(c chan<- struct{}) func(swarm.Chunk) {
	return func(_ swarm.Chunk) {
		c <- struct{}{}
	}
}
