// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive_test

import (
	"bytes"
	"context"
	"encoding/hex"
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

	// new recorder
	streamer := streamtest.New()
	// create a hive server that handles the incoming stream
	serverAddress := swarm.RandAddress(t)
	server := hive.New(streamer, addressbookclean, networkID, false, true, serverAddress, logger)
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
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, block)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, []ma.Multiaddr{underlay1, underlay2}, overlay, networkID, nonce)
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
	clientAddress := swarm.RandAddress(t)
	client := hive.New(serverRecorder, addressbook, networkID, false, true, clientAddress, logger)
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
			for j := 0; j < n; j++ {
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
		overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, block)
		if err != nil {
			t.Fatal(err)
		}
		bzzAddr, err := bzz.NewAddress(signer, underlays, overlay, networkID, nonce)
		if err != nil {
			t.Fatal(err)
		}

		bzzAddresses = append(bzzAddresses, *bzzAddr)
		overlays = append(overlays, bzzAddr.Overlay)
		if err := addressbook.Put(bzzAddr.Overlay, *bzzAddr); err != nil {
			t.Fatal(err)
		}

		wantMsgs[i/hive.MaxBatchSize].Peers = append(wantMsgs[i/hive.MaxBatchSize].Peers, &pb.BzzAddress{
			Overlay:   bzzAddresses[i].Overlay.Bytes(),
			Underlay:  bzz.SerializeUnderlays(bzzAddresses[i].Underlays),
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
			wantMsgs: []pb.Peers{{Peers: []*pb.BzzAddress{
				{
					Overlay:   bzzAddresses[len(bzzAddresses)-1].Overlay.Bytes(),
					Underlay:  bzz.SerializeUnderlays([]ma.Multiaddr{bzzAddresses[len(bzzAddresses)-1].Underlays[0]}),
					Signature: bzzAddresses[len(bzzAddresses)-1].Signature,
					Nonce:     nonce,
				},
			}}},
			wantOverlays: []swarm.Address{overlays[len(overlays)-1]},
			wantBzzAddresses: []bzz.Address{
				{
					Underlays:       []ma.Multiaddr{bzzAddresses[len(bzzAddresses)-1].Underlays[0]},
					Overlay:         bzzAddresses[len(bzzAddresses)-1].Overlay,
					Signature:       bzzAddresses[len(bzzAddresses)-1].Signature,
					Nonce:           bzzAddresses[len(bzzAddresses)-1].Nonce,
					EthereumAddress: bzzAddresses[len(bzzAddresses)-1].EthereumAddress,
				},
			},
			allowPrivateCIDRs: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			addressbookclean := ab.New(mock.NewStateStore())

			streamer := streamtest.New()
			// create a hive server that handles the incoming stream
			serverAddress := swarm.RandAddress(t)
			server := hive.New(streamer, addressbookclean, networkID, false, true, serverAddress, logger)
			testutil.CleanupCloser(t, server)

			// setup the stream recorder to record stream data
			recorder := streamtest.New(
				streamtest.WithProtocols(server.Protocol()),
			)

			// create a hive client that will do broadcast
			clientAddress := swarm.RandAddress(t)
			client := hive.New(recorder, addressbook, networkID, false, tc.allowPrivateCIDRs, clientAddress, logger)

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
				comparePeerMsgs(t, messages[0].Peers, tc.wantMsgs[i].Peers)
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
	clientAddress := swarm.RandAddress(t)

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
	overlay1, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, block)
	if err != nil {
		t.Fatal(err)
	}
	bzzAddr1, err := bzz.NewAddress(signer, []ma.Multiaddr{underlay1}, overlay1, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(bzzAddr1.Overlay, *bzzAddr1); err != nil {
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
	bzzAddrClient, err := bzz.NewAddress(signerClient, []ma.Multiaddr{underlayClient}, clientAddress, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(clientAddress, *bzzAddrClient); err != nil {
		t.Fatal(err)
	}

	// Setup server
	streamer := streamtest.New()
	server := hive.New(streamer, addressbookclean, networkID, false, true, serverAddress, logger)
	testutil.CleanupCloser(t, server)

	serverRecorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	// Setup client
	client := hive.New(serverRecorder, addressbook, networkID, false, true, clientAddress, logger)
	testutil.CleanupCloser(t, client)

	// Try to broadcast: peer1, clientAddress (self), and another peer
	peersIncludingSelf := []swarm.Address{bzzAddr1.Overlay, clientAddress, peer1}

	err = client.BroadcastPeers(context.Background(), serverAddress, peersIncludingSelf...)
	if err != nil {
		t.Fatal(err)
	}

	// Get records
	records, err := serverRecorder.Records(serverAddress, "hive", "1.1.0", "peers")
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

	// Create addresses
	serverAddress := swarm.RandAddress(t)
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
	overlay1, err := crypto.NewOverlayAddress(pk1.PublicKey, networkID, block)
	if err != nil {
		t.Fatal(err)
	}
	bzzAddr1, err := bzz.NewAddress(signer1, []ma.Multiaddr{underlay1}, overlay1, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if err := addressbook.Put(bzzAddr1.Overlay, *bzzAddr1); err != nil {
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
	bzzAddrServer, err := bzz.NewAddress(signerServer, []ma.Multiaddr{underlayServer}, serverAddress, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	// Add server's own address to client's addressbook (so client can send it)
	if err := addressbook.Put(serverAddress, *bzzAddrServer); err != nil {
		t.Fatal(err)
	}

	// Setup server that will receive peers including its own address
	streamer := streamtest.New()
	server := hive.New(streamer, addressbookclean, networkID, false, true, serverAddress, logger)
	testutil.CleanupCloser(t, server)

	serverRecorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)

	// Setup client
	client := hive.New(serverRecorder, addressbook, networkID, false, true, clientAddress, logger)
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
	_, err = addressbookclean.Get(bzzAddr1.Overlay)
	if err != nil {
		t.Fatalf("expected server to have valid peer in addressbook, got error: %v", err)
	}
}
