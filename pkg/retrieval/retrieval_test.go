// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math/big"
	"sync"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pricer/headerutils"
	pricermock "github.com/ethersphere/bee/pkg/pricer/mock"
	"github.com/ethersphere/bee/pkg/retrieval"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/storage"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

var testTimeout = 5 * time.Second

// TestDelivery tests that a naive request -> delivery flow works.
func TestDelivery(t *testing.T) {
	var (
		chunk       = testingc.FixtureChunk("0033")
		headlerFunc = func(h p2p.Headers, a swarm.Address) p2p.Headers {
			headers, _ := headerutils.MakePricingResponseHeaders(10, chunk.Address(), 0)
			return headers
		}

		logger               = logging.New(ioutil.Discard, 0)
		mockStorer           = storemock.NewStorer()
		clientMockAccounting = accountingmock.NewAccounting()
		serverMockAccounting = accountingmock.NewAccounting()
		clientAddr           = swarm.MustParseHexAddress("9ee7add8")
		serverAddr           = swarm.MustParseHexAddress("9ee7add7")

		peerFunc = func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			return serverAddr, nil
		}

		price      = uint64(10)
		pricerMock = pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))
	)

	// put testdata in the mock store of the server
	_, err := mockStorer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// create the server that will handle the request and will serve the response
	server := retrieval.New(swarm.MustParseHexAddress("0034"), mockStorer, nil, logger, serverMockAccounting, pricerMock, nil)
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithBaseAddr(clientAddr),
	)

	// client mock storer does not store any data at this point
	// but should be checked at at the end of the test for the
	// presence of the chunk address key and value to ensure delivery
	// was successful
	clientMockStorer := storemock.NewStorer()

	client := retrieval.New(clientAddr, clientMockStorer, recorder, logger, clientMockAccounting, pricerMock, nil)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	v, err := client.RetrieveChunk(ctx, chunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.Data(), chunk.Data()) {
		t.Fatalf("request and response data not equal. got %s want %s", v, chunk.Data())
	}
	records, err := recorder.Records(serverAddr, "retrieval", "1.0.0", "retrieval")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	record := records[0]

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Request) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var reqs []string
	for _, m := range messages {
		reqs = append(reqs, hex.EncodeToString(m.(*pb.Request).Addr))
	}

	if len(reqs) != 1 {
		t.Fatalf("got too many requests. want 1 got %d", len(reqs))
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.Delivery) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotDeliveries []string
	for _, m := range messages {
		gotDeliveries = append(gotDeliveries, string(m.(*pb.Delivery).Data))
	}

	if len(gotDeliveries) != 1 {
		t.Fatalf("got too many deliveries. want 1 got %d", len(gotDeliveries))
	}

	clientBalance, _ := clientMockAccounting.Balance(serverAddr)
	if clientBalance.Int64() != -int64(price) {
		t.Fatalf("unexpected balance on client. want %d got %d", -price, clientBalance)
	}

	serverBalance, _ := serverMockAccounting.Balance(clientAddr)
	if serverBalance.Int64() != int64(price) {
		t.Fatalf("unexpected balance on server. want %d got %d", price, serverBalance)
	}
}

// TestDelivery tests that a naive request -> delivery flow works.
func TestDeliveryWithPriceUpdate(t *testing.T) {
	var (
		price       = uint64(10)
		serverPrice = uint64(17)
		chunk       = testingc.FixtureChunk("0033")

		headlerFunc = func(h p2p.Headers, a swarm.Address) p2p.Headers {
			headers, _ := headerutils.MakePricingResponseHeaders(serverPrice, chunk.Address(), 5)
			return headers
		}

		logger               = logging.New(ioutil.Discard, 0)
		mockStorer           = storemock.NewStorer()
		clientMockAccounting = accountingmock.NewAccounting()
		serverMockAccounting = accountingmock.NewAccounting()
		clientAddr           = swarm.MustParseHexAddress("9ee7add8")
		serverAddr           = swarm.MustParseHexAddress("9ee7add7")

		peerFunc = func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			return serverAddr, nil
		}

		clientPricerMock = pricermock.NewMockService(pricermock.WithPeerPrice(price), pricermock.WithCheapestPeerFunc(peerFunc))
		serverPricerMock = pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithPrice(serverPrice))
	)

	// put testdata in the mock store of the server
	_, err := mockStorer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// create the server that will handle the request and will serve the response
	server := retrieval.New(swarm.MustParseHexAddress("0034"), mockStorer, nil, logger, serverMockAccounting, serverPricerMock, nil)
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithBaseAddr(clientAddr),
	)

	// client mock storer does not store any data at this point
	// but should be checked at at the end of the test for the
	// presence of the chunk address key and value to ensure delivery
	// was successful
	clientMockStorer := storemock.NewStorer()

	client := retrieval.New(clientAddr, clientMockStorer, recorder, logger, clientMockAccounting, clientPricerMock, nil)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	v, err := client.RetrieveChunk(ctx, chunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.Data(), chunk.Data()) {
		t.Fatalf("request and response data not equal. got %s want %s", v, chunk.Data())
	}
	records, err := recorder.Records(serverAddr, "retrieval", "1.0.0", "retrieval")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	record := records[0]

	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.Request) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var reqs []string
	for _, m := range messages {
		reqs = append(reqs, hex.EncodeToString(m.(*pb.Request).Addr))
	}

	if len(reqs) != 1 {
		t.Fatalf("got too many requests. want 1 got %d", len(reqs))
	}

	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.Out()),
		func() protobuf.Message { return new(pb.Delivery) },
	)
	if err != nil {
		t.Fatal(err)
	}
	var gotDeliveries []string
	for _, m := range messages {
		gotDeliveries = append(gotDeliveries, string(m.(*pb.Delivery).Data))
	}

	if len(gotDeliveries) != 1 {
		t.Fatalf("got too many deliveries. want 1 got %d", len(gotDeliveries))
	}

	clientBalance, _ := clientMockAccounting.Balance(serverAddr)
	if clientBalance.Cmp(big.NewInt(-int64(serverPrice))) != 0 {
		t.Fatalf("unexpected balance on client. want %d got %d", -serverPrice, clientBalance)
	}

	serverBalance, _ := serverMockAccounting.Balance(clientAddr)
	if serverBalance.Cmp(big.NewInt(int64(serverPrice))) != 0 {
		t.Fatalf("unexpected balance on server. want %d got %d", serverPrice, serverBalance)
	}
}

func TestRetrieveChunk(t *testing.T) {

	var (
		headlerFunc = func(h p2p.Headers, a swarm.Address) p2p.Headers {
			target, _ := headerutils.ParseTargetHeader(h)
			headers, _ := headerutils.MakePricingResponseHeaders(10, target, 0)
			return headers
		}

		logger = logging.New(ioutil.Discard, 0)

		pricer = pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc))
	)

	// requesting a chunk from downstream peer is expected
	t.Run("downstream", func(t *testing.T) {
		serverAddress := swarm.MustParseHexAddress("03")
		clientAddress := swarm.MustParseHexAddress("01")
		chunk := testingc.FixtureChunk("02c2")

		serverStorer := storemock.NewStorer()
		_, err := serverStorer.Put(context.Background(), storage.ModePutUpload, chunk)
		if err != nil {
			t.Fatal(err)
		}

		server := retrieval.New(serverAddress, serverStorer, nil, logger, accountingmock.NewAccounting(), pricer, nil)
		recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

		peerFunc := func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			return serverAddress, nil
		}

		clientPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))
		client := retrieval.New(clientAddress, nil, recorder, logger, accountingmock.NewAccounting(), clientPricer, nil)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})

	t.Run("forward", func(t *testing.T) {
		chunk := testingc.FixtureChunk("0025")

		serverAddress := swarm.MustParseHexAddress("0100000000000000000000000000000000000000000000000000000000000000")
		forwarderAddress := swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000")
		clientAddress := swarm.MustParseHexAddress("030000000000000000000000000000000000000000000000000000000000000000")

		serverStorer := storemock.NewStorer()
		_, err := serverStorer.Put(context.Background(), storage.ModePutUpload, chunk)
		if err != nil {
			t.Fatal(err)
		}

		server := retrieval.New(
			serverAddress,
			serverStorer, // chunk is in server's store
			nil,
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
		)

		peerFunc := func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			return serverAddress, nil
		}

		forwarderPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))

		forwarder := retrieval.New(
			forwarderAddress,
			storemock.NewStorer(), // no chunk in forwarder's store
			streamtest.New(streamtest.WithProtocols(server.Protocol())), // connect to server
			logger,
			accountingmock.NewAccounting(),
			forwarderPricer,
			nil,
		)

		peerFunc = func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			return forwarderAddress, nil
		}

		clientPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))

		client := retrieval.New(
			clientAddress,
			storemock.NewStorer(), // no chunk in clients's store
			streamtest.New(streamtest.WithProtocols(forwarder.Protocol())), // connect to forwarder
			logger,
			accountingmock.NewAccounting(),
			clientPricer,
			nil,
		)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})
}

func TestRetrievePreemptiveRetry(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	chunk := testingc.FixtureChunk("0025")
	someOtherChunk := testingc.FixtureChunk("0033")

	headlerFunc := func(h p2p.Headers, a swarm.Address) p2p.Headers {
		target, _ := headerutils.ParseTargetHeader(h)
		headers, _ := headerutils.MakePricingResponseHeaders(10, target, 0)
		return headers
	}

	pricerMock := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc))
	price := uint64(10)

	clientAddress := swarm.MustParseHexAddress("1010")

	serverAddress1 := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	serverAddress2 := swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000")

	serverStorer1 := storemock.NewStorer()
	serverStorer2 := storemock.NewStorer()

	// we put some other chunk on server 1
	_, err := serverStorer1.Put(context.Background(), storage.ModePutUpload, someOtherChunk)
	if err != nil {
		t.Fatal(err)
	}
	// we put chunk we need on server 2
	_, err = serverStorer2.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	server1 := retrieval.New(serverAddress1, serverStorer1, nil, logger, accountingmock.NewAccounting(), pricerMock, nil)
	server2 := retrieval.New(serverAddress2, serverStorer2, nil, logger, accountingmock.NewAccounting(), pricerMock, nil)

	t.Run("peer not reachable", func(t *testing.T) {
		ranOnce := true
		ranMux := sync.Mutex{}
		recorder := streamtest.New(
			streamtest.WithProtocols(
				server1.Protocol(),
				server2.Protocol(),
			),
			streamtest.WithMiddlewares(
				func(h p2p.HandlerFunc) p2p.HandlerFunc {
					return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
						ranMux.Lock()
						defer ranMux.Unlock()
						// NOTE: return error for peer1
						if ranOnce {
							ranOnce = false
							return fmt.Errorf("peer not reachable: %s", peer.Address.String())
						}
						return server2.Handler(ctx, peer, stream)
					}
				},
			),
			streamtest.WithBaseAddr(clientAddress),
		)

		var peerFuncCalled bool
		peerFuncCalled = false

		peerFunc := func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			if !peerFuncCalled {
				peerFuncCalled = true
				return serverAddress1, nil
			}
			return serverAddress2, nil
		}

		clientPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))

		client := retrieval.New(
			clientAddress,
			storemock.NewStorer(),
			recorder,
			logger,
			accountingmock.NewAccounting(),
			clientPricer,
			nil)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})

	t.Run("peer does not have chunk", func(t *testing.T) {
		ranOnce := true
		ranMux := sync.Mutex{}
		recorder := streamtest.New(
			streamtest.WithProtocols(
				server1.Protocol(),
				server2.Protocol(),
			),
			streamtest.WithMiddlewares(
				func(h p2p.HandlerFunc) p2p.HandlerFunc {
					return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
						ranMux.Lock()
						defer ranMux.Unlock()
						if ranOnce {
							ranOnce = false
							//return nil
							return server1.Handler(ctx, peer, stream)
						}
						// return nil
						return server2.Handler(ctx, peer, stream)
					}
				},
			),
		)
		var peerFuncCalled bool
		peerFuncCalled = false

		peerFunc := func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			if !peerFuncCalled {
				peerFuncCalled = true
				return serverAddress1, nil
			}
			return serverAddress2, nil
		}

		clientPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))

		client := retrieval.New(
			clientAddress,
			storemock.NewStorer(),
			recorder,
			logger,
			accountingmock.NewAccounting(),
			clientPricer,
			nil)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})

	t.Run("one peer is slower", func(t *testing.T) {
		serverStorer1 := storemock.NewStorer()
		serverStorer2 := storemock.NewStorer()

		// both peers have required chunk
		_, err := serverStorer1.Put(context.Background(), storage.ModePutUpload, chunk)
		if err != nil {
			t.Fatal(err)
		}
		_, err = serverStorer2.Put(context.Background(), storage.ModePutUpload, chunk)
		if err != nil {
			t.Fatal(err)
		}

		server1MockAccounting := accountingmock.NewAccounting()
		server2MockAccounting := accountingmock.NewAccounting()

		server1 := retrieval.New(serverAddress1, serverStorer1, nil, logger, server1MockAccounting, pricerMock, nil)
		server2 := retrieval.New(serverAddress2, serverStorer2, nil, logger, server2MockAccounting, pricerMock, nil)

		// NOTE: must be more than retry duration
		// (here one second more)
		server1ResponseDelayDuration := 6 * time.Second

		ranOnce := true
		ranMux := sync.Mutex{}
		recorder := streamtest.New(
			streamtest.WithProtocols(
				server1.Protocol(),
				server2.Protocol(),
			),
			streamtest.WithMiddlewares(
				func(h p2p.HandlerFunc) p2p.HandlerFunc {
					return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
						ranMux.Lock()
						if ranOnce {
							// NOTE: sleep time must be more than retry duration
							ranOnce = false
							ranMux.Unlock()
							time.Sleep(server1ResponseDelayDuration)
							return server1.Handler(ctx, peer, stream)
						}

						ranMux.Unlock()
						return server2.Handler(ctx, peer, stream)
					}
				},
			),
		)

		clientMockAccounting := accountingmock.NewAccounting()

		var peerFuncCalled bool
		peerFuncCalled = false

		peerFunc := func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			if !peerFuncCalled {
				peerFuncCalled = true
				return serverAddress1, nil
			}
			return serverAddress2, nil
		}

		clientPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))

		client := retrieval.New(clientAddress, nil, recorder, logger, clientMockAccounting, clientPricer, nil)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}

		clientServer1Balance, _ := clientMockAccounting.Balance(serverAddress1)
		if clientServer1Balance.Int64() != 0 {
			t.Fatalf("unexpected balance on client. want %d got %d", -price, clientServer1Balance)
		}

		clientServer2Balance, _ := clientMockAccounting.Balance(serverAddress2)
		if clientServer2Balance.Int64() != -int64(price) {
			t.Fatalf("unexpected balance on client. want %d got %d", -price, clientServer2Balance)
		}

		// wait and check balance again
		// (yet one second more than before, minus original duration)
		time.Sleep(2 * time.Second)

		clientServer1Balance, _ = clientMockAccounting.Balance(serverAddress1)
		if clientServer1Balance.Int64() != -int64(price) {
			t.Fatalf("unexpected balance on client. want %d got %d", -price, clientServer1Balance)
		}

		clientServer2Balance, _ = clientMockAccounting.Balance(serverAddress2)
		if clientServer2Balance.Int64() != -int64(price) {
			t.Fatalf("unexpected balance on client. want %d got %d", -price, clientServer2Balance)
		}
	})

	t.Run("peer forwards request", func(t *testing.T) {
		// server 2 has the chunk
		server2 := retrieval.New(serverAddress2, serverStorer2, nil, logger, accountingmock.NewAccounting(), pricerMock, nil)

		server1Recorder := streamtest.New(
			streamtest.WithProtocols(server2.Protocol()),
		)

		peerFunc := func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			return serverAddress2, nil
		}

		forwardingPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))

		// server 1 will forward request to server 2
		server1 := retrieval.New(serverAddress1, serverStorer1, server1Recorder, logger, accountingmock.NewAccounting(), forwardingPricer, nil)

		clientRecorder := streamtest.New(
			streamtest.WithProtocols(server1.Protocol()),
		)

		peerFunc = func(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
			return serverAddress1, nil
		}

		clientPricer := pricermock.NewMockService(pricermock.WithPriceHeadlerFunc(headlerFunc), pricermock.WithCheapestPeerFunc(peerFunc))

		// client only knows about server 1
		client := retrieval.New(clientAddress, nil, clientRecorder, logger, accountingmock.NewAccounting(), clientPricer, nil)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})
}
