// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia_test

import (
	"context"
	"errors"
	"io/ioutil"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	beeCrypto "github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/kademlia"
	"github.com/ethersphere/bee/pkg/kademlia/pslice"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	mockstate "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// TestNeighborhoodDepth tests that the kademlia depth changes correctly
// according to the change to known peers slice. This inadvertently tests
// the functionality in `manage()` method, however this is not the main aim of the
// test, since depth calculation happens there and in the disconnect method.
// A more in depth testing of the functionality in `manage()` is explicitly
// tested in TestManage below.
func TestNeighborhoodDepth(t *testing.T) {
	var (
		conns int32 // how many connect calls were made to the p2p mock

		base, kad, ab, _, signer = newTestKademlia(&conns, nil)
		peers                    []swarm.Address
		binEight                 []swarm.Address
	)

	defer kad.Close()

	for i := 0; i < 8; i++ {
		addr := test.RandomAddressAt(base, i)
		peers = append(peers, addr)
	}

	for i := 0; i < 2; i++ {
		addr := test.RandomAddressAt(base, 8)
		binEight = append(binEight, addr)
	}

	// check empty kademlia depth is 0
	kDepth(t, kad, 0)

	// add two bin 8 peers, verify depth still 0
	add(t, signer, kad, ab, binEight, 0, 2)
	kDepth(t, kad, 0)

	// add two first peers (po0,po1)
	add(t, signer, kad, ab, peers, 0, 2)

	// wait for 4 connections
	waitConns(t, &conns, 4)

	// depth 2 (shallowest empty bin)
	kDepth(t, kad, 2)

	for i := 2; i < len(peers)-1; i++ {
		addOne(t, signer, kad, ab, peers[i])

		// wait for one connection
		waitConn(t, &conns)

		// depth is i+1
		kDepth(t, kad, i+1)
	}

	// the last peer in bin 7 which is empty we insert manually,
	addOne(t, signer, kad, ab, peers[len(peers)-1])
	waitConn(t, &conns)

	// depth is 8 because we have nnLowWatermark neighbors in bin 8
	kDepth(t, kad, 8)

	// now add another ONE peer at depth+1, and expect the depth to still
	// stay 8, because the counter for nnLowWatermark would be reached only at the next
	// depth iteration when calculating depth
	addr := test.RandomAddressAt(base, 9)
	addOne(t, signer, kad, ab, addr)
	waitConn(t, &conns)
	kDepth(t, kad, 8)

	// fill the rest up to the bin before last and check that everything works at the edges
	for i := 10; i < kademlia.MaxBins-1; i++ {
		addr := test.RandomAddressAt(base, i)
		addOne(t, signer, kad, ab, addr)
		waitConn(t, &conns)
		kDepth(t, kad, i-1)
	}

	// add a whole bunch of peers in bin 13, expect depth to stay at 13
	for i := 0; i < 15; i++ {
		addr = test.RandomAddressAt(base, 13)
		addOne(t, signer, kad, ab, addr)
	}

	waitConns(t, &conns, 15)
	kDepth(t, kad, 13)

	// add one at 14 - depth should be now 14
	addr = test.RandomAddressAt(base, 14)
	addOne(t, signer, kad, ab, addr)
	kDepth(t, kad, 14)

	addr2 := test.RandomAddressAt(base, 15)
	addOne(t, signer, kad, ab, addr2)
	kDepth(t, kad, 14)

	addr3 := test.RandomAddressAt(base, 15)
	addOne(t, signer, kad, ab, addr3)
	kDepth(t, kad, 15)

	// now remove that peer and check that the depth is back at 14
	removeOne(kad, addr3)
	kDepth(t, kad, 14)

	// remove the peer at bin 1, depth should be 1
	removeOne(kad, peers[1])
	kDepth(t, kad, 1)
}

// TestManage explicitly tests that new connections are made according to
// the addition or subtraction of peers to the knownPeers and connectedPeers
// data structures. It tests that kademlia will try to initiate (emphesis on _initiate_,
// since right now this test does not test for a mark-and-sweep behaviour of kademlia
// that will prune or disconnect old or less performent nodes when a certain condition
// in a bin has been met - these are future optimizations that still need be sketched out)
// connections when a certain bin is _not_ saturated, and that kademlia does _not_ try
// to initiate connections on a saturated bin.
// Saturation from the local node's perspective means whether a bin has enough connections
// on a given bin.
// What Saturation does _not_ mean: that all nodes are performent, that all nodes we know of
// in a given bin are connected (since some of them might be offline)
func TestManage(t *testing.T) {
	var (
		conns int32 // how many connect calls were made to the p2p mock

		saturationVal  = false
		saturationFunc = func(bin, depth uint8, peers *pslice.PSlice) bool {
			return saturationVal
		}
		base, kad, ab, _, signer = newTestKademlia(&conns, saturationFunc)
	)
	// first, saturationFunc returns always false, this means that the bin is not saturated,
	// hence we expect that every peer we add to kademlia will be connected to
	for i := 0; i < 50; i++ {
		addr := test.RandomAddressAt(base, 0)
		addOne(t, signer, kad, ab, addr)
	}

	waitConns(t, &conns, 50)
	saturationVal = true

	// now since the bin is "saturated", no new connections should be made
	for i := 0; i < 50; i++ {
		addr := test.RandomAddressAt(base, 0)
		addOne(t, signer, kad, ab, addr)
	}

	waitConns(t, &conns, 0)

	// check other bins just for fun
	for i := 0; i < 16; i++ {
		for j := 0; j < 10; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, signer, kad, ab, addr)
		}
	}
	waitConns(t, &conns, 0)
}

// TestBinSaturation tests the builtin binSaturated function.
// the test must have two phases of adding peers so that the section
// beyond the first flow control statement gets hit (if po >= depth),
// meaning, on the first iteration we add peer and this condition will always
// be true since depth is increasingly moving deeper, but then we add more peers
// in shallower depth for the rest of the function to be executed
func TestBinSaturation(t *testing.T) {
	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(&conns, nil)
		peers                    []swarm.Address
	)

	// add two peers in a few bins to generate some depth >= 0, this will
	// make the next iteration result in binSaturated==true, causing no new
	// connections to be made
	for i := 0; i < 5; i++ {
		for j := 0; j < 2; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, signer, kad, ab, addr)
			peers = append(peers, addr)
		}
	}
	waitConns(t, &conns, 10)

	// add one more peer in each bin shallower than depth and
	// expect no connections due to saturation. if we add a peer within
	// depth, the short circuit will be hit and we will connect to the peer
	for i := 0; i < 4; i++ {
		addr := test.RandomAddressAt(base, i)
		addOne(t, signer, kad, ab, addr)
	}
	waitConns(t, &conns, 0)

	// add one peer in a bin higher (unsaturated) and expect one connection
	addr := test.RandomAddressAt(base, 6)
	addOne(t, signer, kad, ab, addr)

	waitConns(t, &conns, 1)

	// again, one bin higher
	addr = test.RandomAddressAt(base, 7)
	addOne(t, signer, kad, ab, addr)

	waitConns(t, &conns, 1)

	// this is in order to hit the `if size < 2` in the saturation func
	removeOne(kad, peers[2])
	waitConns(t, &conns, 1)
}

// TestNotifierHooks tests that the Connected/Disconnected hooks
// result in the correct behavior once called.
func TestNotifierHooks(t *testing.T) {
	var (
		base, kad, ab, _, signer = newTestKademlia(nil, nil)
		peer                     = test.RandomAddressAt(base, 3)
		addr                     = test.RandomAddressAt(peer, 4) // address which is closer to peer
	)

	connectOne(t, signer, kad, ab, peer)

	p, err := kad.ClosestPeer(addr)
	if err != nil {
		t.Fatal(err)
	}

	if !p.Equal(peer) {
		t.Fatal("got wrong peer address")
	}

	// disconnect the peer, expect error
	kad.Disconnected(peer)
	_, err = kad.ClosestPeer(addr)
	if !errors.Is(err, topology.ErrNotFound) {
		t.Fatalf("expected topology.ErrNotFound but got %v", err)
	}
}

// TestDiscoveryHooks check that a peer is gossiped to other peers
// once we establish a connection to this peer. This could be as a result of
// us proactively dialing in to a peer, or when a peer dials in.
func TestDiscoveryHooks(t *testing.T) {
	var (
		conns                    int32
		_, kad, ab, disc, signer = newTestKademlia(&conns, nil)
		p1, p2, p3               = test.RandomAddress(), test.RandomAddress(), test.RandomAddress()
	)

	// first add a peer from AddPeer, wait for the connection
	addOne(t, signer, kad, ab, p1)
	waitConn(t, &conns)
	// add another peer from AddPeer, wait for the connection
	// then check that peers are gossiped to each other via discovery
	addOne(t, signer, kad, ab, p2)
	waitConn(t, &conns)
	waitBcast(t, disc, p1, p2)
	waitBcast(t, disc, p2, p1)

	disc.Reset()

	// add another peer that dialed in, check that all peers gossiped
	// correctly to each other
	connectOne(t, signer, kad, ab, p3)
	waitBcast(t, disc, p1, p3)
	waitBcast(t, disc, p2, p3)
	waitBcast(t, disc, p3, p1, p2)
}

func TestBackoff(t *testing.T) {
	// cheat and decrease the timer
	defer func(t time.Duration) {
		*kademlia.TimeToRetry = t
	}(*kademlia.TimeToRetry)

	*kademlia.TimeToRetry = 500 * time.Millisecond

	var (
		conns int32 // how many connect calls were made to the p2p mock

		base, kad, ab, _, signer = newTestKademlia(&conns, nil)
	)

	// add one peer, wait for connection
	addr := test.RandomAddressAt(base, 1)
	addOne(t, signer, kad, ab, addr)

	waitConns(t, &conns, 1)

	// remove that peer
	removeOne(kad, addr)

	// wait for 100ms, add another peer, expect just one more connection
	time.Sleep(100 * time.Millisecond)
	addr = test.RandomAddressAt(base, 1)
	addOne(t, signer, kad, ab, addr)

	waitConns(t, &conns, 1)

	// wait for another 400ms, add another, expect 2 connections
	time.Sleep(400 * time.Millisecond)
	addr = test.RandomAddressAt(base, 1)
	addOne(t, signer, kad, ab, addr)

	waitConns(t, &conns, 2)
}

// TestClosestPeer tests that ClosestPeer method returns closest connected peer to a given address.
func TestClosestPeer(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	base := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000
	connectedPeers := []p2p.Peer{
		{
			Address: swarm.MustParseHexAddress("8000000000000000000000000000000000000000000000000000000000000000"), // binary 1000 -> po 0 to base
		},
		{
			Address: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // binary 0100 -> po 1 to base
		},
		{
			Address: swarm.MustParseHexAddress("6000000000000000000000000000000000000000000000000000000000000000"), // binary 0110 -> po 1 to base
		},
	}

	disc := mock.NewDiscovery()
	ab := addressbook.New(mockstate.NewStateStore())
	var conns int32

	kad := kademlia.New(kademlia.Options{Base: base, Discovery: disc, AddressBook: ab, P2P: p2pMock(&conns), Logger: logger})
	defer kad.Close()

	pk, _ := crypto.GenerateSecp256k1Key()
	for _, v := range connectedPeers {
		addOne(t, beeCrypto.NewDefaultSigner(pk), kad, ab, v.Address)
	}
	waitConns(t, &conns, 3)

	for _, tc := range []struct {
		chunkAddress swarm.Address // chunk address to test
		expectedPeer int           // points to the index of the connectedPeers slice. -1 means self (baseOverlay)
	}{
		{
			chunkAddress: swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000"), // 0111, wants peer 2
			expectedPeer: 2,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("c000000000000000000000000000000000000000000000000000000000000000"), // 1100, want peer 0
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("e000000000000000000000000000000000000000000000000000000000000000"), // 1110, want peer 0
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("a000000000000000000000000000000000000000000000000000000000000000"), // 1010, want peer 0
			expectedPeer: 0,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // 0100, want peer 1
			expectedPeer: 1,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000"), // 0101, want peer 1
			expectedPeer: 1,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("0000001000000000000000000000000000000000000000000000000000000000"), // want self
			expectedPeer: -1,
		},
	} {
		peer, err := kad.ClosestPeer(tc.chunkAddress)
		if err != nil {
			if tc.expectedPeer == -1 && !errors.Is(err, topology.ErrWantSelf) {
				t.Fatalf("wanted %v but got %v", topology.ErrWantSelf, err)
			}
			continue
		}

		expected := connectedPeers[tc.expectedPeer].Address

		if !peer.Equal(expected) {
			t.Fatalf("peers not equal. got %s expected %s", peer, expected)
		}
	}
}

func TestMarshal(t *testing.T) {
	var (
		_, kad, ab, _, signer = newTestKademlia(nil, nil)
	)
	a := test.RandomAddress()
	addOne(t, signer, kad, ab, a)
	_, err := kad.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
}

func newTestKademlia(connCounter *int32, f func(bin, depth uint8, peers *pslice.PSlice) bool) (swarm.Address, *kademlia.Kad, addressbook.Interface, *mock.Discovery, beeCrypto.Signer) {
	var (
		base   = test.RandomAddress() // base address
		p2p    = p2pMock(connCounter)
		logger = logging.New(ioutil.Discard, 0)                                                                                            // logger
		ab     = addressbook.New(mockstate.NewStateStore())                                                                                // address book
		disc   = mock.NewDiscovery()                                                                                                       // mock discovery
		kad    = kademlia.New(kademlia.Options{Base: base, Discovery: disc, AddressBook: ab, P2P: p2p, Logger: logger, SaturationFunc: f}) // kademlia instance
	)

	pk, _ := crypto.GenerateSecp256k1Key()
	return base, kad, ab, disc, beeCrypto.NewDefaultSigner(pk)
}

func p2pMock(counter *int32) p2p.Service {
	p2ps := p2pmock.New(p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (*bzz.Address, error) {
		if counter != nil {
			_ = atomic.AddInt32(counter, 1)
		}
		return nil, nil
	}))

	return p2ps
}

func removeOne(k *kademlia.Kad, peer swarm.Address) {
	k.Disconnected(peer)
}

const underlayBase = "/ip4/127.0.0.1/tcp/7070/dns/"

func connectOne(t *testing.T, signer beeCrypto.Signer, k *kademlia.Kad, ab addressbook.Putter, peer swarm.Address) {
	t.Helper()
	multiaddr, err := ma.NewMultiaddr(underlayBase + peer.String())
	if err != nil {
		t.Fatal(err)
	}

	bzzAddr, err := bzz.NewAddress(signer, multiaddr, peer, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(peer, *bzzAddr); err != nil {
		t.Fatal(err)
	}
	_ = k.Connected(context.Background(), peer)
}

func addOne(t *testing.T, signer beeCrypto.Signer, k *kademlia.Kad, ab addressbook.Putter, peer swarm.Address) {
	t.Helper()
	multiaddr, err := ma.NewMultiaddr(underlayBase + peer.String())
	if err != nil {
		t.Fatal(err)
	}
	bzzAddr, err := bzz.NewAddress(signer, multiaddr, peer, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(peer, *bzzAddr); err != nil {
		t.Fatal(err)
	}

	if err := ab.Put(peer, *bzzAddr); err != nil {
		t.Fatal(err)
	}
	_ = k.AddPeer(context.Background(), peer)
}

func add(t *testing.T, signer beeCrypto.Signer, k *kademlia.Kad, ab addressbook.Putter, peers []swarm.Address, offset, number int) {
	t.Helper()
	for i := offset; i < offset+number; i++ {
		addOne(t, signer, k, ab, peers[i])
	}
}

func kDepth(t *testing.T, k *kademlia.Kad, d int) {
	t.Helper()
	var depth int
	for i := 0; i < 50; i++ {
		depth = int(k.NeighborhoodDepth())
		if depth == d {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for depth. want %d got %d", d, depth)
}

func waitConn(t *testing.T, conns *int32) {
	t.Helper()
	waitConns(t, conns, 1)
}

// waits for some connections for some time. resets the pointer value
// if the correct number of connections have been reached.
func waitConns(t *testing.T, conns *int32, exp int32) {
	t.Helper()
	var got int32
	if exp == 0 {
		// sleep for some time before checking for a 0.
		// this gives some time for unwanted connections to be
		// established.

		time.Sleep(50 * time.Millisecond)
	}
	for i := 0; i < 50; i++ {
		if atomic.LoadInt32(conns) == exp {
			atomic.StoreInt32(conns, 0)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for connections to be established. got %d want %d", got, exp)
}

// wait for discovery BroadcastPeers to happen
func waitBcast(t *testing.T, d *mock.Discovery, pivot swarm.Address, addrs ...swarm.Address) {
	t.Helper()

	for i := 0; i < 50; i++ {
		if d.Broadcasts() > 0 {
			recs, ok := d.AddresseeRecords(pivot)
			if !ok {
				t.Fatal("got no records for pivot")
			}
			oks := 0
			for _, a := range addrs {
				if !isIn(a, recs) {
					t.Fatalf("address %s not found in discovery records: %s", a, addrs)
				}
				oks++
			}

			if oks == len(addrs) {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for broadcast to happen")
}

func isIn(addr swarm.Address, addrs []swarm.Address) bool {
	for _, v := range addrs {
		if v.Equal(addr) {
			return true
		}
	}
	return false
}
