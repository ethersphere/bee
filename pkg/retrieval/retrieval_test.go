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
	"io/ioutil"
	"os"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/retrieval"
	pb "github.com/ethersphere/bee/pkg/retrieval/pb"
	"github.com/ethersphere/bee/pkg/storage"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

var testTimeout = 5 * time.Second

// TestDelivery tests that a naive request -> delivery flow works.
func TestDelivery(t *testing.T) {
	logger := logging.New(os.Stdout, 5)
	mockStorer := storemock.NewStorer()
	chunk := testingc.FixtureChunk("0033")

	// put testdata in the mock store of the server
	_, err := mockStorer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	serverMockAccounting := accountingmock.NewAccounting()

	price := uint64(10)
	pricerMock := accountingmock.NewPricer(price, price)

	// create the server that will handle the request and will serve the response
	server := retrieval.New(swarm.MustParseHexAddress("0034"), mockStorer, nil, nil, logger, serverMockAccounting, pricerMock, nil)
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	clientMockAccounting := accountingmock.NewAccounting()

	// client mock storer does not store any data at this point
	// but should be checked at at the end of the test for the
	// presence of the chunk address key and value to ensure delivery
	// was successful
	clientMockStorer := storemock.NewStorer()

	peerID := swarm.MustParseHexAddress("9ee7add7")
	ps := mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
		_, _, _ = f(peerID, 0)
		return nil
	}}
	client := retrieval.New(swarm.MustParseHexAddress("9ee7add8"), clientMockStorer, recorder, ps, logger, clientMockAccounting, pricerMock, nil)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	v, err := client.RetrieveChunk(ctx, chunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.Data(), chunk.Data()) {
		t.Fatalf("request and response data not equal. got %s want %s", v, chunk.Data())
	}
	records, err := recorder.Records(peerID, "retrieval", "1.0.0", "retrieval")
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

	clientBalance, _ := clientMockAccounting.Balance(peerID)
	if clientBalance != -int64(price) {
		t.Fatalf("unexpected balance on client. want %d got %d", -price, clientBalance)
	}

	serverBalance, _ := serverMockAccounting.Balance(peerID)
	if serverBalance != int64(price) {
		t.Fatalf("unexpected balance on server. want %d got %d", price, serverBalance)
	}
}

func TestRetrieveChunk(t *testing.T) {
	logger := logging.New(os.Stdout, 5)

	pricer := accountingmock.NewPricer(1, 1)

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

		server := retrieval.New(serverAddress, serverStorer, nil, nil, logger, accountingmock.NewAccounting(), pricer, nil)
		recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))

		clientSuggester := mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
			_, _, _ = f(serverAddress, 0)
			return nil
		}}
		client := retrieval.New(clientAddress, nil, recorder, clientSuggester, logger, accountingmock.NewAccounting(), pricer, nil)

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
			nil,
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
		)

		forwarder := retrieval.New(
			forwarderAddress,
			storemock.NewStorer(), // no chunk in forwarder's store
			streamtest.New(streamtest.WithProtocols(server.Protocol())), // connect to server
			mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
				_, _, _ = f(serverAddress, 0) // suggest server's address
				return nil
			}},
			logger,
			accountingmock.NewAccounting(),
			pricer,
			nil,
		)

		client := retrieval.New(
			clientAddress,
			storemock.NewStorer(), // no chunk in clients's store
			streamtest.New(streamtest.WithProtocols(forwarder.Protocol())), // connect to forwarder
			mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
				_, _, _ = f(forwarderAddress, 0) // suggest forwarder's address
				return nil
			}},
			logger,
			accountingmock.NewAccounting(),
			pricer,
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

	price := uint64(1)
	pricerMock := accountingmock.NewPricer(price, price)

	clientAddress := swarm.MustParseHexAddress("1010")

	serverAddress1 := swarm.MustParseHexAddress("0001")
	serverAddress2 := swarm.MustParseHexAddress("0002")
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

	noPeerSuggester := mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
		_, _, _ = f(swarm.ZeroAddress, 0)
		return nil
	}}

	peerSuggesterFn := func(peers ...swarm.Address) topology.EachPeerer {
		if len(peers) == 0 {
			return noPeerSuggester
		}

		peerID := 0
		peerSuggester := mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
			_, _, _ = f(peers[peerID], 0)
			// circulate suggested peers
			peerID++
			if peerID >= len(peers) {
				peerID = 0
			}
			return nil
		}}
		return peerSuggester
	}

	server1 := retrieval.New(serverAddress1, serverStorer1, nil, noPeerSuggester, logger, accountingmock.NewAccounting(), pricerMock, nil)
	server2 := retrieval.New(serverAddress2, serverStorer2, nil, noPeerSuggester, logger, accountingmock.NewAccounting(), pricerMock, nil)

	t.Run("peer not reachable", func(t *testing.T) {
		recorder := streamtest.New(
			streamtest.WithProtocols(
				server1.Protocol(),
				server2.Protocol(),
			),
			streamtest.WithMiddlewares(
				func(h p2p.HandlerFunc) p2p.HandlerFunc {
					return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
						// NOTE: return error for peer1
						if serverAddress1.Equal(peer.Address) {
							return fmt.Errorf("peer not reachable: %s", peer.Address.String())
						}

						if serverAddress2.Equal(peer.Address) {
							return server2.Handler(ctx, peer, stream)
						}

						return fmt.Errorf("unknown peer: %s", peer.Address.String())
					}
				},
			),
		)

		client := retrieval.New(clientAddress, nil, recorder, peerSuggesterFn(peers...), logger, accountingmock.NewAccounting(), pricerMock, nil)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})

	t.Run("peer does not have chunk", func(t *testing.T) {
		recorder := streamtest.New(
			streamtest.WithProtocols(
				server1.Protocol(),
				server2.Protocol(),
			),
			streamtest.WithMiddlewares(
				func(h p2p.HandlerFunc) p2p.HandlerFunc {
					return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
						if serverAddress1.Equal(peer.Address) {
							return server1.Handler(ctx, peer, stream)
						}

						if serverAddress2.Equal(peer.Address) {
							return server2.Handler(ctx, peer, stream)
						}

						return fmt.Errorf("unknown peer: %s", peer.Address.String())
					}
				},
			),
		)

		client := retrieval.New(clientAddress, nil, recorder, peerSuggesterFn(peers...), logger, accountingmock.NewAccounting(), pricerMock, nil)

		got, err := client.RetrieveChunk(context.Background(), chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Data(), chunk.Data()) {
			t.Fatalf("got data %x, want %x", got.Data(), chunk.Data())
		}
	})
}

type mockPeerSuggester struct {
	eachPeerRevFunc func(f topology.EachPeerFunc) error
}

func (s mockPeerSuggester) EachPeer(topology.EachPeerFunc) error {
	return errors.New("not implemented")
}
func (s mockPeerSuggester) EachPeerRev(f topology.EachPeerFunc) error {
	return s.eachPeerRevFunc(f)
}
