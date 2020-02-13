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

	t.Run("OK", func(t *testing.T) {
		suggesterPeers := make(map[int][]p2p.Peer)
		addr1, addr2, addr3 :=
			swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59a"),
			swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59b"),
			swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")

		suggesterPeers[0] = []p2p.Peer{
			{Address: addr1},
			{Address: addr2},
		}
		suggesterPeers[1] = []p2p.Peer{
			{Address: addr3},
		}

		peerSuggester.Peers = suggesterPeers
		addresses := make(map[string]string)
		addresses[addr1.String()] = string(addr1.Bytes())
		addresses[addr2.String()] = string(addr2.Bytes())
		addresses[addr3.String()] = string(addr3.Bytes())
		addressFinder.Underlays = addresses

		initAddr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
		err := nodeInit.Init(context.Background(), p2p.Peer{
			Address: initAddr,
		})

		if err != nil {
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
		var wantGetPeers []*pb.GetPeers
		var gotGetPeers []*pb.GetPeers
		for i := 0; i < maxPO; i++ {
			wantGetPeers = append(wantGetPeers, &pb.GetPeers{
				Bin:   uint32(i),
				Limit: 10,
			})

			messages, err := protobuf.ReadMessages(
				bytes.NewReader(records[i].In()),
				func() protobuf.Message { return new(pb.GetPeers) },
			)

			if err != nil {
				t.Fatal(err)
			}
			if len(messages) != 1 {
				t.Fatalf("got %v messages, want %v", len(messages), 1)
			}

			for _, m := range messages {
				gotGetPeers = append(gotGetPeers, m.(*pb.GetPeers))
			}
		}

		if fmt.Sprint(gotGetPeers) != fmt.Sprint(wantGetPeers) {
			t.Errorf("requestPeers got %v, want %v", gotGetPeers, wantGetPeers)
		}

		// validate Peers response
		var wantPeers []*pb.Peers
		var gotPeers []*pb.Peers

		wantPeers = append(wantPeers,
			&pb.Peers{Peers: []string{
				addresses[addr1.String()],
				addresses[addr2.String()],
			}},
			&pb.Peers{Peers: []string{addresses[addr3.String()]}})

		for i := 0; i < maxPO; i++ {
			if i > 1 {
				wantPeers = append(wantPeers, &pb.Peers{Peers: []string{}})
			}

			messages, err := protobuf.ReadMessages(
				bytes.NewReader(records[i].Out()),
				func() protobuf.Message { return new(pb.Peers) },
			)

			if err != nil {
				t.Fatal(err)
			}
			if len(messages) != 1 {
				t.Fatalf("got %v messages, want %v", len(messages), 1)
			}

			for _, m := range messages {
				gotPeers = append(gotPeers, m.(*pb.Peers))
			}
		}

		if fmt.Sprint(gotPeers) != fmt.Sprint(wantPeers) {
			t.Errorf("Peers got %v, want %v", gotPeers, wantPeers)
		}

	})
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

func (p *PeerSuggesterMock) DiscoveryPeer(peer p2p.Peer, bin, limit int) (peers []p2p.Peer) {
	return p.Peers[bin]
}

type AddressFinderMock struct {
	Underlays map[string]string
	Err       error
}

func (a *AddressFinderMock) FindAddress(overlay swarm.Address) (underlay string, err error) {
	if a.Err != nil {
		return "", a.Err
	}

	return a.Underlays[overlay.String()], nil
}
