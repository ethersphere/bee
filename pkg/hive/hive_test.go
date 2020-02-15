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

	"github.com/ethersphere/bee/pkg/addressbook/inmem"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	pb "github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestInit(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	connecter := &ConnecterMock{}
	peerer := mock.NewPeerer()
	addressBook := inmem.New()

	// this is the receiving side
	nodeReceiver := New(Options{
		Peerer:      peerer,
		AddressBook: addressBook,
		Logger:      logger,
	})

	// setup the stream recorder to record stream data
	streamer := streamtest.New(
		streamtest.WithProtocols(nodeReceiver.Protocol()),
	)

	nodeInit := New(Options{
		Streamer:  streamer,
		Connecter: connecter,
		Logger:    logger,
	})

	overlays := []swarm.Address{swarm.MustParseHexAddress("1aaaaaaa"),
		swarm.MustParseHexAddress("1bbbbbbb"),
		swarm.MustParseHexAddress("1ccccccc")}
	underlays := []ma.Multiaddr{newMultiAddr("/ip4/1.1.1.1"),
		newMultiAddr("/ip4/1.1.1.2"),
		newMultiAddr("/ip4/1.1.1.3")}
	testPeer := p2p.Peer{Address: swarm.MustParseHexAddress("ca1e9f3a")}

	//populate discovery peerer for bin 1 & 2
	peerer.Add(testPeer, 0, p2p.Peer{Address: overlays[0]}, p2p.Peer{Address: overlays[1]})
	peerer.Add(testPeer, 1, p2p.Peer{Address: overlays[2]})

	// populate address book
	for i := 0; i < len(overlays); i++ {
		addressBook.Put(overlays[i], underlays[i])
	}

	t.Run("OK - node responds with some peers for bin 1 & 2", func(t *testing.T) {
		if err := nodeInit.Init(context.Background(), testPeer); err != nil {
			t.Fatal(err)
		}

		records, err := streamer.Records(testPeer.Address, protocolName, protocolVersion, peersStreamName)
		if err != nil {
			t.Fatal(err)
		}
		if l := len(records); l != maxPO {
			t.Fatalf("got %v records, want %v", l, maxPO)
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

		for i := 0; i < maxPO; i++ {
			p := peerer.Peers(testPeer, i, 0)
			var addrs []string
			for _, addr := range p {
				underlay, exists := addressBook.Get(addr.Address)
				if !exists {
					t.Fatalf("underlay not found")
				}

				addrs = append(addrs, underlay.String())
			}
			wantPeers = append(wantPeers, &pb.Peers{Peers: addrs})

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

type ConnecterMock struct {
	Err error
}

func (c *ConnecterMock) Connect(ctx context.Context, addr ma.Multiaddr) (overlay swarm.Address, err error) {
	return swarm.Address{}, c.Err
}
