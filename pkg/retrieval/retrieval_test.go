// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/topology"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	pricermock "github.com/ethersphere/bee/pkg/pricer/mock"
	"github.com/ethersphere/bee/pkg/retrieval"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/storage"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"

	topologymock "github.com/ethersphere/bee/pkg/topology/mock"
)

var (
	testTimeout  = 5 * time.Second
	defaultPrice = uint64(10)
)

// TestDelivery tests that a naive request -> delivery flow works.
func TestDelivery(t *testing.T) {
	var (
		chunk                = testingc.FixtureChunk("0033")
		logger               = log.Noop
		mockStorer           = storemock.NewStorer()
		clientMockAccounting = accountingmock.NewAccounting()
		serverMockAccounting = accountingmock.NewAccounting()
		clientAddr           = swarm.MustParseHexAddress("9ee7add8")
		serverAddr           = swarm.MustParseHexAddress("9ee7add7")

		pricerMock = pricermock.NewMockService(defaultPrice, defaultPrice)
	)
	stamp, err := chunk.Stamp().MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	// put testdata in the mock store of the server
	_, err = mockStorer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	// create the server that will handle the request and will serve the response
	server := retrieval.New(swarm.MustParseHexAddress("0034"), mockStorer, nil, nil, logger, serverMockAccounting, pricerMock, nil, false, noopStampValidator)
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithBaseAddr(clientAddr),
	)

	// client mock storer does not store any data at this point
	// but should be checked at at the end of the test for the
	// presence of the chunk address key and value to ensure delivery
	// was successful
	clientMockStorer := storemock.NewStorer()

	mt := topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddr))

	client := retrieval.New(clientAddr, clientMockStorer, recorder, mt, logger, clientMockAccounting, pricerMock, nil, false, noopStampValidator)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	v, err := client.RetrieveChunk(ctx, chunk.Address(), swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.Data(), chunk.Data()) {
		t.Fatalf("request and response data not equal. got %s want %s", v, chunk.Data())
	}
	vstamp, err := v.Stamp().MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(vstamp, stamp) {
		t.Fatal("stamp mismatch")
	}
	records, err := recorder.Records(serverAddr, "retrieval", "1.2.0", "retrieval")
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
	reqs := make([]string, 0, len(messages))
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

	gotDeliveries := make([]string, 0, len(messages))
	for _, m := range messages {
		gotDeliveries = append(gotDeliveries, string(m.(*pb.Delivery).Data))
	}

	if len(gotDeliveries) != 1 {
		t.Fatalf("got too many deliveries. want 1 got %d", len(gotDeliveries))
	}

	clientBalance, _ := clientMockAccounting.Balance(serverAddr)
	if clientBalance.Int64() != -int64(defaultPrice) {
		t.Fatalf("unexpected balance on client. want %d got %d", -defaultPrice, clientBalance)
	}

	serverBalance, _ := serverMockAccounting.Balance(clientAddr)
	if serverBalance.Int64() != int64(defaultPrice) {
		t.Fatalf("unexpected balance on server. want %d got %d", defaultPrice, serverBalance)
	}
}

func TestRetrieveChunk(t *testing.T) {

	var (
		logger = log.Noop
		pricer = pricermock.NewMockService(defaultPrice, defaultPrice)
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

		server := retrieval.New(serverAddress, serverStorer, nil, nil, logger, accountingmock.NewAccounting(), pricer, nil, false, noopStampValidator)
		recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

		mt := topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddress))

		client := retrieval.New(clientAddress, nil, recorder, mt, logger, accountingmock.NewAccounting(), pricer, nil, false, noopStampValidator)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
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
			nil,
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			false,
			noopStampValidator,
		)

		forwarderStore := storemock.NewStorer()

		forwarder := retrieval.New(
			forwarderAddress,
			forwarderStore, // no chunk in forwarder's store
			streamtest.New(streamtest.WithProtocols(server.Protocol())), // connect to server
			topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddress)),
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			true, // note explicit caching
			noopStampValidator,
		)

		client := retrieval.New(
			clientAddress,
			storemock.NewStorer(), // no chunk in clients's store
			streamtest.New(streamtest.WithProtocols(forwarder.Protocol())), // connect to forwarder
			topologymock.NewTopologyDriver(topologymock.WithClosestPeer(forwarderAddress)),
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			false,
			noopStampValidator,
		)

		if got, _ := forwarderStore.Has(context.Background(), chunk.Address()); got {
			t.Fatalf("forwarder node already has chunk")
		}

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
		gots := false
		for i := 0; i < 100; i++ {
			if gots, _ = forwarderStore.Has(context.Background(), chunk.Address()); gots {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !gots {
			t.Fatalf("forwarder did not cache chunk")
		}
	})
}

func TestRetrievePreemptiveRetry(t *testing.T) {
	logger := log.Noop

	chunk := testingc.FixtureChunk("0025")
	someOtherChunk := testingc.FixtureChunk("0033")

	pricerMock := pricermock.NewMockService(defaultPrice, defaultPrice)

	clientAddress := swarm.MustParseHexAddress("1010")

	serverAddress1 := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	serverAddress2 := swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000")
	peers := []swarm.Address{
		serverAddress1,
		serverAddress2,
	}

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

	noClosestPeer := topologymock.NewTopologyDriver()
	closetPeers := topologymock.NewTopologyDriver(topologymock.WithPeers(peers...))

	server1 := retrieval.New(serverAddress1, serverStorer1, nil, noClosestPeer, logger, accountingmock.NewAccounting(), pricerMock, nil, false, noopStampValidator)
	server2 := retrieval.New(serverAddress2, serverStorer2, nil, noClosestPeer, logger, accountingmock.NewAccounting(), pricerMock, nil, false, noopStampValidator)

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

		client := retrieval.New(clientAddress, nil, recorder, closetPeers, logger, accountingmock.NewAccounting(), pricerMock, nil, false, noopStampValidator)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
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
							return server1.Handler(ctx, peer, stream)
						}

						return server2.Handler(ctx, peer, stream)
					}
				},
			),
		)

		client := retrieval.New(clientAddress, nil, recorder, closetPeers, logger, accountingmock.NewAccounting(), pricerMock, nil, false, noopStampValidator)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
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

		server1 := retrieval.New(serverAddress1, serverStorer1, nil, noClosestPeer, logger, server1MockAccounting, pricerMock, nil, false, noopStampValidator)
		server2 := retrieval.New(serverAddress2, serverStorer2, nil, noClosestPeer, logger, server2MockAccounting, pricerMock, nil, false, noopStampValidator)

		// NOTE: must be more than retry duration
		// (here one second more)
		server1ResponseDelayDuration := 2 * time.Second

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
							// server2 is picked first because it's address is closer to the chunk than server1
							return server2.Handler(ctx, peer, stream)
						}
						ranMux.Unlock()

						return server1.Handler(ctx, peer, stream)
					}
				},
			),
		)

		clientMockAccounting := accountingmock.NewAccounting()

		client := retrieval.New(clientAddress, nil, recorder, closetPeers, logger, clientMockAccounting, pricerMock, nil, false, noopStampValidator)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}

		clientServer1Balance, _ := clientMockAccounting.Balance(serverAddress1)
		if clientServer1Balance.Int64() != -int64(defaultPrice) {
			t.Fatalf("unexpected balance on client. want %d got %d", -int64(defaultPrice), clientServer1Balance)
		}

		clientServer2Balance, _ := clientMockAccounting.Balance(serverAddress2)
		if clientServer2Balance.Int64() != 0 {
			t.Fatalf("unexpected balance on client. want %d got %d", 0, clientServer2Balance)
		}

		// wait and check balance again
		// (yet one second more than before, minus original duration)
		time.Sleep(2 * time.Second)

		clientServer1Balance, _ = clientMockAccounting.Balance(serverAddress1)
		if clientServer1Balance.Int64() != -int64(defaultPrice) {
			t.Fatalf("unexpected balance on client. want %d got %d", -int64(defaultPrice), clientServer1Balance)
		}

		clientServer2Balance, _ = clientMockAccounting.Balance(serverAddress2)
		if clientServer2Balance.Int64() != -int64(defaultPrice) {
			t.Fatalf("unexpected balance on client. want %d got %d", -int64(defaultPrice), clientServer2Balance)
		}
	})

	t.Run("peer forwards request", func(t *testing.T) {
		// server 2 has the chunk
		server2 := retrieval.New(serverAddress2, serverStorer2, nil, noClosestPeer, logger, accountingmock.NewAccounting(), pricerMock, nil, false, noopStampValidator)

		server1Recorder := streamtest.New(
			streamtest.WithProtocols(server2.Protocol()),
		)

		// server 1 will forward request to server 2
		server1 := retrieval.New(serverAddress1, serverStorer1, server1Recorder, topologymock.NewTopologyDriver(topologymock.WithPeers(serverAddress2)), logger, accountingmock.NewAccounting(), pricerMock, nil, true, noopStampValidator)

		clientRecorder := streamtest.New(
			streamtest.WithProtocols(server1.Protocol()),
		)

		// client only knows about server 1
		client := retrieval.New(clientAddress, nil, clientRecorder, topologymock.NewTopologyDriver(topologymock.WithPeers(serverAddress1)), logger, accountingmock.NewAccounting(), pricerMock, nil, false, noopStampValidator)

		if got, _ := serverStorer1.Has(context.Background(), chunk.Address()); got {
			t.Fatalf("forwarder node already has chunk")
		}

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
		has := false
		for i := 0; i < 10; i++ {
			has, _ = serverStorer1.Has(context.Background(), chunk.Address())
			if has {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !has {
			t.Fatalf("forwarder node does not have chunk")
		}
	})
}

func TestClosestPeer(t *testing.T) {

	srvAd := swarm.MustParseHexAddress("0100000000000000000000000000000000000000000000000000000000000000")

	addr1 := swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000")
	addr2 := swarm.MustParseHexAddress("0300000000000000000000000000000000000000000000000000000000000000")
	addr3 := swarm.MustParseHexAddress("0400000000000000000000000000000000000000000000000000000000000000")

	ret := retrieval.New(srvAd, nil, nil, topologymock.NewTopologyDriver(topologymock.WithPeers(addr1, addr2, addr3)), log.Noop, nil, nil, nil, false, nil)

	t.Run("closest", func(t *testing.T) {
		addr, err := ret.ClosestPeer(addr1, nil, false)
		if err != nil {
			t.Fatal("closest peer", err)
		}
		if !addr.Equal(addr1) {
			t.Fatalf("want %s, got %s", addr1.String(), addr.String())
		}
	})

	t.Run("second closest", func(t *testing.T) {
		addr, err := ret.ClosestPeer(addr1, []swarm.Address{addr1}, false)
		if err != nil {
			t.Fatal("closest peer", err)
		}
		if !addr.Equal(addr2) {
			t.Fatalf("want %s, got %s", addr2.String(), addr.String())
		}
	})

	t.Run("closest is further than base addr", func(t *testing.T) {
		_, err := ret.ClosestPeer(srvAd, nil, false)
		if !errors.Is(err, topology.ErrNotFound) {
			t.Fatal("closest peer", err)
		}
	})
}

var noopStampValidator = func(chunk swarm.Chunk, stampBytes []byte) (swarm.Chunk, error) {
	return chunk, nil
}
