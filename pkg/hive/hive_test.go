// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	pb "github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestInit(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	connectionManager := &ConnectionManagerMock{}
	peerSuggester := &PeerSuggesterMock{}
	addressFinder := &AddressFinderMock{}

	// this is the receiving side
	nodeReceiver := New(Options{
		PeerSuggester: peerSuggester,
		AddressFinder: addressFinder,
		Logger:        logger,
	})

	// setup the stream recorder to record stream data
	streamer := streamtest.New(
		streamtest.WithProtocols(nodeReceiver.Protocol()),
	)

	nodeInit := New(Options{
		Streamer:          streamer,
		ConnectionManager: connectionManager,
		Logger:            logger,
	})

	t.Run("OK - node responds with some peers for bin 1 & 2", func(t *testing.T) {
		// prepare expected peers to return from discovery peerer for bins 0 & 1
		discoveryPeers := make(map[int][]p2p.Peer)
		addr1, addr2, addr3 :=
			swarm.MustParseHexAddress("1aaaaaaa"),
			swarm.MustParseHexAddress("1bbbbbbb"),
			swarm.MustParseHexAddress("1ccccccc")
		discoveryPeers[0] = []p2p.Peer{
			{Address: addr1},
			{Address: addr2},
		}
		discoveryPeers[1] = []p2p.Peer{
			{Address: addr3},
		}
		peerSuggester.Peers = discoveryPeers

		// map overlay addresses to multiaddr in address finder
		addresses := make(map[string]ma.Multiaddr)
		addr1Multi, addr2Multi, addr3Multi := newMultiAddr("/ip4/1.1.1.1"),
			newMultiAddr("/ip4/1.1.1.2"),
			newMultiAddr("/ip4/1.1.1.3")

		addresses[addr1.String()] = addr1Multi
		addresses[addr2.String()] = addr2Multi
		addresses[addr3.String()] = addr3Multi
		addressFinder.Underlays = addresses

		initAddr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
		if err := nodeInit.Init(context.Background(), p2p.Peer{
			Address: initAddr,
		}); err != nil {
			t.Fatal(err)
		}

		records, err := streamer.Records(initAddr, protocolName, protocolVersion, peersStreamName)
		if err != nil {
			t.Fatal(err)
		}
		if l := len(records); l != 7 {
			t.Fatalf("got %v records, want %v", l, 1)
		}

		// validate received requestPeers requests
		// there should be maxPO peer requests
		var wantGetPeers []*pb.GetPeers
		var gotGetPeers []*pb.GetPeers
		for i := 0; i < maxPO; i++ {
			wantGetPeers = append(wantGetPeers, &pb.GetPeers{
				Bin:   uint32(i),
				Limit: 10,
			})

			messages, err := readAndAssertMessages(records[i].In(), 1, func() protobuf.Message {
				return new(pb.GetPeers)
			})
			if err != nil {
				t.Fatal(err)
			}

			gotGetPeers = append(gotGetPeers, messages[0].(*pb.GetPeers))
		}

		if fmt.Sprint(gotGetPeers) != fmt.Sprint(wantGetPeers) {
			t.Errorf("requestPeers got %v, want %v", gotGetPeers, wantGetPeers)
		}

		// validate Peers response
		// only response for bin 0, 1 should have appropriate peers, others are empty
		var wantPeers []*pb.Peers
		var gotPeers []*pb.Peers

		wantPeers = append(wantPeers,
			&pb.Peers{Peers: []string{
				addresses[addr1.String()].String(),
				addresses[addr2.String()].String(),
			}},
			&pb.Peers{Peers: []string{addresses[addr3.String()].String()}})

		for i := 0; i < maxPO; i++ {
			if i > 1 {
				wantPeers = append(wantPeers, &pb.Peers{Peers: []string{}})
			}

			messages, err := readAndAssertMessages(records[i].Out(), 1, func() protobuf.Message {
				return new(pb.Peers)
			})
			if err != nil {
				t.Fatal(err)
			}

			gotPeers = append(gotPeers, messages[0].(*pb.Peers))
		}

		if fmt.Sprint(gotPeers) != fmt.Sprint(wantPeers) {
			t.Errorf("Peers got %v, want %v", gotPeers, wantPeers)
		}
	})
}

func readAndAssertMessages(in []byte, expectedLen int, initMsgFunc func() protobuf.Message) ([]protobuf.Message, error) {
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(in),
		initMsgFunc,
	)

	if err != nil {
		return nil, err
	}

	if len(messages) != expectedLen {
		return nil, fmt.Errorf("got %v messages, want %v", len(messages), 1)
	}

	return messages, nil
}

func newMultiAddr(address string) ma.Multiaddr {
	addr, err := ma.NewMultiaddr(address)
	if err != nil {
		panic(err)
	}

	return addr
}

type ConnectionManagerMock struct {
	Err error
}

func (c *ConnectionManagerMock) Connect(ctx context.Context, addr ma.Multiaddr) (overlay swarm.Address, err error) {
	return swarm.Address{}, c.Err
}

type PeerSuggesterMock struct {
	Peers map[int][]p2p.Peer
}

func (p *PeerSuggesterMock) DiscoveryPeers(peer p2p.Peer, bin, limit int) (peers []p2p.Peer) {
	return p.Peers[bin]
}

type AddressFinderMock struct {
	Underlays map[string]ma.Multiaddr
	Err       error
}

func (a *AddressFinderMock) FindAddress(overlay swarm.Address) (underlay ma.Multiaddr, err error) {
	if a.Err != nil {
		return nil, a.Err
	}

	return a.Underlays[overlay.String()], nil
}
