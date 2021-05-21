// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"runtime/debug"
	"strconv"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	ab "github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/hive"
	"github.com/ethersphere/bee/pkg/hive/pb"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

func TestHandlerRateLimit(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)
	statestore := mock.NewStateStore()
	addressbook := ab.New(statestore)
	networkID := uint64(1)

	addressbookclean := ab.New(mock.NewStateStore())

	// create a hive server that handles the incoming stream
	server := hive.New(nil, addressbookclean, networkID, logger)

	serverAddress := test.RandomAddress()

	// setup the stream recorder to record stream data
	serverRecorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithBaseAddr(serverAddress),
	)

	peers := make([]swarm.Address, hive.LimitBurst+1)
	for i := range peers {

		underlay, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		pk, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(pk)
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, underlay, overlay, networkID)
		if err != nil {
			t.Fatal(err)
		}

		err = addressbook.Put(bzzAddr.Overlay, *bzzAddr)
		if err != nil {
			t.Fatal(err)
		}
		peers[i] = bzzAddr.Overlay
	}

	// create a hive client that will do broadcast
	client := hive.New(serverRecorder, addressbook, networkID, logger)
	err := client.BroadcastPeers(context.Background(), serverAddress, peers...)
	if err != nil {
		t.Fatal(err)
	}

	// // get a record for this stream
	rec, err := serverRecorder.Records(serverAddress, "hive", "1.0.0", "peers")
	if err != nil {
		t.Fatal(err)
	}

	lastRec := rec[len(rec)-1]
	if !errors.Is(lastRec.Err(), hive.ErrRateLimitExceeded) {
		t.Fatal(err)
	}
}

func TestBroadcastPeers(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	logger := logging.New(ioutil.Discard, 0)
	statestore := mock.NewStateStore()
	addressbook := ab.New(statestore)
	networkID := uint64(1)

	// populate all expected and needed random resources for 2 full batches
	// tests cases that uses fewer resources can use sub-slices of this data
	var bzzAddresses []bzz.Address
	var overlays []swarm.Address
	var wantMsgs []pb.Peers

	for i := 0; i < 2; i++ {
		wantMsgs = append(wantMsgs, pb.Peers{Peers: []*pb.BzzAddress{}})
	}

	for i := 0; i < 2*hive.MaxBatchSize; i++ {
		underlay, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		pk, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(pk)
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, underlay, overlay, networkID)
		if err != nil {
			t.Fatal(err)
		}

		bzzAddresses = append(bzzAddresses, *bzzAddr)
		overlays = append(overlays, bzzAddr.Overlay)
		err = addressbook.Put(bzzAddr.Overlay, *bzzAddr)
		if err != nil {
			t.Fatal(err)
		}

		wantMsgs[i/hive.MaxBatchSize].Peers = append(wantMsgs[i/hive.MaxBatchSize].Peers, &pb.BzzAddress{
			Overlay:   bzzAddresses[i].Overlay.Bytes(),
			Underlay:  bzzAddresses[i].Underlay.Bytes(),
			Signature: bzzAddresses[i].Signature,
		})
	}

	testCases := map[string]struct {
		addresee         swarm.Address
		peers            []swarm.Address
		wantMsgs         []pb.Peers
		wantOverlays     []swarm.Address
		wantBzzAddresses []bzz.Address
	}{
		"OK - single record": {
			addresee:         swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:            []swarm.Address{overlays[0]},
			wantMsgs:         []pb.Peers{{Peers: wantMsgs[0].Peers[:1]}},
			wantOverlays:     []swarm.Address{overlays[0]},
			wantBzzAddresses: []bzz.Address{bzzAddresses[0]},
		},
		"OK - single batch - multiple records": {
			addresee:         swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:            overlays[:15],
			wantMsgs:         []pb.Peers{{Peers: wantMsgs[0].Peers[:15]}},
			wantOverlays:     overlays[:15],
			wantBzzAddresses: bzzAddresses[:15],
		},
		"OK - single batch - max number of records": {
			addresee:         swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:            overlays[:hive.MaxBatchSize],
			wantMsgs:         []pb.Peers{{Peers: wantMsgs[0].Peers[:hive.MaxBatchSize]}},
			wantOverlays:     overlays[:hive.MaxBatchSize],
			wantBzzAddresses: bzzAddresses[:hive.MaxBatchSize],
		},
		"OK - multiple batches": {
			addresee:         swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:            overlays[:hive.MaxBatchSize+10],
			wantMsgs:         []pb.Peers{{Peers: wantMsgs[0].Peers}, {Peers: wantMsgs[1].Peers[:10]}},
			wantOverlays:     overlays[:hive.MaxBatchSize+10],
			wantBzzAddresses: bzzAddresses[:hive.MaxBatchSize+10],
		},
		"OK - multiple batches - max number of records": {
			addresee:         swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:            overlays[:2*hive.MaxBatchSize],
			wantMsgs:         []pb.Peers{{Peers: wantMsgs[0].Peers}, {Peers: wantMsgs[1].Peers}},
			wantOverlays:     overlays[:2*hive.MaxBatchSize],
			wantBzzAddresses: bzzAddresses[:2*hive.MaxBatchSize],
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			addressbookclean := ab.New(mock.NewStateStore())

			// create a hive server that handles the incoming stream
			server := hive.New(nil, addressbookclean, networkID, logger)

			// setup the stream recorder to record stream data
			recorder := streamtest.New(
				streamtest.WithProtocols(server.Protocol()),
			)

			// create a hive client that will do broadcast
			client := hive.New(recorder, addressbook, networkID, logger)
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

			expectOverlaysEventually(t, addressbookclean, tc.wantOverlays)
			expectBzzAddresessEventually(t, addressbookclean, tc.wantBzzAddresses)
		})
	}
}

func expectOverlaysEventually(t *testing.T, exporter ab.Interface, wantOverlays []swarm.Address) {
	var (
		overlays []swarm.Address
		err      error
		isIn     = func(a swarm.Address, addrs []swarm.Address) bool {
			for _, v := range addrs {
				if a.Equal(v) {
					return true
				}
			}
			return false
		}
	)

	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		overlays, err = exporter.Overlays()
		if err != nil {
			t.Fatal(err)
		}

		if len(overlays) == len(wantOverlays) {
			break
		}
	}
	if len(overlays) != len(wantOverlays) {
		debug.PrintStack()
		t.Fatal("timed out waiting for overlays")
	}

	for _, v := range wantOverlays {
		if !isIn(v, overlays) {
			t.Errorf("overlay %s expected but not found", v.String())
		}
	}

	if t.Failed() {
		t.Errorf("overlays got %v, want %v", overlays, wantOverlays)
	}
}

func expectBzzAddresessEventually(t *testing.T, exporter ab.Interface, wantBzzAddresses []bzz.Address) {
	var (
		addresses []bzz.Address
		err       error

		isIn = func(a bzz.Address, addrs []bzz.Address) bool {
			for _, v := range addrs {
				if a.Equal(&v) {
					return true
				}
			}
			return false
		}
	)

	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		addresses, err = exporter.Addresses()
		if err != nil {
			t.Fatal(err)
		}

		if len(addresses) == len(wantBzzAddresses) {
			break
		}
	}
	if len(addresses) != len(wantBzzAddresses) {
		debug.PrintStack()
		t.Fatal("timed out waiting for bzz addresses")
	}

	for _, v := range wantBzzAddresses {
		if !isIn(v, addresses) {
			t.Errorf("address %s expected but not found", v.Overlay.String())
		}
	}

	if t.Failed() {
		t.Errorf("bzz addresses got %v, want %v", addresses, wantBzzAddresses)
	}
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
