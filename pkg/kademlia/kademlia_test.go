// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia_test

import (
	"context"
	"io/ioutil"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/kademlia"
	"github.com/ethersphere/bee/pkg/kademlia/pslice"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	mockstate "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/test"
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

		p2p = p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			_ = atomic.AddInt32(&conns, 1)
			return swarm.ZeroAddress, nil
		}))

		base, kad, ab = newTestKademlia(p2p, nil)
		peers         []swarm.Address
		binEight      []swarm.Address
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
	add(t, kad, ab, binEight, 0, 2)
	kDepth(t, kad, 0)

	// add two first peers (po0,po1)
	add(t, kad, ab, peers, 0, 2)

	// wait for 4 connections
	waitConns(t, &conns, 4)

	// depth 2 (shallowest empty bin)
	kDepth(t, kad, 2)

	// reset the counter
	atomic.StoreInt32(&conns, 0)

	for i := 2; i < len(peers)-1; i++ {
		addOne(t, kad, ab, peers[i])

		// wait for one connection
		waitConn(t, &conns)

		// depth is i+1
		kDepth(t, kad, i+1)

		// reset
		atomic.StoreInt32(&conns, 0)
	}

	// the last peer in bin 7 which is empty we insert manually,
	addOne(t, kad, ab, peers[len(peers)-1])
	waitConn(t, &conns)

	// depth is 8 because we have nnLowWatermark neighbors in bin 8
	kDepth(t, kad, 8)

	atomic.StoreInt32(&conns, 0)

	// now add another ONE peer at depth+1, and expect the depth to still
	// stay 8, because the counter for nnLowWatermark would be reached only at the next
	// depth iteration when calculating depth
	addr := test.RandomAddressAt(base, 9)
	addOne(t, kad, ab, addr)
	waitConn(t, &conns)
	kDepth(t, kad, 8)

	atomic.StoreInt32(&conns, 0)

	// fill the rest up to the bin before last and check that everything works at the edges
	for i := 10; i < kademlia.MaxBins-1; i++ {
		addr := test.RandomAddressAt(base, i)
		addOne(t, kad, ab, addr)
		waitConn(t, &conns)
		kDepth(t, kad, i-1)
		atomic.StoreInt32(&conns, 0)
	}

	// add a whole bunch of peers in bin 13, expect depth to stay at 13
	for i := 0; i < 15; i++ {
		addr = test.RandomAddressAt(base, 13)
		addOne(t, kad, ab, addr)
	}

	waitConns(t, &conns, 15)
	atomic.StoreInt32(&conns, 0)
	kDepth(t, kad, 13)

	// add one at 14 - depth should be now 14
	addr = test.RandomAddressAt(base, 14)
	addOne(t, kad, ab, addr)
	kDepth(t, kad, 14)

	addr2 := test.RandomAddressAt(base, 15)
	addOne(t, kad, ab, addr2)
	kDepth(t, kad, 14)

	addr3 := test.RandomAddressAt(base, 15)
	addOne(t, kad, ab, addr3)
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

		p2p = p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			_ = atomic.AddInt32(&conns, 1)
			return swarm.ZeroAddress, nil
		}))

		saturationVal  = false
		saturationFunc = func(bin, depth uint8, peers *pslice.PSlice) bool {
			return saturationVal
		}
		base, kad, ab = newTestKademlia(p2p, saturationFunc)
	)
	// first, saturationFunc returns always false, this means that the bin is not saturated,
	// hence we expect that every peer we add to kademlia will be connected to
	for i := 0; i < 50; i++ {
		addr := test.RandomAddressAt(base, 0)
		addOne(t, kad, ab, addr)
	}

	waitConns(t, &conns, 50)
	atomic.StoreInt32(&conns, 0)
	saturationVal = true

	// now since the bin is "saturated", no new connections should be made
	for i := 0; i < 50; i++ {
		addr := test.RandomAddressAt(base, 0)
		addOne(t, kad, ab, addr)
	}

	waitConns(t, &conns, 0)

	// check other bins just for fun
	for i := 0; i < 16; i++ {
		for j := 0; j < 10; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, kad, ab, addr)
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
		conns int32 // how many connect calls were made to the p2p mock

		p2p = p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			_ = atomic.AddInt32(&conns, 1)
			return swarm.ZeroAddress, nil
		}))
		base, kad, ab = newTestKademlia(p2p, nil)

		peers []swarm.Address
	)

	// add two peers in a few bins to generate some depth >= 0, this will
	// make the next iteration result in binSaturated==true, causing no new
	// connections to be made
	for i := 0; i < 5; i++ {
		for j := 0; j < 2; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, kad, ab, addr)
			peers = append(peers, addr)
		}
	}
	waitConns(t, &conns, 10)
	atomic.StoreInt32(&conns, 0)

	// add one more peer in each bin shallower than depth and
	// expect no connections due to saturation. if we add a peer within
	// depth, the short circuit will be hit and we will connect to the peer
	for i := 0; i < 4; i++ {
		addr := test.RandomAddressAt(base, i)
		addOne(t, kad, ab, addr)
	}
	waitConns(t, &conns, 0)

	// add one peer in a bin higher (unsaturated) and expect one connection
	addr := test.RandomAddressAt(base, 6)
	addOne(t, kad, ab, addr)

	waitConns(t, &conns, 1)
	atomic.StoreInt32(&conns, 0)

	// again, one bin higher
	addr = test.RandomAddressAt(base, 7)
	addOne(t, kad, ab, addr)

	waitConns(t, &conns, 1)
	atomic.StoreInt32(&conns, 0)

	// this is in order to hit the `if size < 2` in the saturation func
	removeOne(kad, peers[2])
	waitConns(t, &conns, 1)
}

func TestBackoff(t *testing.T) {
	// cheat and decrease the timer
	defer func(t time.Duration) {
		*kademlia.TimeToRetry = t
	}(*kademlia.TimeToRetry)

	*kademlia.TimeToRetry = 500 * time.Millisecond

	var (
		conns int32 // how many connect calls were made to the p2p mock

		p2p = p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			_ = atomic.AddInt32(&conns, 1)
			return swarm.ZeroAddress, nil
		}))
		base, kad, ab = newTestKademlia(p2p, nil)
	)

	// add one peer, wait for connection
	addr := test.RandomAddressAt(base, 1)
	addOne(t, kad, ab, addr)

	waitConns(t, &conns, 1)
	atomic.StoreInt32(&conns, 0)

	// remove that peer
	removeOne(kad, addr)

	// wait for 100ms, add another peer, expect just one more connection
	time.Sleep(100 * time.Millisecond)
	addr = test.RandomAddressAt(base, 1)
	addOne(t, kad, ab, addr)

	waitConns(t, &conns, 1)
	atomic.StoreInt32(&conns, 0)

	// wait for another 400ms, add another, expect 2 connections
	time.Sleep(400 * time.Millisecond)
	addr = test.RandomAddressAt(base, 1)
	addOne(t, kad, ab, addr)

	waitConns(t, &conns, 2)
}

func TestMarshal(t *testing.T) {
	var (
		p2p = p2pmock.New(p2pmock.WithConnectFunc(func(_ context.Context, addr ma.Multiaddr) (swarm.Address, error) {
			return swarm.ZeroAddress, nil
		}))
		_, kad, ab = newTestKademlia(p2p, nil)
	)
	a := test.RandomAddress()
	addOne(t, kad, ab, a)
	_, err := kad.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
}

func newTestKademlia(p2p p2p.Service, f func(bin, depth uint8, peers *pslice.PSlice) bool) (swarm.Address, *kademlia.Kad, addressbook.Interface) {
	var (
		base   = test.RandomAddress()                                                                                                      // base address
		logger = logging.New(ioutil.Discard, 0)                                                                                            // logger
		ab     = addressbook.New(mockstate.NewStateStore())                                                                                // address book
		disc   = mock.NewDiscovery()                                                                                                       // mock discovery
		kad    = kademlia.New(kademlia.Options{Base: base, Discovery: disc, AddressBook: ab, P2P: p2p, Logger: logger, SaturationFunc: f}) // kademlia instance
	)
	return base, kad, ab
}

func removeOne(k *kademlia.Kad, peer swarm.Address) {
	k.Disconnected(peer)
}

const underlayBase = "/ip4/127.0.0.1/tcp/7070/dns/"

func addOne(t *testing.T, k *kademlia.Kad, ab addressbook.Putter, peer swarm.Address) {
	t.Helper()
	multiaddr, err := ma.NewMultiaddr(underlayBase + peer.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(peer, multiaddr); err != nil {
		t.Fatal(err)
	}
	_ = k.AddPeer(context.Background(), peer)
}

func add(t *testing.T, k *kademlia.Kad, ab addressbook.Putter, peers []swarm.Address, offset, number int) {
	t.Helper()
	for i := offset; i < offset+number; i++ {
		addOne(t, k, ab, peers[i])
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

func waitConns(t *testing.T, conns *int32, exp int32) {
	t.Helper()
	var got int32
	for i := 0; i < 50; i++ {
		got = atomic.LoadInt32(conns)
		if got == exp {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for connections to be established. got %d want %d", got, exp)
}
