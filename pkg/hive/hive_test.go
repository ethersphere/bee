// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	book "github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type AddressExporter interface {
	Overlays() ([]swarm.Address, error)
	Multiaddresses() ([]ma.Multiaddr, error)
}

func TestBroadcastPeers(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	logger := logging.New(ioutil.Discard, 0)
	inmem := mock.NewStateStore()
	addressbook := book.New(inmem)

	// populate all expected and needed random resources for 2 full batches
	// tests cases that uses fewer resources can use sub-slices of this data
	var multiaddrs []ma.Multiaddr
	var addrs []swarm.Address
	var wantMsgs []pb.Peers

	for i := 0; i < 2; i++ {
		wantMsgs = append(wantMsgs, pb.Peers{Peers: []*pb.BzzAddress{}})
	}

	for i := 0; i < 2*hive.MaxBatchSize; i++ {
		ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}

		multiaddrs = append(multiaddrs, ma)
		addrs = append(addrs, swarm.NewAddress(createRandomBytes()))
		_, err = addressbook.Put(addrs[i], multiaddrs[i])
		if err != nil {
			t.Fatal(err)
		}
		wantMsgs[i/hive.MaxBatchSize].Peers = append(wantMsgs[i/hive.MaxBatchSize].Peers, &pb.BzzAddress{Overlay: addrs[i].Bytes(), Underlay: multiaddrs[i].String()})
	}

	testCases := map[string]struct {
		addresee           swarm.Address
		peers              []swarm.Address
		wantMsgs           []pb.Peers
		wantOverlays       []swarm.Address
		wantMultiAddresses []ma.Multiaddr
	}{
		"OK - single record": {
			addresee:           swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:              []swarm.Address{addrs[0]},
			wantMsgs:           []pb.Peers{{Peers: wantMsgs[0].Peers[:1]}},
			wantOverlays:       []swarm.Address{addrs[0]},
			wantMultiAddresses: []ma.Multiaddr{multiaddrs[0]},
		},
		"OK - single batch - multiple records": {
			addresee:           swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:              addrs[:15],
			wantMsgs:           []pb.Peers{{Peers: wantMsgs[0].Peers[:15]}},
			wantOverlays:       addrs[:15],
			wantMultiAddresses: multiaddrs[:15],
		},
		"OK - single batch - max number of records": {
			addresee:           swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:              addrs[:hive.MaxBatchSize],
			wantMsgs:           []pb.Peers{{Peers: wantMsgs[0].Peers[:hive.MaxBatchSize]}},
			wantOverlays:       addrs[:hive.MaxBatchSize],
			wantMultiAddresses: multiaddrs[:hive.MaxBatchSize],
		},
		"OK - multiple batches": {
			addresee:           swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:              addrs[:hive.MaxBatchSize+10],
			wantMsgs:           []pb.Peers{{Peers: wantMsgs[0].Peers}, {Peers: wantMsgs[1].Peers[:10]}},
			wantOverlays:       addrs[:hive.MaxBatchSize+10],
			wantMultiAddresses: multiaddrs[:hive.MaxBatchSize+10],
		},
		"OK - multiple batches - max number of records": {
			addresee:           swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:              addrs[:2*hive.MaxBatchSize],
			wantMsgs:           []pb.Peers{{Peers: wantMsgs[0].Peers}, {Peers: wantMsgs[1].Peers}},
			wantOverlays:       addrs[:2*hive.MaxBatchSize],
			wantMultiAddresses: multiaddrs[:2*hive.MaxBatchSize],
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			addressbookclean := book.New(mock.NewStateStore())

			exporter, ok := addressbookclean.(AddressExporter)
			if !ok {
				t.Fatal("could not type assert AddressExporter")
			}

			// create a hive server that handles the incoming stream
			server := hive.New(hive.Options{
				Logger:      logger,
				AddressBook: addressbookclean,
			})

			// setup the stream recorder to record stream data
			recorder := streamtest.New(
				streamtest.WithProtocols(server.Protocol()),
			)

			// create a hive client that will do broadcast
			client := hive.New(hive.Options{
				Streamer:    recorder,
				Logger:      logger,
				AddressBook: addressbook,
			})

			if err := client.BroadcastPeers(context.Background(), tc.addresee, tc.peers...); err != nil {
				t.Fatal(err)
			}

			// get a record for this stream
			records, err := recorder.Records(tc.addresee, "hive", "1.0.0", "peers")
			if err != nil {
				t.Fatal(err)
			}
			if l := len(records); l != len(tc.wantMsgs) {
				t.Fatalf("got %v records, want %v", l, len(tc.wantMsgs))
			}

			// there is a one record per batch (wantMsg)
			for i, record := range records {
				messages, err := readAndAssertPeersMsgs(record.In(), 1)
				if err != nil {
					t.Fatal(err)
				}

				if fmt.Sprint(messages[0]) != fmt.Sprint(tc.wantMsgs[i]) {
					t.Errorf("Messages got %v, want %v", messages, tc.wantMsgs)
				}
			}

			expectOverlaysEventually(t, exporter, tc.wantOverlays)
			expectMultiaddresessEventually(t, exporter, tc.wantMultiAddresses)
		})
	}
}

func expectOverlaysEventually(t *testing.T, exporter AddressExporter, wantOverlays []swarm.Address) {
	for i := 0; i < 100; i++ {
		var stringOverlays []string
		var stringWantOverlays []string
		o, err := exporter.Overlays()
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range o {
			stringOverlays = append(stringOverlays, k.String())
		}

		for _, k := range wantOverlays {
			stringWantOverlays = append(stringWantOverlays, k.String())
		}

		sort.Strings(stringOverlays)
		sort.Strings(stringWantOverlays)
		if reflect.DeepEqual(stringOverlays, stringWantOverlays) {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	o, err := exporter.Overlays()
	if err != nil {
		t.Fatal(err)
	}

	t.Errorf("Overlays got %v, want %v", o, wantOverlays)
}

func expectMultiaddresessEventually(t *testing.T, exporter AddressExporter, wantMultiaddresses []ma.Multiaddr) {
	for i := 0; i < 10; i++ {
		var stringMultiaddresses []string
		m, err := exporter.Multiaddresses()
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range m {
			stringMultiaddresses = append(stringMultiaddresses, v.String())
		}

		var stringWantMultiAddresses []string
		for _, v := range wantMultiaddresses {
			stringWantMultiAddresses = append(stringWantMultiAddresses, v.String())
		}

		sort.Strings(stringMultiaddresses)
		sort.Strings(stringWantMultiAddresses)
		if reflect.DeepEqual(stringMultiaddresses, stringWantMultiAddresses) {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	m, err := exporter.Multiaddresses()
	if err != nil {
		t.Fatal(err)
	}

	t.Errorf("Multiaddresses got %v, want %v", m, wantMultiaddresses)
}

func readAndAssertPeersMsgs(in []byte, expectedLen int) ([]pb.Peers, error) {
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(in),
		func() protobuf.Message {
			return new(pb.Peers)
		},
	)

	if err != nil {
		return nil, err
	}

	if len(messages) != expectedLen {
		return nil, fmt.Errorf("got %v messages, want %v", len(messages), expectedLen)
	}

	var peers []pb.Peers

	for _, m := range messages {
		peers = append(peers, *m.(*pb.Peers))
	}

	return peers, nil
}

func createRandomBytes() []byte {
	randBytes := make([]byte, 32)
	rand.Read(randBytes)
	return randBytes
}
