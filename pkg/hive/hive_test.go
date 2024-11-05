// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ma "github.com/multiformats/go-multiaddr"

	ab "github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/hive"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

var (
	nonce = common.HexToHash("0x2").Bytes()
	block = common.HexToHash("0x1").Bytes()
)

const spinTimeout = time.Second * 5

func TestHandlerRateLimit(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	statestore := mock.NewStateStore()
	addressbook := ab.New(statestore)
	networkID := uint64(1)

	addressbookclean := ab.New(mock.NewStateStore())

	// new recorder for handling Ping
	streamer := streamtest.New()
	// create a hive server that handles the incoming stream
	server := hive.New(streamer, addressbookclean, networkID, false, true, logger)
	testutil.CleanupCloser(t, server)

	serverAddress := swarm.RandAddress(t)

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
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, block)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, underlay, overlay, networkID, nonce)
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
	client := hive.New(serverRecorder, addressbook, networkID, false, true, logger)
	err := client.BroadcastPeers(context.Background(), serverAddress, peers...)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, client)

	rec, err := serverRecorder.Records(serverAddress, "hive", "1.1.0", "peers")
	if err != nil {
		t.Fatal(err)
	}

	lastRec := rec[len(rec)-1]

	if lastRec.Err() != nil {
		t.Fatal("want nil error")
	}
}

func TestBroadcastPeers(t *testing.T) {
	t.Parallel()

	logger := log.Noop
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
		base := "/ip4/127.0.0.1/udp/"
		if i == 2*hive.MaxBatchSize-1 {
			base = "/ip4/1.1.1.1/udp/" // The last underlay has public address.
		}
		underlay, err := ma.NewMultiaddr(base + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		pk, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(pk)
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, block)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, underlay, overlay, networkID, nonce)
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
			Nonce:     nonce,
		})
	}

	testCases := map[string]struct {
		addresee          swarm.Address
		peers             []swarm.Address
		wantMsgs          []pb.Peers
		wantOverlays      []swarm.Address
		wantBzzAddresses  []bzz.Address
		allowPrivateCIDRs bool
		pingErr           func(addr ma.Multiaddr) (time.Duration, error)
	}{
		"OK - single record": {
			addresee:          swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:             []swarm.Address{overlays[0]},
			wantMsgs:          []pb.Peers{{Peers: wantMsgs[0].Peers[:1]}},
			wantOverlays:      []swarm.Address{overlays[0]},
			wantBzzAddresses:  []bzz.Address{bzzAddresses[0]},
			allowPrivateCIDRs: true,
		},
		"OK - single batch - multiple records": {
			addresee:          swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:             overlays[:15],
			wantMsgs:          []pb.Peers{{Peers: wantMsgs[0].Peers[:15]}},
			wantOverlays:      overlays[:15],
			wantBzzAddresses:  bzzAddresses[:15],
			allowPrivateCIDRs: true,
		},
		"OK - single batch - max number of records": {
			addresee:          swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:             overlays[:hive.MaxBatchSize],
			wantMsgs:          []pb.Peers{{Peers: wantMsgs[0].Peers[:hive.MaxBatchSize]}},
			wantOverlays:      overlays[:hive.MaxBatchSize],
			wantBzzAddresses:  bzzAddresses[:hive.MaxBatchSize],
			allowPrivateCIDRs: true,
		},
		"OK - multiple batches": {
			addresee:          swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:             overlays[:hive.MaxBatchSize+10],
			wantMsgs:          []pb.Peers{{Peers: wantMsgs[0].Peers}, {Peers: wantMsgs[1].Peers[:10]}},
			wantOverlays:      overlays[:hive.MaxBatchSize+10],
			wantBzzAddresses:  bzzAddresses[:hive.MaxBatchSize+10],
			allowPrivateCIDRs: true,
		},
		"OK - multiple batches - max number of records": {
			addresee:          swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:             overlays[:2*hive.MaxBatchSize],
			wantMsgs:          []pb.Peers{{Peers: wantMsgs[0].Peers}, {Peers: wantMsgs[1].Peers}},
			wantOverlays:      overlays[:2*hive.MaxBatchSize],
			wantBzzAddresses:  bzzAddresses[:2*hive.MaxBatchSize],
			allowPrivateCIDRs: true,
		},
		"OK - single batch - skip ping failures": {
			addresee:          swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:             overlays[:15],
			wantMsgs:          []pb.Peers{{Peers: wantMsgs[0].Peers[:15]}},
			wantOverlays:      overlays[:10],
			wantBzzAddresses:  bzzAddresses[:10],
			allowPrivateCIDRs: true,
			pingErr: func(addr ma.Multiaddr) (rtt time.Duration, err error) {
				for _, v := range bzzAddresses[10:15] {
					if v.Underlay.Equal(addr) {
						return rtt, errors.New("ping failure")
					}
				}
				return rtt, nil
			},
		},
		"Ok - don't advertise private CIDRs": {
			addresee:          overlays[len(overlays)-1],
			peers:             overlays[:15],
			wantMsgs:          []pb.Peers{{}},
			wantOverlays:      nil,
			wantBzzAddresses:  nil,
			allowPrivateCIDRs: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			addressbookclean := ab.New(mock.NewStateStore())

			// new recorder for handling Ping
			var streamer *streamtest.Recorder
			if tc.pingErr != nil {
				streamer = streamtest.New(streamtest.WithPingErr(tc.pingErr))
			} else {
				streamer = streamtest.New()
			}
			// create a hive server that handles the incoming stream
			server := hive.New(streamer, addressbookclean, networkID, false, true, logger)
			testutil.CleanupCloser(t, server)

			// setup the stream recorder to record stream data
			recorder := streamtest.New(
				streamtest.WithProtocols(server.Protocol()),
			)

			// create a hive client that will do broadcast
			client := hive.New(recorder, addressbook, networkID, false, tc.allowPrivateCIDRs, logger)
			if err := client.BroadcastPeers(context.Background(), tc.addresee, tc.peers...); err != nil {
				t.Fatal(err)
			}
			testutil.CleanupCloser(t, client)

			// get a record for this stream
			records, err := recorder.Records(tc.addresee, "hive", "1.1.0", "peers")
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
	t.Helper()

	var (
		overlays []swarm.Address
		err      error
	)

	err = spinlock.Wait(spinTimeout, func() bool {
		overlays, err = exporter.Overlays()
		if err != nil {
			t.Fatal(err)
		}

		return len(overlays) == len(wantOverlays)
	})
	if err != nil {
		t.Fatal("timed out waiting for overlays")
	}

	for _, v := range wantOverlays {
		if !swarm.ContainsAddress(overlays, v) {
			t.Errorf("overlay %s expected but not found", v.String())
		}
	}

	if t.Failed() {
		t.Errorf("overlays got %v, want %v", overlays, wantOverlays)
	}
}

func expectBzzAddresessEventually(t *testing.T, exporter ab.Interface, wantBzzAddresses []bzz.Address) {
	t.Helper()

	var (
		addresses []bzz.Address
		err       error
	)

	err = spinlock.Wait(spinTimeout, func() bool {
		addresses, err = exporter.Addresses()
		if err != nil {
			t.Fatal(err)
		}

		return len(addresses) == len(wantBzzAddresses)
	})
	if err != nil {
		t.Fatal("timed out waiting for bzz addresses")
	}

	for _, v := range wantBzzAddresses {
		if !bzz.ContainsAddress(addresses, &v) {
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

	peers := make([]pb.Peers, len(messages))
	for i := range messages {
		peers[i] = *messages[i].(*pb.Peers)
	}

	return peers, nil
}
