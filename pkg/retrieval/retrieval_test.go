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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accounting"
	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/pricer"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	pb "github.com/ethersphere/bee/v2/pkg/retrieval/pb"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	storemock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"

	topologymock "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

var (
	testTimeout  = 5 * time.Second
	defaultPrice = uint64(10)
)

type testStorer struct {
	storage.ChunkStore
}

func (t *testStorer) Lookup() storage.Getter { return t.ChunkStore }

func (t *testStorer) Cache() storage.Putter { return t.ChunkStore }

// TestDelivery tests that a naive request -> delivery flow works.
func TestDelivery(t *testing.T) {
	t.Parallel()

	var (
		chunk                = testingc.FixtureChunk("0033")
		logger               = log.Noop
		mockStorer           = &testStorer{ChunkStore: inmemchunkstore.New()}
		clientMockAccounting = accountingmock.NewAccounting()
		serverMockAccounting = accountingmock.NewAccounting()
		clientAddr           = swarm.MustParseHexAddress("9ee7add8")
		serverAddr           = swarm.MustParseHexAddress("9ee7add7")

		pricerMock = pricermock.NewMockService(defaultPrice, defaultPrice)
	)
	// put testdata in the mock store of the server
	err := mockStorer.Put(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	// create the server that will handle the request and will serve the response
	server := createRetrieval(t, swarm.MustParseHexAddress("0034"), mockStorer, nil, nil, logger, serverMockAccounting, pricerMock, nil, false)
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithBaseAddr(clientAddr),
	)

	// client mock storer does not store any data at this point
	// but should be checked at the end of the test for the
	// presence of the chunk address key and value to ensure delivery
	// was successful
	clientMockStorer := &testStorer{ChunkStore: inmemchunkstore.New()}

	mt := topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddr))

	client := createRetrieval(t, clientAddr, clientMockStorer, recorder, mt, logger, clientMockAccounting, pricerMock, nil, false)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	v, err := client.RetrieveChunk(ctx, chunk.Address(), swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.Data(), chunk.Data()) {
		t.Fatalf("request and response data not equal. got %s want %s", v, chunk.Data())
	}
	records, err := recorder.Records(serverAddr, "retrieval", "1.4.0", "retrieval")
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

func TestWaitForInflight(t *testing.T) {
	t.Parallel()

	var (
		chunk      = testingc.FixtureChunk("7000")
		logger     = log.Noop
		pricerMock = pricermock.NewMockService(defaultPrice, defaultPrice)

		badMockStorer           = &testStorer{ChunkStore: inmemchunkstore.New()}
		badServerMockAccounting = accountingmock.NewAccounting()
		badServerAddr           = swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000")

		mockStorer           = &testStorer{ChunkStore: inmemchunkstore.New()}
		serverMockAccounting = accountingmock.NewAccounting()
		serverAddr           = swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000")

		clientMockStorer     = &testStorer{ChunkStore: inmemchunkstore.New()}
		clientMockAccounting = accountingmock.NewAccounting()
		clientAddr           = swarm.MustParseHexAddress("9ee7add8")
	)

	// put testdata in the mock store of the server
	err := mockStorer.Put(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	// create the server that will handle the request and will serve the response
	server := createRetrieval(t, serverAddr, mockStorer, nil, nil, logger, serverMockAccounting, pricerMock, nil, false)

	badServer := createRetrieval(t, badServerAddr, badMockStorer, nil, nil, logger, badServerMockAccounting, pricerMock, nil, false)

	fail := true
	var lock sync.Mutex

	recorder := streamtest.New(
		streamtest.WithBaseAddr(clientAddr),
		streamtest.WithProtocols(badServer.Protocol(), server.Protocol()),
		streamtest.WithMiddlewares(func(h p2p.HandlerFunc) p2p.HandlerFunc {
			return func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
				lock.Lock()
				defer lock.Unlock()

				if fail {
					fail = false
					s.Close()
					return errors.New("peer not reachable")
				}

				time.Sleep(time.Second * 2)

				if err := h(ctx, p, s); err != nil {
					return err
				}
				// close stream after all previous middlewares wrote to it
				// so that the receiving peer can get all the post messages
				return s.Close()
			}
		}),
	)

	mt := topologymock.NewTopologyDriver(topologymock.WithPeers(badServerAddr, serverAddr))

	client := createRetrieval(t, clientAddr, clientMockStorer, recorder, mt, logger, clientMockAccounting, pricerMock, nil, false)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*30)
	defer cancel()

	v, err := client.RetrieveChunk(ctx, chunk.Address(), swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(v.Data(), chunk.Data()) {
		t.Fatalf("request and response data not equal. got %s want %s", v, chunk.Data())
	}
}

// TestRetrieveChunkPeersExhausted checks that running out of peers is reported
// as not found when at least one peer answered, and as no usable peer otherwise.
// The request is repeated on the same client so that the second one runs while
// the peers from the first one are still skipped.
func TestRetrieveChunkPeersExhausted(t *testing.T) {
	t.Parallel()

	var (
		chunk        = testingc.FixtureChunk("0033")
		clientAddr   = swarm.MustParseHexAddress(strings.Repeat("ff", swarm.HashSize))
		peer1        = swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
		peer2        = swarm.MustParseHexAddress("2000000000000000000000000000000000000000000000000000000000000000")
		pricer       = pricermock.NewMockService(defaultPrice, defaultPrice)
		streamFailed = errors.New("stream failed")
	)

	// server does not have the chunk and has no peers to forward the request to,
	// so it answers every request with a delivery error.
	server := createRetrieval(t, peer1, &testStorer{ChunkStore: inmemchunkstore.New()}, nil,
		topologymock.NewTopologyDriver(), log.Noop, accountingmock.NewAccounting(), pricer, nil, false)

	for _, tc := range []struct {
		name     string
		peers    []swarm.Address
		streamer p2p.Streamer
		want     error
		notWant  error
	}{
		{
			name:     "peers answered",
			peers:    []swarm.Address{peer1, peer2},
			streamer: streamtest.New(streamtest.WithProtocols(server.Protocol())),
			want:     storage.ErrNotFound,
			notWant:  topology.ErrNotFound,
		},
		{
			name:     "no peers",
			streamer: streamtest.New(streamtest.WithProtocols(server.Protocol())),
			want:     topology.ErrNotFound,
			notWant:  storage.ErrNotFound,
		},
		{
			name:  "no peer answered",
			peers: []swarm.Address{peer1, peer2},
			streamer: streamtest.New(streamtest.WithStreamError(func(swarm.Address, string, string, string) error {
				return streamFailed
			})),
			want:    topology.ErrNotFound,
			notWant: storage.ErrNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := createRetrieval(t, clientAddr, nil, tc.streamer,
				topologymock.NewTopologyDriver(topologymock.WithPeers(tc.peers...)), log.Noop,
				accountingmock.NewAccounting(), pricer, nil, false)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for i := range 2 {
				_, err := client.RetrieveChunk(ctx, chunk.Address(), swarm.ZeroAddress)
				if !errors.Is(err, tc.want) {
					t.Fatalf("request %d: got error %v, want %v", i, err, tc.want)
				}
				if errors.Is(err, tc.notWant) {
					t.Fatalf("request %d: got error %v, must not match %v", i, err, tc.notWant)
				}
			}
		})
	}
}

func TestRetrieveChunk(t *testing.T) {
	t.Parallel()

	var (
		logger = log.Noop
		pricer = pricermock.NewMockService(defaultPrice, defaultPrice)
	)

	// requesting a chunk from downstream peer is expected
	t.Run("downstream", func(t *testing.T) {
		t.Parallel()
		t.Skip()

		serverAddress := swarm.MustParseHexAddress("03")
		clientAddress := swarm.MustParseHexAddress("01")
		chunk := testingc.FixtureChunk("02c2")

		serverStorer := &testStorer{ChunkStore: inmemchunkstore.New()}
		err := serverStorer.Put(context.Background(), chunk)
		if err != nil {
			t.Fatal(err)
		}

		server := createRetrieval(t, serverAddress, serverStorer, nil, nil, logger, accountingmock.NewAccounting(), pricer, nil, false)
		recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

		mt := topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddress))

		client := createRetrieval(t, clientAddress, nil, recorder, mt, logger, accountingmock.NewAccounting(), pricer, nil, false)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})

	t.Run("forward", func(t *testing.T) {
		t.Parallel()
		t.Skip()

		chunk := testingc.FixtureChunk("0025")

		serverAddress := swarm.MustParseHexAddress("0100000000000000000000000000000000000000000000000000000000000000")
		forwarderAddress := swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000")
		clientAddress := swarm.MustParseHexAddress("030000000000000000000000000000000000000000000000000000000000000000")

		serverStorer := &testStorer{ChunkStore: inmemchunkstore.New()}
		err := serverStorer.Put(context.Background(), chunk)
		if err != nil {
			t.Fatal(err)
		}

		server := createRetrieval(t,
			serverAddress,
			serverStorer, // chunk is in server's store
			nil,
			nil,
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			false,
		)

		forwarderStore := &testStorer{ChunkStore: inmemchunkstore.New()}

		forwarder := createRetrieval(t,
			forwarderAddress,
			forwarderStore, // no chunk in forwarder's store
			streamtest.New(streamtest.WithProtocols(server.Protocol())), // connect to server
			topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddress)),
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			true, // note explicit caching
		)

		client := createRetrieval(t,
			clientAddress,
			storemock.New(), // no chunk in clients's store
			streamtest.New(streamtest.WithProtocols(forwarder.Protocol())), // connect to forwarder
			topologymock.NewTopologyDriver(topologymock.WithClosestPeer(forwarderAddress)),
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			false,
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

		err = spinlock.Wait(time.Second, func() bool {
			gots, _ := forwarderStore.Has(context.Background(), chunk.Address())
			return gots
		})
		if err != nil {
			t.Fatalf("forwarder did not cache chunk")
		}
	})

	t.Run("propagate error to origin", func(t *testing.T) {
		t.Parallel()

		chunk := testingc.FixtureChunk("0025")

		serverAddress := swarm.MustParseHexAddress("0100000000000000000000000000000000000000000000000000000000000000")
		forwarderAddress := swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000")
		clientAddress := swarm.MustParseHexAddress("030000000000000000000000000000000000000000000000000000000000000000")

		buf := new(bytes.Buffer)
		captureLogger := log.NewLogger("test", log.WithSink(buf))

		server := createRetrieval(t,
			serverAddress,
			&testStorer{ChunkStore: inmemchunkstore.New()},
			nil,
			topologymock.NewTopologyDriver(),
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			false,
		)

		forwarderStore := &testStorer{ChunkStore: inmemchunkstore.New()}

		forwarder := createRetrieval(t,
			forwarderAddress,
			forwarderStore, // no chunk in forwarder's store
			streamtest.New(streamtest.WithProtocols(server.Protocol())), // connect to server
			topologymock.NewTopologyDriver(topologymock.WithClosestPeer(serverAddress)),
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			true, // note explicit caching
		)

		client := createRetrieval(t,
			clientAddress,
			storemock.New(), // no chunk in clients's store
			streamtest.New(streamtest.WithProtocols(forwarder.Protocol())), // connect to forwarder
			topologymock.NewTopologyDriver(topologymock.WithClosestPeer(forwarderAddress)),
			captureLogger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
			false,
		)

		_, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("got error %v, want %v", err, storage.ErrNotFound)
		}

		// the server has no peers and answers the forwarder with a delivery error,
		// so the forwarder reports to the client that the chunk was not found.
		want := p2p.NewChunkDeliveryError("retrieve chunk: " + storage.ErrNotFound.Error())
		if got := buf.String(); !strings.Contains(got, want.Error()) {
			t.Fatalf("got log %s, want %s", got, want)
		}
	})
}

func TestRetrievePreemptiveRetry(t *testing.T) {
	t.Parallel()

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

	serverStorer1 := &testStorer{ChunkStore: inmemchunkstore.New()}
	serverStorer2 := &testStorer{ChunkStore: inmemchunkstore.New()}

	// we put some other chunk on server 1
	err := serverStorer1.Put(context.Background(), someOtherChunk)
	if err != nil {
		t.Fatal(err)
	}
	// we put chunk we need on server 2
	err = serverStorer2.Put(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	noClosestPeer := topologymock.NewTopologyDriver()
	closetPeers := topologymock.NewTopologyDriver(topologymock.WithPeers(peers...))

	server1 := createRetrieval(t, serverAddress1, serverStorer1, nil, noClosestPeer, logger, accountingmock.NewAccounting(), pricerMock, nil, false)
	server2 := createRetrieval(t, serverAddress2, serverStorer2, nil, noClosestPeer, logger, accountingmock.NewAccounting(), pricerMock, nil, false)

	t.Run("peer not reachable", func(t *testing.T) {
		t.Parallel()

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

		client := createRetrieval(t, clientAddress, nil, recorder, closetPeers, logger, accountingmock.NewAccounting(), pricerMock, nil, false)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})

	t.Run("peer does not have chunk", func(t *testing.T) {
		t.Parallel()

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

		client := createRetrieval(t, clientAddress, nil, recorder, closetPeers, logger, accountingmock.NewAccounting(), pricerMock, nil, false)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address(), swarm.ZeroAddress)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})

	t.Run("one peer is slower", func(t *testing.T) {
		t.Parallel()

		serverStorer1 := &testStorer{ChunkStore: inmemchunkstore.New()}
		serverStorer2 := &testStorer{ChunkStore: inmemchunkstore.New()}

		// both peers have required chunk
		err := serverStorer1.Put(context.Background(), chunk)
		if err != nil {
			t.Fatal(err)
		}
		err = serverStorer2.Put(context.Background(), chunk)
		if err != nil {
			t.Fatal(err)
		}

		server1MockAccounting := accountingmock.NewAccounting()
		server2MockAccounting := accountingmock.NewAccounting()

		server1 := createRetrieval(t, serverAddress1, serverStorer1, nil, noClosestPeer, logger, server1MockAccounting, pricerMock, nil, false)
		server2 := createRetrieval(t, serverAddress2, serverStorer2, nil, noClosestPeer, logger, server2MockAccounting, pricerMock, nil, false)

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

		client := createRetrieval(t, clientAddress, nil, recorder, closetPeers, logger, clientMockAccounting, pricerMock, nil, false)

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
		t.Parallel()

		// server 2 has the chunk
		server2 := createRetrieval(t, serverAddress2, serverStorer2, nil, noClosestPeer, logger, accountingmock.NewAccounting(), pricerMock, nil, false)

		server1Recorder := streamtest.New(
			streamtest.WithProtocols(server2.Protocol()),
		)

		// server 1 will forward request to server 2
		server1 := createRetrieval(t, serverAddress1, serverStorer1, server1Recorder, topologymock.NewTopologyDriver(topologymock.WithPeers(serverAddress2)), logger, accountingmock.NewAccounting(), pricerMock, nil, true)

		clientRecorder := streamtest.New(
			streamtest.WithProtocols(server1.Protocol()),
		)

		// client only knows about server 1
		client := createRetrieval(t, clientAddress, nil, clientRecorder, topologymock.NewTopologyDriver(topologymock.WithPeers(serverAddress1)), logger, accountingmock.NewAccounting(), pricerMock, nil, false)

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
		err = spinlock.Wait(time.Second, func() bool {
			has, _ := serverStorer1.Has(context.Background(), chunk.Address())
			return has
		})
		if err != nil {
			t.Fatalf("forwarder node does not have chunk")
		}
	})
}

func TestClosestPeer(t *testing.T) {
	t.Parallel()

	srvAd := swarm.MustParseHexAddress("0100000000000000000000000000000000000000000000000000000000000000")

	addr1 := swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000")
	addr2 := swarm.MustParseHexAddress("0300000000000000000000000000000000000000000000000000000000000000")
	addr3 := swarm.MustParseHexAddress("0400000000000000000000000000000000000000000000000000000000000000")

	ret := createRetrieval(t, srvAd, nil, nil, topologymock.NewTopologyDriver(topologymock.WithPeers(addr1, addr2, addr3)), log.Noop, nil, nil, nil, false)

	t.Run("closest", func(t *testing.T) {
		t.Parallel()

		addr, err := ret.ClosestPeer(addr1, nil, false)
		if err != nil {
			t.Fatal("closest peer", err)
		}
		if !addr.Equal(addr1) {
			t.Fatalf("want %s, got %s", addr1.String(), addr.String())
		}
	})

	t.Run("second closest", func(t *testing.T) {
		t.Parallel()

		addr, err := ret.ClosestPeer(addr1, []swarm.Address{addr1}, false)
		if err != nil {
			t.Fatal("closest peer", err)
		}
		if !addr.Equal(addr2) {
			t.Fatalf("want %s, got %s", addr2.String(), addr.String())
		}
	})

	t.Run("closest is further than base addr", func(t *testing.T) {
		t.Parallel()

		_, err := ret.ClosestPeer(srvAd, nil, false)
		if !errors.Is(err, topology.ErrNotFound) {
			t.Fatal("closest peer", err)
		}
	})
}

func createRetrieval(
	t *testing.T,
	addr swarm.Address,
	storer retrieval.Storer,
	streamer p2p.Streamer,
	chunkPeerer topology.ClosestPeerer,
	logger log.Logger,
	accounting accounting.Interface,
	pricer pricer.Interface,
	tracer *tracing.Tracer,
	forwarderCaching bool,
) *retrieval.Service {
	t.Helper()

	radiusF := func() (uint8, error) { return swarm.MaxBins, nil }

	ret := retrieval.New(addr, radiusF, storer, streamer, chunkPeerer, logger, accounting, pricer, tracer, forwarderCaching)
	t.Cleanup(func() { ret.Close() })
	return ret
}

// TestClosestPeerIgnoresHealth checks that the origin and a forwarder ask the
// closest reachable peer, also when it is not healthy.
func TestClosestPeerIgnoresHealth(t *testing.T) {
	t.Parallel()

	var (
		base      = swarm.MustParseHexAddress("ff00000000000000000000000000000000000000000000000000000000000000")
		nearBase  = swarm.MustParseHexAddress("0180000000000000000000000000000000000000000000000000000000000000") // PO 8
		chunk     = swarm.MustParseHexAddress("0100000000000000000000000000000000000000000000000000000000000000")
		closer    = swarm.MustParseHexAddress("0300000000000000000000000000000000000000000000000000000000000000") // PO 6, not healthy
		samePO    = swarm.MustParseHexAddress("0200000000000000000000000000000000000000000000000000000000000000") // PO 6, healthy, farther than closer
		far       = swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000") // PO 0, healthy
		private   = swarm.MustParseHexAddress("0100000000000000000000000000000000000000000000000000000000000001") // PO 255, not reachable
		unhealthy = map[string]bool{closer.ByteString(): true}
	)

	driver := func(peers ...swarm.Address) topology.ClosestPeerer {
		return topologymock.NewTopologyDriver(topologymock.WithClosestPeerFunc(
			func(addr swarm.Address, _ bool, filter topology.Select, skip ...swarm.Address) (swarm.Address, error) {
				best := swarm.ZeroAddress
				for _, p := range peers {
					if (filter.Healthy && unhealthy[p.ByteString()]) || (filter.Reachable && p.Equal(private)) || swarm.ContainsAddress(skip, p) {
						continue
					}
					if best.IsZero() {
						best = p
						continue
					}
					if ok, _ := p.Closer(addr, best); ok {
						best = p
					}
				}
				if best.IsZero() {
					return swarm.ZeroAddress, topology.ErrNotFound
				}
				return best, nil
			}))
	}

	for _, tc := range []struct {
		name   string
		base   swarm.Address
		peers  []swarm.Address
		origin bool
		want   swarm.Address // zero when no peer is expected
	}{
		{name: "origin, closer peer is not healthy", peers: []swarm.Address{far, closer}, origin: true, want: closer},
		{name: "origin, same proximity order", peers: []swarm.Address{samePO, closer}, origin: true, want: closer},
		{name: "forwarder, closer peer is not healthy", peers: []swarm.Address{far, closer}, origin: false, want: closer},
		{name: "forwarder, same proximity order", peers: []swarm.Address{samePO, closer}, origin: false, want: closer},
		{name: "forwarder, closer to the chunk than every peer", base: nearBase, peers: []swarm.Address{far, closer}, origin: false},
		{name: "origin, closer to the chunk than every peer", base: nearBase, peers: []swarm.Address{far, closer}, origin: true, want: closer},
		{name: "origin, a reachable peer comes before a closer peer that is not reachable", peers: []swarm.Address{far, private}, origin: true, want: far},
		{name: "origin, no reachable peer", peers: []swarm.Address{private}, origin: true, want: private},
		{name: "forwarder, no reachable peer", peers: []swarm.Address{private}, origin: false, want: private},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.base.IsZero() {
				tc.base = base
			}
			ret := createRetrieval(t, tc.base, nil, nil, driver(tc.peers...), log.Noop, nil, nil, nil, false)

			got, err := ret.ClosestPeer(chunk, nil, tc.origin)
			if tc.want.IsZero() {
				if !errors.Is(err, topology.ErrNotFound) {
					t.Fatalf("want %v, got peer %s, error %v", topology.ErrNotFound, got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
		})
	}
}
