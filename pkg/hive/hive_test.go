// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ab "github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/hive"
	"github.com/ethersphere/bee/v2/pkg/hive/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	chequebookmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-varint"
)

var nonce = common.HexToHash("0x2").Bytes()

const (
	spinTimeout          = time.Second * 5
	testCoalesceInterval = 100 * time.Millisecond
	testCoalesceWait     = 120 * time.Millisecond
)

func waitForCoalesceFlush(t *testing.T) {
	t.Helper()
	time.Sleep(testCoalesceWait)
	synctest.Wait()
}

func newCoalescingClient(
	t *testing.T,
	recorder p2p.Streamer,
	addressbook ab.GetPutter,
	networkID uint64,
	overlay swarm.Address,
	logger log.Logger,
	opts hive.Options,
) *hive.Service {
	t.Helper()

	opts.GossipCoalesceInterval = testCoalesceInterval
	client := hive.New(recorder, addressbook, networkID, overlay, logger, opts)
	client.SetCoalesceJitterForTest(0)
	testutil.CleanupCloser(t, client)
	return client
}

func assertNoGossipRecords(t *testing.T, recorder *streamtest.Recorder, addressee swarm.Address) {
	t.Helper()

	records, err := recorder.Records(addressee, "hive", "2.0.0", "peers")
	if err == nil {
		if len(records) != 0 {
			t.Fatalf("got %d gossip records, want none before coalesce flush", len(records))
		}
		return
	}
	if !errors.Is(err, streamtest.ErrRecordsNotFound) {
		t.Fatal(err)
	}
}

func TestHandlerRateLimit(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	statestore := mock.NewStateStore()
	addressbook := ab.New(statestore)
	networkID := uint64(1)

	addressbookclean := ab.New(mock.NewStateStore())

	// new recorder
	streamer := streamtest.New()
	// create a hive server that handles the incoming stream
	serverAddress := swarm.RandAddress(t)
	server := hive.New(streamer, addressbookclean, networkID, serverAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	testutil.CleanupCloser(t, server)

	// setup the stream recorder to record stream data
	serverRecorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
		streamtest.WithBaseAddr(serverAddress),
	)

	peers := make([]swarm.Address, hive.LimitBurst+1)
	for i := range peers {
		underlay1, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}

		underlay2, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		pk, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(pk)
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, []ma.Multiaddr{underlay1, underlay2}, overlay, networkID, nonce, 1, common.Address{})
		if err != nil {
			t.Fatal(err)
		}

		err = addressbook.Put(bzzAddr.Overlay, *bzzAddr, true)
		if err != nil {
			t.Fatal(err)
		}
		peers[i] = bzzAddr.Overlay
	}

	// create a hive client that will do broadcast
	clientAddress := swarm.RandAddress(t)
	client := hive.New(serverRecorder, addressbook, networkID, clientAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	err := client.BroadcastPeers(context.Background(), serverAddress, peers...)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, client)

	rec, err := serverRecorder.Records(serverAddress, "hive", "2.0.0", "peers")
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
	bzzAddresses := make([]bzz.Address, 0, 2*hive.MaxBatchSize)
	overlays := make([]swarm.Address, 0, 2*hive.MaxBatchSize)
	wantMsgs := make([]pb.Peers, 0, 2*hive.MaxBatchSize)

	for range 2 {
		wantMsgs = append(wantMsgs, pb.Peers{Peers: []*pb.BzzAddress{}})
	}

	last := 2*hive.MaxBatchSize - 1

	for i := 0; i < 2*hive.MaxBatchSize; i++ {
		var underlays []ma.Multiaddr
		if i == last {
			u, err := ma.NewMultiaddr("/ip4/1.1.1.1/udp/" + strconv.Itoa(i)) // public
			if err != nil {
				t.Fatal(err)
			}
			u2, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(i))
			if err != nil {
				t.Fatal(err)
			}
			underlays = []ma.Multiaddr{u, u2}
		} else {
			n := (i % 3) + 1
			for j := range n {
				port := i + j*10000
				u, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(port))
				if err != nil {
					t.Fatal(err)
				}
				underlays = append(underlays, u)
			}
		}

		pk, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(pk)
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, underlays, overlay, networkID, nonce, 1, common.Address{})
		if err != nil {
			t.Fatal(err)
		}

		bzzAddresses = append(bzzAddresses, *bzzAddr)
		overlays = append(overlays, bzzAddr.Overlay)
		if err := addressbook.Put(bzzAddr.Overlay, *bzzAddr, true); err != nil {
			t.Fatal(err)
		}

		underlayBytes, err := bzz.SerializeUnderlays(bzzAddresses[i].Underlays)
		if err != nil {
			t.Fatal(err)
		}
		wantMsgs[i/hive.MaxBatchSize].Peers = append(wantMsgs[i/hive.MaxBatchSize].Peers, &pb.BzzAddress{
			Overlay:   bzzAddresses[i].Overlay.Bytes(),
			Underlay:  underlayBytes,
			Signature: bzzAddresses[i].Signature,
			Nonce:     nonce,
			Timestamp: bzzAddresses[i].Timestamp,
		})
	}

	testCases := map[string]struct {
		addresee          swarm.Address
		peers             []swarm.Address
		wantMsgs          []pb.Peers
		wantOverlays      []swarm.Address
		wantBzzAddresses  []bzz.Address
		allowPrivateCIDRs bool
		coalesceSingle    bool
	}{
		"OK - single record": {
			addresee:          swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c"),
			peers:             []swarm.Address{overlays[0]},
			wantMsgs:          []pb.Peers{{Peers: wantMsgs[0].Peers[:1]}},
			wantOverlays:      []swarm.Address{overlays[0]},
			wantBzzAddresses:  []bzz.Address{bzzAddresses[0]},
			allowPrivateCIDRs: true,
			coalesceSingle:    true,
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
		"Ok - don't advertise private CIDRs only": {
			addresee:          overlays[len(overlays)-1],
			peers:             overlays[:15],
			wantMsgs:          []pb.Peers{{}},
			wantOverlays:      nil,
			wantBzzAddresses:  nil,
			allowPrivateCIDRs: false,
		},
		"Ok - don't advertise private CIDRs only (but include one public peer)": {
			addresee: overlays[0],
			peers:    overlays[58:],
			wantMsgs: []pb.Peers{{Peers: func() []*pb.BzzAddress {
				ub, err := bzz.SerializeUnderlays(bzzAddresses[len(bzzAddresses)-1].Underlays)
				if err != nil {
					t.Fatal(err)
				}
				return []*pb.BzzAddress{{
					Overlay:   bzzAddresses[len(bzzAddresses)-1].Overlay.Bytes(),
					Underlay:  ub,
					Signature: bzzAddresses[len(bzzAddresses)-1].Signature,
					Nonce:     nonce,
					Timestamp: bzzAddresses[len(bzzAddresses)-1].Timestamp,
				}}
			}()}},
			wantOverlays: []swarm.Address{overlays[len(overlays)-1]},
			wantBzzAddresses: []bzz.Address{
				{
					Underlays:       bzzAddresses[len(bzzAddresses)-1].Underlays,
					Overlay:         bzzAddresses[len(bzzAddresses)-1].Overlay,
					Signature:       bzzAddresses[len(bzzAddresses)-1].Signature,
					Nonce:           bzzAddresses[len(bzzAddresses)-1].Nonce,
					Timestamp:       bzzAddresses[len(bzzAddresses)-1].Timestamp,
					EthereumAddress: bzzAddresses[len(bzzAddresses)-1].EthereumAddress,
				},
			},
			allowPrivateCIDRs: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			run := func(t *testing.T) {
				addressbookclean := ab.New(mock.NewStateStore())

				streamer := streamtest.New()
				// create a hive server that handles the incoming stream
				serverAddress := swarm.RandAddress(t)
				server := hive.New(streamer, addressbookclean, networkID, serverAddress, logger, hive.Options{AllowPrivateCIDRs: true})
				testutil.CleanupCloser(t, server)

				// setup the stream recorder to record stream data
				recorder := streamtest.New(
					streamtest.WithProtocols(server.Protocol()),
				)

				// create a hive client that will do broadcast
				clientAddress := swarm.RandAddress(t)
				opts := hive.Options{AllowPrivateCIDRs: tc.allowPrivateCIDRs}
				var client *hive.Service
				if tc.coalesceSingle {
					client = newCoalescingClient(t, recorder, addressbook, networkID, clientAddress, logger, opts)
				} else {
					client = hive.New(recorder, addressbook, networkID, clientAddress, logger, opts)
					testutil.CleanupCloser(t, client)
				}

				if err := client.BroadcastPeers(context.Background(), tc.addresee, tc.peers...); err != nil {
					t.Fatal(err)
				}

				if tc.coalesceSingle {
					assertNoGossipRecords(t, recorder, tc.addresee)
					waitForCoalesceFlush(t)
				}

				// get a record for this stream
				records, err := recorder.Records(tc.addresee, "hive", "2.0.0", "peers")
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
					comparePeerMsgs(t, messages[0].Peers, tc.wantMsgs[i].Peers)
				}

				expectOverlaysEventually(t, addressbookclean, tc.wantOverlays)
				expectBzzAddresessEventually(t, addressbookclean, tc.wantBzzAddresses)
			}
			if tc.coalesceSingle {
				synctest.Test(t, run)
			} else {
				t.Parallel()
				run(t)
			}
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

func comparePeerMsgs(t *testing.T, got, want []*pb.BzzAddress) {
	t.Helper()

	gotMap := flattenByOverlay(t, got)
	wantMap := flattenByOverlay(t, want)

	for ovlHex, w := range wantMap {
		g, ok := gotMap[ovlHex]
		if !ok {
			t.Fatalf("expected peer %s, but not found", ovlHex)
		}
		if !bytes.Equal(g.Underlay, w.Underlay) {
			t.Fatalf("peer %s: underlays (got=%s want=%s)", ovlHex, g.Underlay, w.Underlay)
		}
		if !bytes.Equal(g.Signature, w.Signature) {
			t.Fatalf("peer %s: expected signatures (got=%s want=%s)",
				ovlHex, shortHex(g.Signature), shortHex(w.Signature))
		}
		if !bytes.Equal(g.Nonce, w.Nonce) {
			t.Fatalf("peer %s: expected nonce (got=%s want=%s)",
				ovlHex, shortHex(g.Nonce), shortHex(w.Nonce))
		}
		if g.Timestamp != w.Timestamp {
			t.Fatalf("peer %s: timestamp (got=%d want=%d)", ovlHex, g.Timestamp, w.Timestamp)
		}
	}
}

func flattenByOverlay(t *testing.T, batches []*pb.BzzAddress) map[string]*pb.BzzAddress {
	t.Helper()

	m := make(map[string]*pb.BzzAddress)
	for _, batch := range batches {
		overlay := hex.EncodeToString(batch.Overlay)
		if _, ok := m[overlay]; ok {
			t.Fatalf("multiple bzz addresses with the same overlay address: %s", overlay)
		}
		m[overlay] = batch
	}
	return m
}

func shortHex(b []byte) string {
	s := hex.EncodeToString(b)
	if len(s) > 32 {
		return s[:32] + fmt.Sprintf("…(%dB)", len(b))
	}
	return s
}

// TestBroadcastPeersSkipsSelf verifies that hive does not broadcast self address
// to other peers, preventing self-connection attempts.
func TestBroadcastPeersSkipsSelf(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	statestore := mock.NewStateStore()
	addressbook := ab.New(statestore)
	networkID := uint64(1)
	addressbookclean := ab.New(mock.NewStateStore())

	// Create addresses
	serverAddress := swarm.RandAddress(t)

	// Create a peer address
	peer1 := swarm.RandAddress(t)
	underlay1, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1234")
	if err != nil {
		t.Fatal(err)
	}
	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	overlay1, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	bzzAddr1, err := bzz.NewAddress(signer, []ma.Multiaddr{underlay1}, overlay1, networkID, nonce, 1, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(bzzAddr1.Overlay, *bzzAddr1, true); err != nil {
		t.Fatal(err)
	}

	// Create self address entry in addressbook
	underlayClient, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/9999")
	if err != nil {
		t.Fatal(err)
	}
	pkClient, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signerClient := crypto.NewDefaultSigner(pkClient)
	clientAddress, err := crypto.NewOverlayAddress(pkClient.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	bzzAddrClient, err := bzz.NewAddress(signerClient, []ma.Multiaddr{underlayClient}, clientAddress, networkID, nonce, 1, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(clientAddress, *bzzAddrClient, true); err != nil {
		t.Fatal(err)
	}

	// Setup server
	streamer := streamtest.New()
	server := hive.New(streamer, addressbookclean, networkID, serverAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	testutil.CleanupCloser(t, server)

	serverRecorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	// Setup client
	client := hive.New(serverRecorder, addressbook, networkID, clientAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	testutil.CleanupCloser(t, client)

	// Try to broadcast: peer1, clientAddress (self), and another peer
	peersIncludingSelf := []swarm.Address{bzzAddr1.Overlay, clientAddress, peer1}

	err = client.BroadcastPeers(context.Background(), serverAddress, peersIncludingSelf...)
	if err != nil {
		t.Fatal(err)
	}

	// Get records
	records, err := serverRecorder.Records(serverAddress, "hive", "2.0.0", "peers")
	if err != nil {
		t.Fatal(err)
	}

	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// Read the messages
	messages, err := readAndAssertPeersMsgs(records[0].In(), 1)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that clientAddress (self) was NOT included in broadcast
	for _, peerMsg := range messages[0].Peers {
		receivedOverlay := swarm.NewAddress(peerMsg.Overlay)
		if receivedOverlay.Equal(clientAddress) {
			t.Fatal("self address should not be broadcast to peers")
		}
	}

	// Verify server addressbook eventually contains only the valid peers, not self
	err = spinlock.Wait(spinTimeout, func() bool {
		overlays, err := addressbookclean.Overlays()
		if err != nil {
			t.Fatal(err)
		}
		// Should only have bzzAddr1, not clientAddress
		for _, o := range overlays {
			if o.Equal(clientAddress) {
				return false // self should not be in addressbook
			}
		}
		return true
	})
	if err != nil {
		t.Fatal("self address found in server addressbook")
	}
}

// TestReceivePeersSkipsSelf verifies that hive does not add self address
// when receiving peer lists from other peers.
func TestReceivePeersSkipsSelf(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	statestore := mock.NewStateStore()
	addressbook := ab.New(statestore)
	networkID := uint64(1)
	addressbookclean := ab.New(mock.NewStateStore())

	// Create client address
	clientAddress := swarm.RandAddress(t)

	// Create a valid peer
	underlay1, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1234")
	if err != nil {
		t.Fatal(err)
	}
	pk1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer1 := crypto.NewDefaultSigner(pk1)
	overlay1, err := crypto.NewOverlayAddress(pk1.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	bzzAddr1, err := bzz.NewAddress(signer1, []ma.Multiaddr{underlay1}, overlay1, networkID, nonce, 1, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(bzzAddr1.Overlay, *bzzAddr1, true); err != nil {
		t.Fatal(err)
	}

	// Create self address entry (serverAddress) that will be sent by client
	underlayServer, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/8888")
	if err != nil {
		t.Fatal(err)
	}
	pkServer, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signerServer := crypto.NewDefaultSigner(pkServer)
	serverAddress, err := crypto.NewOverlayAddress(pkServer.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	bzzAddrServer, err := bzz.NewAddress(signerServer, []ma.Multiaddr{underlayServer}, serverAddress, networkID, nonce, 1, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	// Add server's own address to client's addressbook (so client can send it)
	if err := addressbook.Put(serverAddress, *bzzAddrServer, true); err != nil {
		t.Fatal(err)
	}

	// Setup server that will receive peers including its own address
	streamer := streamtest.New()
	server := hive.New(streamer, addressbookclean, networkID, serverAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	testutil.CleanupCloser(t, server)

	serverRecorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	// Setup client
	client := hive.New(serverRecorder, addressbook, networkID, clientAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	testutil.CleanupCloser(t, client)

	// Client broadcasts: valid peer and server's own address
	peersIncludingSelf := []swarm.Address{bzzAddr1.Overlay, serverAddress}

	err = client.BroadcastPeers(context.Background(), serverAddress, peersIncludingSelf...)
	if err != nil {
		t.Fatal(err)
	}

	// Wait a bit for server to process
	time.Sleep(100 * time.Millisecond)

	// Verify server's addressbook does NOT contain its own address
	overlays, err := addressbookclean.Overlays()
	if err != nil {
		t.Fatal(err)
	}

	for _, o := range overlays {
		if o.Equal(serverAddress) {
			t.Fatal("server should not add its own address to addressbook when received from peer")
		}
	}

	// Verify server does have the valid peer
	_, _, err = addressbookclean.Get(bzzAddr1.Overlay)
	if err != nil {
		t.Fatalf("expected server to have valid peer in addressbook, got error: %v", err)
	}
}

// Avoids multiaddr port collisions across parallel chequebook tests.
var cbPortCounter atomic.Uint32

// signedPeerWithChequebook returns a freshly-signed record for a generated
// identity, with the given chequebook in the signed payload. The returned
// peerIdentity can be used to sign successive records with different
// timestamps or chequebooks for the same overlay.
func signedPeerWithChequebook(t *testing.T, networkID uint64, ts int64, cb common.Address) (*pb.BzzAddress, *peerIdentity, common.Address) {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	n := common.HexToHash("0xab").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, n)
	if err != nil {
		t.Fatal(err)
	}

	port := uint16(20000) + uint16(cbPortCounter.Add(1))
	u, err := ma.NewMultiaddr("/ip4/10.0.0.1/tcp/" + strconv.Itoa(int(port)))
	if err != nil {
		t.Fatal(err)
	}

	id := &peerIdentity{
		signer:   signer,
		overlay:  overlay,
		nonce:    n,
		underlay: u,
	}

	peerEth, err := crypto.NewEthereumAddress(pk.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	return id.protoWithChequebook(t, networkID, ts, cb), id, common.BytesToAddress(peerEth)
}

// protoWithChequebook signs and serialises a record carrying a non-zero
// chequebook in the signed payload. Companion to (peerIdentity).protoAt
// in timestamp_test.go (which signs with an empty chequebook).
func (p *peerIdentity) protoWithChequebook(t *testing.T, networkID uint64, ts int64, cb common.Address) *pb.BzzAddress {
	t.Helper()
	addr, err := bzz.NewAddress(p.signer, []ma.Multiaddr{p.underlay}, p.overlay, networkID, p.nonce, ts, cb)
	if err != nil {
		t.Fatal(err)
	}

	underlayByes, err := bzz.SerializeUnderlays(addr.Underlays)
	if err != nil {
		t.Fatal(err)
	}

	return &pb.BzzAddress{
		Underlay:          underlayByes,
		Overlay:           addr.Overlay.Bytes(),
		Signature:         addr.Signature,
		Nonce:             addr.Nonce,
		Timestamp:         addr.Timestamp,
		ChequebookAddress: addr.ChequebookAddress.Bytes(),
	}
}

func newHiveForTest(t *testing.T, verifier chequebook.Verifier, storer hive.ChequebookStorer) (*hive.Service, ab.Interface) {
	t.Helper()

	store := mock.NewStateStore()
	addressbook := ab.New(store)
	self := swarm.RandAddress(t)
	svc := hive.New(streamtest.New(), addressbook, 1, self, log.Noop, hive.Options{
		AllowPrivateCIDRs:  true,
		ChequebookVerifier: verifier,
		ChequebookStorer:   storer,
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc, addressbook
}

func TestHive_ChequebookVerifierAcceptsValidRecord(t *testing.T) {
	t.Parallel()

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	rec, id, expectedIssuer := signedPeerWithChequebook(t, 1, time.Now().Unix(), cb)

	v := &chequebookmock.Verifier{Behavior: func(got, peerEth common.Address, peerOverlay swarm.Address, _ bool) error {
		if got != cb {
			t.Fatalf("verifier got cb %s, want %s", got.Hex(), cb.Hex())
		}
		if peerEth != expectedIssuer {
			t.Fatalf("verifier got eth %s, want %s", peerEth.Hex(), expectedIssuer.Hex())
		}
		if !peerOverlay.Equal(id.overlay) {
			t.Fatalf("verifier got overlay %s, want %s", peerOverlay, id.overlay)
		}
		return nil
	}}
	storer := &chequebookmock.Storer{}
	svc, addressbook := newHiveForTest(t, v, storer)

	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})

	if v.Calls != 1 {
		t.Fatalf("verifier called %d times, want 1", v.Calls)
	}
	if got, ok := storer.Puts[id.overlay.String()]; !ok || got != cb {
		t.Fatalf("storer not called with (%s, %s); got %v", id.overlay, cb.Hex(), storer.Puts)
	}
	_, verified, err := addressbook.Get(id.overlay)
	if err != nil {
		t.Fatalf("expected peer in addressbook after successful verify, got %v", err)
	}
	if !verified {
		t.Fatal("addressbook entry must be stored with Verified=true after successful verify")
	}
}

func TestHive_ChequebookVerifierRejectsBadIssuer(t *testing.T) {
	t.Parallel()

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	rec, id, _ := signedPeerWithChequebook(t, 1, time.Now().Unix(), cb)

	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, _ bool) error {
		return chequebook.ErrChequebookIssuerMismatch
	}}
	storer := &chequebookmock.Storer{}
	svc, addressbook := newHiveForTest(t, v, storer)

	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})

	if v.Calls != 1 {
		t.Fatalf("verifier called %d times, want 1", v.Calls)
	}
	if len(storer.Puts) != 0 {
		t.Fatalf("storer should not be called on rejection, got %v", storer.Puts)
	}
	if _, _, err := addressbook.Get(id.overlay); !errors.Is(err, ab.ErrNotFound) {
		t.Fatalf("expected peer NOT in addressbook after issuer mismatch, got %v", err)
	}
}

func TestHive_RejectsMissingChequebook(t *testing.T) {
	t.Parallel()

	rec, id, _ := signedPeerWithChequebook(t, 1, time.Now().Unix(), common.Address{})

	v := &chequebookmock.Verifier{}
	svc, addressbook := newHiveForTest(t, v, &chequebookmock.Storer{})

	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})

	if v.Calls != 0 {
		t.Fatalf("verifier must not run when chequebook is missing")
	}
	if _, _, err := addressbook.Get(id.overlay); !errors.Is(err, ab.ErrNotFound) {
		t.Fatalf("verifier-enabled service must reject empty chequebook, got %v", err)
	}
}

func TestHive_VerifierUnconfiguredAcceptsAll(t *testing.T) {
	t.Parallel()

	svc, addressbook := newHiveForTest(t, nil, nil)

	rec, id, _ := signedPeerWithChequebook(t, 1, time.Now().Unix(), common.Address{})

	// Verifier nil → permissive mode.
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})

	_, verified, err := addressbook.Get(id.overlay)
	if err != nil {
		t.Fatalf("unconfigured verifier must preserve pre-PR behaviour, got %v", err)
	}
	if verified {
		t.Fatal("addressbook entry must be stored with Verified=false on the no-verifier fallback path")
	}
}

// TestHive_VerifyFails_ExistingAddressbookPreserved verifies that when
// Verify returns an error, the storer is not called and the existing
// addressbook entry is left untouched.
func TestHive_VerifyFails_ExistingAddressbookPreserved(t *testing.T) {
	t.Parallel()

	v := &chequebookmock.Verifier{Behavior: func(_ common.Address, _ common.Address, _ swarm.Address, _ bool) error {
		return chequebook.ErrChequebookInsufficientBalance
	}}
	storer := &chequebookmock.Storer{}
	svc, addressbook := newHiveForTest(t, v, storer)

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	rec, id, _ := signedPeerWithChequebook(t, 1, time.Now().Unix()-int64(bzz.MinimumUpdateInterval.Seconds())-1, cb)

	bzzAddr, err := bzz.ParseAddress(rec.Underlay, rec.Overlay, rec.Signature, rec.Nonce, rec.Timestamp, 1, rec.ChequebookAddress)
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(id.overlay, *bzzAddr, true); err != nil {
		t.Fatal(err)
	}
	originalTimestamp := bzzAddr.Timestamp

	rec2 := id.protoWithChequebook(t, 1, time.Now().Unix(), cb)

	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec2}})

	if v.Calls != 1 {
		t.Fatalf("Verify called %d times, want 1", v.Calls)
	}
	if len(storer.Puts) != 0 {
		t.Fatalf("storer must not be called on verify failure, got %v", storer.Puts)
	}
	got, _, err := addressbook.Get(id.overlay)
	if err != nil {
		t.Fatalf("existing addressbook entry must be preserved, got %v", err)
	}
	if got.Timestamp != originalTimestamp {
		t.Fatalf("addressbook timestamp mutated: got %d, want %d", got.Timestamp, originalTimestamp)
	}
}

// TestHive_GossipUpdatesChequebook covers the gossip-source path for a peer
// whose chequebook address changes between updates: the same overlay arrives
// twice with a newer timestamp and a different (verifier-accepted) chequebook,
// and the addressbook must reflect the new chequebook on the second pass.
// Symmetric to the registry-level TestRegistry_HandshakeIgnoresMinInterval
// for the gossip ingestion point.
func TestHive_GossipUpdatesChequebook(t *testing.T) {
	t.Parallel()

	cbX := common.HexToAddress("0x1111111111111111111111111111111111111111")
	cbY := common.HexToAddress("0x2222222222222222222222222222222222222222")

	var verified []common.Address
	v := &chequebookmock.Verifier{Behavior: func(got, _ common.Address, _ swarm.Address, _ bool) error {
		verified = append(verified, got)
		return nil
	}}
	svc, addressbook := newHiveForTest(t, v, &chequebookmock.Storer{})

	// Backdate ts1 past MinimumUpdateInterval so ts2 := now passes both the
	// min-interval gate (relative to existing) and the skew gate (relative to now).
	ts1 := time.Now().Unix() - int64(bzz.MinimumUpdateInterval.Seconds()) - 1
	rec1, id, _ := signedPeerWithChequebook(t, 1, ts1, cbX)
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec1}})

	got, _, err := addressbook.Get(id.overlay)
	if err != nil {
		t.Fatalf("after first gossip: %v", err)
	}
	if got.ChequebookAddress != cbX {
		t.Fatalf("first gossip cb: got %s, want %s", got.ChequebookAddress.Hex(), cbX.Hex())
	}

	// Same identity re-signs a fresh record with a different chequebook.
	ts2 := time.Now().Unix()
	rec2 := id.protoWithChequebook(t, 1, ts2, cbY)
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec2}})

	got, _, err = addressbook.Get(id.overlay)
	if err != nil {
		t.Fatalf("after second gossip: %v", err)
	}
	if got.ChequebookAddress != cbY {
		t.Fatalf("second gossip cb: got %s, want %s", got.ChequebookAddress.Hex(), cbY.Hex())
	}
	if got.Timestamp != ts2 {
		t.Fatalf("second gossip ts: got %d, want %d", got.Timestamp, ts2)
	}

	if len(verified) != 2 {
		t.Fatalf("verifier called %d times, want 2 (one per gossip)", len(verified))
	}
	if verified[0] != cbX || verified[1] != cbY {
		t.Fatalf("verifier saw chequebooks %v, want [%s, %s]", verified, cbX.Hex(), cbY.Hex())
	}
}

// TestHive_VerifierPastVerifiedFalseForNewPeer asserts that for a peer not yet
// in the addressbook, the verifier is called with pastVerified=false.
func TestHive_VerifierPastVerifiedFalseForNewPeer(t *testing.T) {
	t.Parallel()

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	rec, _, _ := signedPeerWithChequebook(t, 1, time.Now().Unix(), cb)

	var got []bool
	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, pastVerified bool) error {
		got = append(got, pastVerified)
		return nil
	}}
	svc, _ := newHiveForTest(t, v, &chequebookmock.Storer{})

	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec}})

	if len(got) != 1 {
		t.Fatalf("verifier calls: got %d, want 1", len(got))
	}
	if got[0] {
		t.Fatalf("pastVerified: got true, want false for new peer")
	}
}

// TestHive_VerifierPastVerifiedTrueForKnownPeer asserts that for a peer whose
// addressbook entry was previously stored with Verified=true, the verifier is
// called with pastVerified=true on the next gossip update.
func TestHive_VerifierPastVerifiedTrueForKnownPeer(t *testing.T) {
	t.Parallel()

	var got []bool
	v := &chequebookmock.Verifier{Behavior: func(_, _ common.Address, _ swarm.Address, pastVerified bool) error {
		got = append(got, pastVerified)
		return nil
	}}
	svc, addressbook := newHiveForTest(t, v, &chequebookmock.Storer{})

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	// Backdate so the second record is past MinimumUpdateInterval.
	ts1 := time.Now().Unix() - int64(bzz.MinimumUpdateInterval.Seconds()) - 1
	rec1, id, _ := signedPeerWithChequebook(t, 1, ts1, cb)

	bzzAddr, err := bzz.ParseAddress(rec1.Underlay, rec1.Overlay, rec1.Signature, rec1.Nonce, rec1.Timestamp, 1, rec1.ChequebookAddress)
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(id.overlay, *bzzAddr, true); err != nil {
		t.Fatal(err)
	}

	rec2 := id.protoWithChequebook(t, 1, time.Now().Unix(), cb)
	svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{rec2}})

	if len(got) != 1 {
		t.Fatalf("verifier calls: got %d, want 1", len(got))
	}
	if !got[0] {
		t.Fatalf("pastVerified: got false, want true for previously-verified peer")
	}
}

// peerIdentity bundles a deterministic node identity so tests can produce
// multiple re-signed BzzAddress records for the same overlay.
type peerIdentity struct {
	signer   crypto.Signer
	overlay  swarm.Address
	nonce    []byte
	underlay ma.Multiaddr
}

// newPeerIdentity returns a deterministic node identity so tests can produce
// multiple re-signed BzzAddress records for the same overlay.
func newPeerIdentity(t *testing.T, networkID uint64, underlayStr string) *peerIdentity {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 32)
	nonce[31] = 0xAA
	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	u, err := ma.NewMultiaddr(underlayStr)
	if err != nil {
		t.Fatal(err)
	}
	return &peerIdentity{
		signer:   crypto.NewDefaultSigner(pk),
		overlay:  overlay,
		nonce:    nonce,
		underlay: u,
	}
}

func (p *peerIdentity) signedAddress(t *testing.T, networkID uint64, ts int64) *bzz.Address {
	t.Helper()
	addr, err := bzz.NewAddress(p.signer, []ma.Multiaddr{p.underlay}, p.overlay, networkID, p.nonce, ts, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func (p *peerIdentity) protoAt(t *testing.T, networkID uint64, ts int64) *pb.BzzAddress {
	t.Helper()
	addr := p.signedAddress(t, networkID, ts)

	underlayByes, err := bzz.SerializeUnderlays(addr.Underlays)
	if err != nil {
		t.Fatal(err)
	}

	return &pb.BzzAddress{
		Overlay:   addr.Overlay.Bytes(),
		Underlay:  underlayByes,
		Signature: addr.Signature,
		Nonce:     addr.Nonce,
		Timestamp: addr.Timestamp,
	}
}

// TestHiveGossipTimestampRules drives checkAndAddPeers directly under a fake
// clock (testing/synctest) to exercise the timestamp validation paths for the
// gossip ingestion point.
func TestHiveGossipTimestampRules(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		networkID := uint64(1)
		addressbook := ab.New(mock.NewStateStore())
		selfOverlay := swarm.RandAddress(t)
		svc := hive.New(streamtest.New(), addressbook, networkID, selfOverlay, log.Noop, hive.Options{AllowPrivateCIDRs: true})
		t.Cleanup(func() { _ = svc.Close() })

		peer := newPeerIdentity(t, networkID, "/ip4/10.0.0.1/tcp/7070")

		mustGet := func() *bzz.Address {
			got, _, err := addressbook.Get(peer.overlay)
			if err != nil {
				t.Fatalf("addressbook.Get: %v", err)
			}
			return got
		}

		// first gossip is stored
		ts1 := time.Now().Unix()
		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{peer.protoAt(t, networkID, ts1)}})
		if got := mustGet(); got.Timestamp != ts1 {
			t.Fatalf("initial gossip: got ts=%d want ts=%d", got.Timestamp, ts1)
		}

		// gossip within MinimumUpdateInterval is rejected
		time.Sleep(bzz.MinimumUpdateInterval / 2)
		ts2 := time.Now().Unix()
		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{peer.protoAt(t, networkID, ts2)}})
		if got := mustGet(); got.Timestamp != ts1 {
			t.Fatalf("gossip within interval should be dropped: got ts=%d want ts=%d", got.Timestamp, ts1)
		}

		// gossip past MinimumUpdateInterval is accepted
		time.Sleep(bzz.MinimumUpdateInterval + time.Second)
		ts3 := time.Now().Unix()
		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{peer.protoAt(t, networkID, ts3)}})
		if got := mustGet(); got.Timestamp != ts3 {
			t.Fatalf("gossip past interval should update: got ts=%d want ts=%d", got.Timestamp, ts3)
		}

		// stale gossip (older than existing) is rejected
		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{peer.protoAt(t, networkID, ts3-1)}})
		if got := mustGet(); got.Timestamp != ts3 {
			t.Fatalf("stale gossip should be dropped: got ts=%d want ts=%d", got.Timestamp, ts3)
		}

		// future gossip beyond MaxClockSkew is rejected
		future := time.Now().Unix() + int64(bzz.MaxClockSkew.Seconds()) + 10
		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{peer.protoAt(t, networkID, future)}})
		if got := mustGet(); got.Timestamp != ts3 {
			t.Fatalf("future gossip beyond skew should be dropped: got ts=%d want ts=%d", got.Timestamp, ts3)
		}
	})
}

// TestHiveGossipLegacyZeroUpgrade verifies that an addressbook entry with
// Timestamp=0 (pre-upgrade legacy record) is overwritten by a fresh gossip
// for the same overlay carrying a positive timestamp.
func TestHiveGossipLegacyZeroUpgrade(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		networkID := uint64(1)
		addressbook := ab.New(mock.NewStateStore())
		selfOverlay := swarm.RandAddress(t)
		svc := hive.New(streamtest.New(), addressbook, networkID, selfOverlay, log.Noop, hive.Options{AllowPrivateCIDRs: true})
		t.Cleanup(func() { _ = svc.Close() })

		peer := newPeerIdentity(t, networkID, "/ip4/10.0.0.2/tcp/7071")

		// Seed a legacy record with Timestamp=0 directly via the addressbook.
		// NewAddress refuses ts<=0, so sign with ts=1 and mutate to 0 to
		// mimic a record produced by a pre-upgrade binary.
		legacy := peer.signedAddress(t, networkID, 1)
		legacy.Timestamp = 0
		if err := addressbook.Put(peer.overlay, *legacy, true); err != nil {
			t.Fatal(err)
		}

		// Fresh gossip arrives for the same identity with a positive timestamp.
		ts := time.Now().Unix()
		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{peer.protoAt(t, networkID, ts)}})

		got, _, err := addressbook.Get(peer.overlay)
		if err != nil {
			t.Fatalf("addressbook.Get: %v", err)
		}
		if got.Timestamp != ts {
			t.Fatalf("legacy upgrade: got ts=%d want ts=%d", got.Timestamp, ts)
		}
	})
}

// TestBroadcastSkipsLegacyZeroRecord verifies that the gossip sender drops
// addressbook entries whose Timestamp is 0 (pre-upgrade records) so peers
// running the new protocol don't reject them as invalid.
func TestBroadcastSkipsLegacyZeroRecord(t *testing.T) {
	t.Parallel()

	networkID := uint64(1)
	logger := log.Noop
	clientStore := mock.NewStateStore()
	clientBook := ab.New(clientStore)

	// Legacy peer: signed with ts=1, mutated to Timestamp=0 to mimic an
	// addressbook entry carried over from a pre-upgrade binary.
	legacyPeer := newPeerIdentity(t, networkID, "/ip4/10.0.0.100/tcp/7070")
	legacyAddr := legacyPeer.signedAddress(t, networkID, 1)
	legacyAddr.Timestamp = 0
	if err := clientBook.Put(legacyPeer.overlay, *legacyAddr, true); err != nil {
		t.Fatal(err)
	}

	// Modern peer with a positive timestamp — must be included.
	modernPeer := newPeerIdentity(t, networkID, "/ip4/10.0.0.101/tcp/7071")
	modernAddr := modernPeer.signedAddress(t, networkID, time.Now().Unix())
	if err := clientBook.Put(modernPeer.overlay, *modernAddr, true); err != nil {
		t.Fatal(err)
	}

	serverBook := ab.New(mock.NewStateStore())
	serverAddress := swarm.RandAddress(t)
	serverStreamer := streamtest.New()
	server := hive.New(serverStreamer, serverBook, networkID, serverAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	t.Cleanup(func() { _ = server.Close() })

	recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))
	clientAddress := swarm.RandAddress(t)
	client := hive.New(recorder, clientBook, networkID, clientAddress, logger, hive.Options{AllowPrivateCIDRs: true})
	t.Cleanup(func() { _ = client.Close() })

	addressee := swarm.RandAddress(t)
	if err := client.BroadcastPeers(context.Background(), addressee, legacyPeer.overlay, modernPeer.overlay); err != nil {
		t.Fatalf("BroadcastPeers: %v", err)
	}

	records, err := recorder.Records(addressee, "hive", "2.0.0", "peers")
	if err != nil {
		t.Fatalf("recorder.Records: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected at least one gossip record")
	}

	messages, err := readAndAssertPeersMsgs(records[0].In(), 1)
	if err != nil {
		t.Fatalf("readAndAssertPeersMsgs: %v", err)
	}

	sent := messages[0].Peers
	if len(sent) != 1 {
		t.Fatalf("expected 1 peer sent (legacy dropped), got %d", len(sent))
	}
	if !swarm.NewAddress(sent[0].Overlay).Equal(modernPeer.overlay) {
		t.Fatalf("expected modern overlay %s, got %s", modernPeer.overlay, swarm.NewAddress(sent[0].Overlay))
	}
}

// TestHiveGossipUnderlayCaps drives checkAndAddPeers with crafted peer entries
// that violate the underlay byte-size and count caps, and verifies the entries
// are dropped before reaching the addressbook. The byte-size gate runs in
// DeserializeUnderlays before any signature work, so the Signature field on the
// crafted records is intentionally bogus.
//
// Canonical caps live in pkg/bzz/underlay.go (maxUnderlayBytes=2048,
// maxUnderlaysPerPeer=20) and are not exported to other packages. The values
// below are chosen well above those caps so they remain valid even if the
// limits are widened.
func TestHiveGossipUnderlayCaps(t *testing.T) {
	t.Parallel()

	const (
		oversizedUnderlayBytes = 8192 // > maxUnderlayBytes (2048)
		oversizedUnderlayCount = 30   // > maxUnderlaysPerPeer (20)
		bzzUnderlayListPrefix  = 0x99 // underlayListPrefix in pkg/bzz/underlay.go
	)
	networkID := uint64(1)
	ts := time.Now().Unix()

	t.Run("oversized underlay payload", func(t *testing.T) {
		t.Parallel()

		addressbook := ab.New(mock.NewStateStore())
		svc := hive.New(streamtest.New(), addressbook, networkID, swarm.RandAddress(t), log.Noop, hive.Options{AllowPrivateCIDRs: true})
		t.Cleanup(func() { _ = svc.Close() })

		peer := newPeerIdentity(t, networkID, "/ip4/10.0.0.1/tcp/7070")
		bad := &pb.BzzAddress{
			Overlay:   peer.overlay.Bytes(),
			Underlay:  make([]byte, oversizedUnderlayBytes),
			Signature: []byte{0},
			Nonce:     peer.nonce,
			Timestamp: ts,
		}

		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{bad}})

		if _, _, err := addressbook.Get(peer.overlay); !errors.Is(err, ab.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after oversized underlay rejection, got err=%v", err)
		}
	})

	t.Run("too many underlays", func(t *testing.T) {
		t.Parallel()

		addressbook := ab.New(mock.NewStateStore())
		svc := hive.New(streamtest.New(), addressbook, networkID, swarm.RandAddress(t), log.Noop, hive.Options{AllowPrivateCIDRs: true})
		t.Cleanup(func() { _ = svc.Close() })

		peer := newPeerIdentity(t, networkID, "/ip4/10.0.0.2/tcp/7070")

		// Manually serialize oversizedUnderlayCount small multiaddrs using the
		// on-wire list format. SerializeUnderlays would refuse to emit this,
		// so the bytes are built directly to exercise the receive-side cap.
		entry, err := ma.NewMultiaddr("/ip4/1.2.3.4/tcp/80")
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		buf.WriteByte(bzzUnderlayListPrefix)
		entryBytes := entry.Bytes()
		for range oversizedUnderlayCount {
			buf.Write(varint.ToUvarint(uint64(len(entryBytes))))
			buf.Write(entryBytes)
		}

		bad := &pb.BzzAddress{
			Overlay:   peer.overlay.Bytes(),
			Underlay:  buf.Bytes(),
			Signature: []byte{0},
			Nonce:     peer.nonce,
			Timestamp: ts,
		}

		svc.CheckAndAddPeers(pb.Peers{Peers: []*pb.BzzAddress{bad}})

		if _, _, err := addressbook.Get(peer.overlay); !errors.Is(err, ab.ErrNotFound) {
			t.Fatalf("expected ErrNotFound after count-cap rejection, got err=%v", err)
		}
	})
}

func TestBroadcastPeersCoalesce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logger := log.Noop
		statestore := mock.NewStateStore()
		addressbook := ab.New(statestore)
		networkID := uint64(1)

		overlays := make([]swarm.Address, 3)
		for i := range overlays {
			underlay, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(2000+i))
			if err != nil {
				t.Fatal(err)
			}
			pk, err := crypto.GenerateSecp256k1Key()
			if err != nil {
				t.Fatal(err)
			}
			signer := crypto.NewDefaultSigner(pk)
			overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
			if err != nil {
				t.Fatal(err)
			}
			bzzAddr, err := bzz.NewAddress(signer, []ma.Multiaddr{underlay}, overlay, networkID, nonce, 1, common.Address{})
			if err != nil {
				t.Fatal(err)
			}
			if err := addressbook.Put(bzzAddr.Overlay, *bzzAddr, true); err != nil {
				t.Fatal(err)
			}
			overlays[i] = bzzAddr.Overlay
		}

		streamer := streamtest.New()
		serverAddress := swarm.RandAddress(t)
		server := hive.New(streamer, ab.New(mock.NewStateStore()), networkID, serverAddress, logger, hive.Options{AllowPrivateCIDRs: true})
		testutil.CleanupCloser(t, server)

		recorder := streamtest.New(streamtest.WithProtocols(server.Protocol()))
		clientAddress := swarm.RandAddress(t)
		client := newCoalescingClient(t, recorder, addressbook, networkID, clientAddress, logger, hive.Options{
			AllowPrivateCIDRs: true,
		})

		ctx := context.Background()
		for _, overlay := range overlays {
			if err := client.BroadcastPeers(ctx, serverAddress, overlay); err != nil {
				t.Fatal(err)
			}
		}

		assertNoGossipRecords(t, recorder, serverAddress)
		waitForCoalesceFlush(t)

		records, err := recorder.Records(serverAddress, "hive", "2.0.0", "peers")
		if err != nil {
			t.Fatal(err)
		}
		if got, want := len(records), 1; got != want {
			t.Fatalf("after flush got %d gossip messages, want %d", got, want)
		}

		messages, err := readAndAssertPeersMsgs(records[0].In(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := len(messages[0].Peers), len(overlays); got != want {
			t.Fatalf("coalesced peer count: got %d, want %d", got, want)
		}

		// Batched gossip is sent immediately without coalescing (use fresh peers).
		batchedOverlays := make([]swarm.Address, 2)
		for i := range batchedOverlays {
			underlay, err := ma.NewMultiaddr("/ip4/127.0.0.1/udp/" + strconv.Itoa(3000+i))
			if err != nil {
				t.Fatal(err)
			}
			pk, err := crypto.GenerateSecp256k1Key()
			if err != nil {
				t.Fatal(err)
			}
			signer := crypto.NewDefaultSigner(pk)
			overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
			if err != nil {
				t.Fatal(err)
			}
			bzzAddr, err := bzz.NewAddress(signer, []ma.Multiaddr{underlay}, overlay, networkID, nonce, 1, common.Address{})
			if err != nil {
				t.Fatal(err)
			}
			if err := addressbook.Put(bzzAddr.Overlay, *bzzAddr, true); err != nil {
				t.Fatal(err)
			}
			batchedOverlays[i] = bzzAddr.Overlay
		}

		if err := client.BroadcastPeers(ctx, serverAddress, batchedOverlays...); err != nil {
			t.Fatal(err)
		}
		records, err = recorder.Records(serverAddress, "hive", "2.0.0", "peers")
		if err != nil {
			t.Fatal(err)
		}
		if got, want := len(records), 2; got != want {
			t.Fatalf("after batched broadcast got %d gossip messages, want %d", got, want)
		}
	})
}
