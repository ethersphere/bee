// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia_test

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/shed"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/bzz"
	beeCrypto "github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/discovery/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/pkg/p2p/mock"
	mockstate "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/kademlia"
	"github.com/ethersphere/bee/pkg/topology/pslice"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var nonConnectableAddress, _ = ma.NewMultiaddr(underlayBase + "16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")

// TestNeighborhoodDepth tests that the kademlia depth changes correctly
// according to the change to known peers slice. This inadvertently tests
// the functionality in `manage()` method, however this is not the main aim of the
// test, since depth calculation happens there and in the disconnect method.
// A more in depth testing of the functionality in `manage()` is explicitly
// tested in TestManage below.
func TestNeighborhoodDepth(t *testing.T) {
	defer func(p int) {
		*kademlia.SaturationPeers = p
	}(*kademlia.SaturationPeers)
	*kademlia.SaturationPeers = 4

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
	)

	kad.SetRadius(swarm.MaxPO) // initial tests do not check for radius

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// add 2 peers in bin 8
	for i := 0; i < 2; i++ {
		addr := test.RandomAddressAt(base, 8)
		addOne(t, signer, kad, ab, addr)

		// wait for one connection
		waitConn(t, &conns)
	}
	// depth is 0
	kDepth(t, kad, 0)

	var shallowPeers []swarm.Address
	// add two first peers (po0,po1)
	for i := 0; i < 2; i++ {
		addr := test.RandomAddressAt(base, i)
		addOne(t, signer, kad, ab, addr)
		shallowPeers = append(shallowPeers, addr)

		// wait for one connection
		waitConn(t, &conns)
	}

	for _, a := range shallowPeers {
		if !kad.IsWithinDepth(a) {
			t.Fatal("expected address to be within depth")
		}
	}

	// depth 0 - bin 0 is unsaturated
	kDepth(t, kad, 0)

	for i := 2; i < 8; i++ {
		addr := test.RandomAddressAt(base, i)
		addOne(t, signer, kad, ab, addr)

		// wait for one connection
		waitConn(t, &conns)
	}
	// still zero
	kDepth(t, kad, 0)

	// now add peers from bin 0 and expect the depth
	// to shift. the depth will be that of the shallowest
	// unsaturated bin.
	for i := 0; i < 7; i++ {
		for j := 0; j < 3; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, signer, kad, ab, addr)
			waitConn(t, &conns)
		}
		kDepth(t, kad, i+1)
	}

	// depth is 7 because bin 7 is unsaturated (1 peer)
	kDepth(t, kad, 7)

	// set the radius to be lower than unsaturated, expect radius as depth
	kad.SetRadius(6)
	kDepth(t, kad, 6)

	// set the radius to MaxPO again so that intermediate checks can run
	kad.SetRadius(swarm.MaxPO)

	// expect shallow peers not in depth
	for _, a := range shallowPeers {
		if kad.IsWithinDepth(a) {
			t.Fatal("expected address to outside of depth")
		}
	}

	// now add another ONE peer at depth, and expect the depth to still
	// stay 8, because the counter for nnLowWatermark would be reached only at the next
	// depth iteration when calculating depth
	addr := test.RandomAddressAt(base, 8)
	addOne(t, signer, kad, ab, addr)
	waitConn(t, &conns)
	kDepth(t, kad, 7)

	// now fill bin 7 so that it is saturated, expect depth 8
	for i := 0; i < 3; i++ {
		addr := test.RandomAddressAt(base, 7)
		addOne(t, signer, kad, ab, addr)
		waitConn(t, &conns)
	}
	kDepth(t, kad, 8)

	// saturate bin 8
	addr = test.RandomAddressAt(base, 8)
	addOne(t, signer, kad, ab, addr)
	waitConn(t, &conns)
	kDepth(t, kad, 8)

	// again set radius to lower value, expect that as depth
	kad.SetRadius(5)
	kDepth(t, kad, 5)

	// reset radius to MaxPO for the rest of the checks
	kad.SetRadius(swarm.MaxPO)

	var addrs []swarm.Address
	// fill the rest up to the bin before last and check that everything works at the edges
	for i := 9; i < int(swarm.MaxBins); i++ {
		for j := 0; j < 4; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, signer, kad, ab, addr)
			waitConn(t, &conns)
			addrs = append(addrs, addr)
		}
		kDepth(t, kad, i)
	}

	// add a whole bunch of peers in the last bin, expect depth to stay at 31
	for i := 0; i < 15; i++ {
		addr = test.RandomAddressAt(base, int(swarm.MaxPO))
		addOne(t, signer, kad, ab, addr)
	}

	waitCounter(t, &conns, 15)
	kDepth(t, kad, 31)

	// remove one at 14, depth should be 14
	removeOne(kad, addrs[len(addrs)-5])
	kDepth(t, kad, 30)

	// empty bin 9 and expect depth 9
	for i := 0; i < 4; i++ {
		removeOne(kad, addrs[i])
	}
	kDepth(t, kad, 9)

	if !kad.IsWithinDepth(addrs[0]) {
		t.Fatal("expected address to be within depth")
	}

}

func TestEachNeighbor(t *testing.T) {
	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
		peers                    []swarm.Address
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	for i := 0; i < 15; i++ {
		addr := test.RandomAddressAt(base, i)
		peers = append(peers, addr)
	}

	add(t, signer, kad, ab, peers, 0, 15)
	waitCounter(t, &conns, 15)

	var depth uint8 = 15

	err := kad.EachNeighbor(func(adr swarm.Address, po uint8) (stop, jumpToNext bool, err error) {

		if po < depth {
			depth = po
		}
		return false, false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if depth < kad.NeighborhoodDepth() {
		t.Fatalf("incorrect depth argument pass to iterator function: expected >= %d (neighbourhood depth), got %d", kad.NeighborhoodDepth(), depth)
	}

	depth = 15
	err = kad.EachNeighborRev(func(adr swarm.Address, po uint8) (stop, jumpToNext bool, err error) {

		if po < depth {
			depth = po
		}
		return false, false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if depth < kad.NeighborhoodDepth() {
		t.Fatalf("incorrect depth argument pass to iterator function: expected >= %d (neighbourhood depth), got %d", kad.NeighborhoodDepth(), depth)
	}
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
		conns                    int32 // how many connect calls were made to the p2p mock
		saturation               = *kademlia.QuickSaturationPeers
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{BitSuffixLength: -1})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	kad.SetRadius(6)

	// first, we add peers to bin 0
	for i := 0; i < saturation; i++ {
		addr := test.RandomAddressAt(base, 0)
		addOne(t, signer, kad, ab, addr)
	}

	waitCounter(t, &conns, 4)

	// next, we add peers to the next bin
	for i := 0; i < saturation; i++ {
		addr := test.RandomAddressAt(base, 1)
		addOne(t, signer, kad, ab, addr)
	}

	waitCounter(t, &conns, 4)

	// here, we attempt to add to bin 0, but bin is saturated, so no new peers should connect to it
	for i := 0; i < saturation; i++ {
		addr := test.RandomAddressAt(base, 0)
		addOne(t, signer, kad, ab, addr)
	}

	waitCounter(t, &conns, 0)
}

func TestManageWithBalancing(t *testing.T) {
	// use "fixed" seed for this
	defer func(p int) {
		*kademlia.SaturationPeers = p
	}(*kademlia.SaturationPeers)
	*kademlia.SaturationPeers = 4
	rand.Seed(2)

	var (
		conns int32 // how many connect calls were made to the p2p mock

		saturationFuncImpl *func(bin uint8, peers, connected *pslice.PSlice) (bool, bool)
		saturationFunc     = func(bin uint8, peers, connected *pslice.PSlice) (bool, bool) {
			f := *saturationFuncImpl
			return f(bin, peers, connected)
		}
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{SaturationFunc: saturationFunc, BitSuffixLength: 2})
	)

	kad.SetRadius(swarm.MaxPO) // don't use radius for checks

	// implement satiration function (while having access to Kademlia instance)
	sfImpl := func(bin uint8, peers, connected *pslice.PSlice) (bool, bool) {
		return kad.IsBalanced(bin), false
	}
	saturationFuncImpl = &sfImpl

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// add peers for bin '0', enough to have balanced connections
	for i := 0; i < 20; i++ {
		addr := test.RandomAddressAt(base, 0)
		addOne(t, signer, kad, ab, addr)
	}

	waitBalanced(t, kad, 0)

	// add peers for other bins, enough to have balanced connections
	for i := 1; i <= int(swarm.MaxPO); i++ {
		for j := 0; j < 20; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, signer, kad, ab, addr)
		}
		// sanity check depth
		kDepth(t, kad, i)
	}

	// Without introducing ExtendedPO / ExtendedProximity, we could only have balanced connections until a depth of 12
	// That is because, the proximity expected for a balanced connection is Bin + 1 + suffix length
	// But, Proximity(one, other) is limited to return MaxPO.
	// So, when we get to 1 + suffix length near MaxPO, our expected proximity is not returned,
	// even if the addresses match in the expected number of bits, because of the MaxPO limiting
	// Without extendedPO, suffix length is 2, + 1 = 3, MaxPO is 15,
	// so we could only have balanced connections for up until bin 12, but not bin 13,
	// as we would be expecting proximity of pseudoaddress-balancedConnection as 16 and get 15 only

	for i := 1; i <= int(swarm.MaxPO); i++ {
		waitBalanced(t, kad, uint8(i))
	}
}

// TestBinSaturation tests the builtin binSaturated function.
// the test must have two phases of adding peers so that the section
// beyond the first flow control statement gets hit (if po >= depth),
// meaning, on the first iteration we add peer and this condition will always
// be true since depth is increasingly moving deeper, but then we add more peers
// in shallower depth for the rest of the function to be executed
func TestBinSaturation(t *testing.T) {
	defer func(p int) {
		*kademlia.QuickSaturationPeers = p
	}(*kademlia.QuickSaturationPeers)
	*kademlia.QuickSaturationPeers = 2

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{BitSuffixLength: -1})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	kad.SetRadius(6)

	// add two peers in a few bins to generate some depth >= 0, this will
	// make the next iteration result in binSaturated==true, causing no new
	// connections to be made
	for i := 0; i < 5; i++ {
		for j := 0; j < 2; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, signer, kad, ab, addr)
		}
	}
	waitCounter(t, &conns, 10)

	// add one more peer in each bin shallower than depth and
	// expect no connections due to saturation. if we add a peer within
	// depth, the short circuit will be hit and we will connect to the peer
	for i := 0; i < 4; i++ {
		addr := test.RandomAddressAt(base, i)
		addOne(t, signer, kad, ab, addr)
	}
	waitCounter(t, &conns, 0)

	// add one peer in a bin higher (unsaturated) and expect one connection
	addr := test.RandomAddressAt(base, 6)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

	// again, one bin higher
	addr = test.RandomAddressAt(base, 7)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

}

func TestOversaturation(t *testing.T) {
	defer func(p int) {
		*kademlia.OverSaturationPeers = p
	}(*kademlia.OverSaturationPeers)
	*kademlia.OverSaturationPeers = 8

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
	)
	kad.SetRadius(swarm.MaxPO) // don't use radius for checks

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// Add maximum accepted number of peers up until bin 5 without problems
	for i := 0; i < 6; i++ {
		for j := 0; j < *kademlia.OverSaturationPeers; j++ {
			addr := test.RandomAddressAt(base, i)
			// if error is not nil as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
		}
		// see depth is limited to currently added peers proximity
		kDepth(t, kad, i)
	}

	// see depth is 5
	kDepth(t, kad, 5)

	for k := 0; k < 5; k++ {
		// no further connections can be made
		for l := 0; l < 3; l++ {
			addr := test.RandomAddressAt(base, k)
			// if error is not as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, topology.ErrOversaturated)
			// check that pick works correctly
			if kad.Pick(p2p.Peer{Address: addr}) {
				t.Fatal("should not pick the peer")
			}
		}
		// see depth is still as expected
		kDepth(t, kad, 5)
	}

	// see we can still add / not limiting more peers in neighborhood depth
	for m := 0; m < 12; m++ {
		addr := test.RandomAddressAt(base, 5)
		// if error is not nil as specified, connectOne goes fatal
		connectOne(t, signer, kad, ab, addr, nil)
		// see depth is still as expected
		kDepth(t, kad, 5)
	}
}

func TestOversaturationBootnode(t *testing.T) {
	defer func(p int) {
		*kademlia.OverSaturationPeers = p
	}(*kademlia.OverSaturationPeers)
	*kademlia.OverSaturationPeers = 4

	defer func(p int) {
		*kademlia.SaturationPeers = p
	}(*kademlia.SaturationPeers)
	*kademlia.SaturationPeers = 4

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{BootnodeMode: true})
	)
	kad.SetRadius(swarm.MaxPO) // don't use radius for checks

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// Add maximum accepted number of peers up until bin 5 without problems
	for i := 0; i < 6; i++ {
		for j := 0; j < *kademlia.OverSaturationPeers; j++ {
			addr := test.RandomAddressAt(base, i)
			// if error is not nil as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
		}
		// see depth is limited to currently added peers proximity
		kDepth(t, kad, i)
	}

	// see depth is 5
	kDepth(t, kad, 5)

	for k := 0; k < 5; k++ {
		// further connections should succeed outside of depth
		for l := 0; l < 3; l++ {
			addr := test.RandomAddressAt(base, k)
			// if error is not as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
			// check that pick works correctly
			if !kad.Pick(p2p.Peer{Address: addr}) {
				t.Fatal("should pick the peer but didnt")
			}
		}
		// see depth is still as expected
		kDepth(t, kad, 5)
	}

	// see we can still add / not limiting more peers in neighborhood depth
	for m := 0; m < 12; m++ {
		addr := test.RandomAddressAt(base, 5)
		// if error is not nil as specified, connectOne goes fatal
		connectOne(t, signer, kad, ab, addr, nil)
		// see depth is still as expected
		kDepth(t, kad, 5)
	}
}

func TestBootnodeMaxConnections(t *testing.T) {
	defer func(p int) {
		*kademlia.BootnodeOverSaturationPeers = p
	}(*kademlia.BootnodeOverSaturationPeers)
	*kademlia.BootnodeOverSaturationPeers = 4

	defer func(p int) {
		*kademlia.SaturationPeers = p
	}(*kademlia.SaturationPeers)
	*kademlia.SaturationPeers = 4

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{BootnodeMode: true})
	)
	kad.SetRadius(swarm.MaxPO) // don't use radius for checks

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// Add maximum accepted number of peers up until bin 5 without problems
	for i := 0; i < 6; i++ {
		for j := 0; j < *kademlia.BootnodeOverSaturationPeers; j++ {
			addr := test.RandomAddressAt(base, i)
			// if error is not nil as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
		}
		// see depth is limited to currently added peers proximity
		kDepth(t, kad, i)
	}

	// see depth is 5
	kDepth(t, kad, 5)

	depth := 5
	outSideDepthPeers := 5

	for k := 0; k < depth; k++ {
		// further connections should succeed outside of depth
		for l := 0; l < outSideDepthPeers; l++ {
			addr := test.RandomAddressAt(base, k)
			// if error is not as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
			// check that pick works correctly
			if !kad.Pick(p2p.Peer{Address: addr}) {
				t.Fatal("should pick the peer but didnt")
			}
		}
	}

	got := atomic.LoadInt32(&conns)
	want := -int32(depth * outSideDepthPeers)
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

// TestNotifierHooks tests that the Connected/Disconnected hooks
// result in the correct behavior once called.
func TestNotifierHooks(t *testing.T) {
	t.Skip("disabled due to kademlia inconsistencies hotfix")
	var (
		base, kad, ab, _, signer = newTestKademlia(t, nil, nil, kademlia.Options{})
		peer                     = test.RandomAddressAt(base, 3)
		addr                     = test.RandomAddressAt(peer, 4) // address which is closer to peer
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	connectOne(t, signer, kad, ab, peer, nil)

	p, err := kad.ClosestPeer(addr, true)
	if err != nil {
		t.Fatal(err)
	}

	if !p.Equal(peer) {
		t.Fatal("got wrong peer address")
	}

	// disconnect the peer, expect error
	kad.Disconnected(p2p.Peer{Address: peer})
	_, err = kad.ClosestPeer(addr, true)
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
		_, kad, ab, disc, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
		p1, p2, p3               = test.RandomAddress(), test.RandomAddress(), test.RandomAddress()
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// first add a peer from AddPeers, wait for the connection
	addOne(t, signer, kad, ab, p1)
	waitConn(t, &conns)
	// add another peer from AddPeers, wait for the connection
	// then check that peers are gossiped to each other via discovery
	addOne(t, signer, kad, ab, p2)
	waitConn(t, &conns)
	waitBcast(t, disc, p1, p2)
	waitBcast(t, disc, p2, p1)

	disc.Reset()

	// add another peer that dialed in, check that all peers gossiped
	// correctly to each other
	connectOne(t, signer, kad, ab, p3, nil)
	waitBcast(t, disc, p1, p3)
	waitBcast(t, disc, p2, p3)
	waitBcast(t, disc, p3, p1, p2)
}

func TestAnnounceTo(t *testing.T) {
	var (
		conns                    int32
		_, kad, ab, disc, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
		p1, p2                   = test.RandomAddress(), test.RandomAddress()
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// first add a peer from AddPeers, wait for the connection
	addOne(t, signer, kad, ab, p1)
	waitConn(t, &conns)

	if err := kad.AnnounceTo(context.Background(), p1, p2, true); err != nil {
		t.Fatal(err)
	}
	waitBcast(t, disc, p1, p2)

	if err := kad.AnnounceTo(context.Background(), p1, p2, false); err == nil {
		t.Fatal("expected error")
	}
}

func TestBackoff(t *testing.T) {
	// cheat and decrease the timer
	defer func(t time.Duration) {
		*kademlia.TimeToRetry = t
	}(*kademlia.TimeToRetry)

	*kademlia.TimeToRetry = 500 * time.Millisecond

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	// add one peer, wait for connection
	addr := test.RandomAddressAt(base, 1)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

	// remove that peer
	removeOne(kad, addr)

	// wait for 100ms, add another peer, expect just one more connection
	time.Sleep(100 * time.Millisecond)
	addr = test.RandomAddressAt(base, 1)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

	// wait for another 400ms, add another, expect 2 connections
	time.Sleep(400 * time.Millisecond)
	addr = test.RandomAddressAt(base, 1)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 2)
}

func TestAddressBookPrune(t *testing.T) {
	// test pruning addressbook after successive failed connect attempts
	// cheat and decrease the timer
	defer func(t time.Duration) {
		*kademlia.TimeToRetry = t
	}(*kademlia.TimeToRetry)

	*kademlia.TimeToRetry = 20 * time.Millisecond

	var (
		conns, failedConns       int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, &failedConns, kademlia.Options{})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	nonConnPeer, err := bzz.NewAddress(signer, nonConnectableAddress, test.RandomAddressAt(base, 1), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(nonConnPeer.Overlay, *nonConnPeer); err != nil {
		t.Fatal(err)
	}

	// add non connectable peer, check connection and failed connection counters
	kad.AddPeers(nonConnPeer.Overlay)
	waitCounter(t, &conns, 0)
	waitCounter(t, &failedConns, 1)

	_, err = ab.Get(nonConnPeer.Overlay)
	if err != addressbook.ErrNotFound {
		t.Fatal(err)
	}

	addr := test.RandomAddressAt(base, 1)
	addr1 := test.RandomAddressAt(base, 1)
	addr2 := test.RandomAddressAt(base, 1)

	// add one valid peer to initiate the retry, check connection and failed connection counters
	addOne(t, signer, kad, ab, addr)
	waitCounter(t, &conns, 1)
	waitCounter(t, &failedConns, 0)

	_, err = ab.Get(nonConnPeer.Overlay)
	if err != addressbook.ErrNotFound {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	// add one valid peer to initiate the retry, check connection and failed connection counters
	addOne(t, signer, kad, ab, addr1)
	waitCounter(t, &conns, 1)
	waitCounter(t, &failedConns, 0)

	_, err = ab.Get(nonConnPeer.Overlay)
	if err != addressbook.ErrNotFound {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	// add one valid peer to initiate the retry, check connection and failed connection counters
	addOne(t, signer, kad, ab, addr2)
	waitCounter(t, &conns, 1)
	waitCounter(t, &failedConns, 0)

	_, err = ab.Get(nonConnPeer.Overlay)
	if err != addressbook.ErrNotFound {
		t.Fatal(err)
	}
}

func TestAddressBookQuickPrune(t *testing.T) {
	// test pruning addressbook after successive failed connect attempts
	// cheat and decrease the timer
	defer func(t time.Duration) {
		*kademlia.TimeToRetry = t
	}(*kademlia.TimeToRetry)

	*kademlia.TimeToRetry = 50 * time.Millisecond

	var (
		conns, failedConns       int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, &failedConns, kademlia.Options{})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	time.Sleep(100 * time.Millisecond)

	nonConnPeer, err := bzz.NewAddress(signer, nonConnectableAddress, test.RandomAddressAt(base, 1), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(nonConnPeer.Overlay, *nonConnPeer); err != nil {
		t.Fatal(err)
	}

	addr := test.RandomAddressAt(base, 1)
	// add one valid peer
	addOne(t, signer, kad, ab, addr)
	waitCounter(t, &conns, 1)
	waitCounter(t, &failedConns, 0)

	// add non connectable peer, check connection and failed connection counters
	kad.AddPeers(nonConnPeer.Overlay)
	waitCounter(t, &conns, 0)
	waitCounter(t, &failedConns, 1)

	_, err = ab.Get(nonConnPeer.Overlay)
	if !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatal(err)
	}
}

// TestClosestPeer tests that ClosestPeer method returns closest connected peer to a given address.
func TestClosestPeer(t *testing.T) {
	metricsDB, err := shed.NewDB("", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := metricsDB.Close(); err != nil {
			t.Fatal(err)
		}
	})

	_ = waitPeers
	t.Skip("disabled due to kademlia inconsistencies hotfix")

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

	kad := kademlia.New(base, ab, disc, p2pMock(ab, nil, nil, nil), metricsDB, logger, kademlia.Options{})
	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	pk, _ := beeCrypto.GenerateSecp256k1Key()
	for _, v := range connectedPeers {
		addOne(t, beeCrypto.NewDefaultSigner(pk), kad, ab, v.Address)
	}

	waitPeers(t, kad, 3)

	for _, tc := range []struct {
		chunkAddress swarm.Address // chunk address to test
		expectedPeer int           // points to the index of the connectedPeers slice. -1 means self (baseOverlay)
		includeSelf  bool
	}{
		{
			chunkAddress: swarm.MustParseHexAddress("7000000000000000000000000000000000000000000000000000000000000000"), // 0111, wants peer 2
			expectedPeer: 2,
			includeSelf:  true,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("c000000000000000000000000000000000000000000000000000000000000000"), // 1100, want peer 0
			expectedPeer: 0,
			includeSelf:  true,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("e000000000000000000000000000000000000000000000000000000000000000"), // 1110, want peer 0
			expectedPeer: 0,
			includeSelf:  true,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("a000000000000000000000000000000000000000000000000000000000000000"), // 1010, want peer 0
			expectedPeer: 0,
			includeSelf:  true,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("4000000000000000000000000000000000000000000000000000000000000000"), // 0100, want peer 1
			expectedPeer: 1,
			includeSelf:  true,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("5000000000000000000000000000000000000000000000000000000000000000"), // 0101, want peer 1
			expectedPeer: 1,
			includeSelf:  true,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("0000001000000000000000000000000000000000000000000000000000000000"), // 1000 want self
			expectedPeer: -1,
			includeSelf:  true,
		},
		{
			chunkAddress: swarm.MustParseHexAddress("0000001000000000000000000000000000000000000000000000000000000000"), // 1000 want peer 1
			expectedPeer: 1,                                                                                             // smallest distance: 2894...
			includeSelf:  false,
		},
	} {
		peer, err := kad.ClosestPeer(tc.chunkAddress, tc.includeSelf)
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

func TestKademlia_SubscribePeersChange(t *testing.T) {
	testSignal := func(t *testing.T, k *kademlia.Kad, c <-chan struct{}) {
		t.Helper()

		select {
		case _, ok := <-c:
			if !ok {
				t.Error("closed signal channel")
			}
		case <-time.After(1 * time.Second):
			t.Error("timeout")
		}
	}

	t.Run("single subscription", func(t *testing.T) {
		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer kad.Close()

		c, u := kad.SubscribePeersChange()
		defer u()

		addr := test.RandomAddressAt(base, 9)
		addOne(t, sg, kad, ab, addr)

		testSignal(t, kad, c)
	})

	t.Run("single subscription, remove peer", func(t *testing.T) {
		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer kad.Close()

		c, u := kad.SubscribePeersChange()
		defer u()

		addr := test.RandomAddressAt(base, 9)
		addOne(t, sg, kad, ab, addr)

		testSignal(t, kad, c)

		removeOne(kad, addr)
		testSignal(t, kad, c)
	})

	t.Run("multiple subscriptions", func(t *testing.T) {
		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer kad.Close()

		c1, u1 := kad.SubscribePeersChange()
		defer u1()

		c2, u2 := kad.SubscribePeersChange()
		defer u2()

		for i := 0; i < 4; i++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, sg, kad, ab, addr)
		}
		testSignal(t, kad, c1)
		testSignal(t, kad, c2)
	})

	t.Run("multiple changes", func(t *testing.T) {
		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer kad.Close()

		c, u := kad.SubscribePeersChange()
		defer u()

		for i := 0; i < 4; i++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, sg, kad, ab, addr)
		}

		testSignal(t, kad, c)

		for i := 0; i < 4; i++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, sg, kad, ab, addr)
		}

		testSignal(t, kad, c)
	})

	t.Run("no depth change", func(t *testing.T) {
		_, kad, _, _, _ := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer kad.Close()

		c, u := kad.SubscribePeersChange()
		defer u()

		select {
		case _, ok := <-c:
			if !ok {
				t.Error("closed signal channel")
			}
			t.Error("signal received")
		case <-time.After(1 * time.Second):
			// all fine
		}
	})
}

func TestSnapshot(t *testing.T) {
	var conns = new(int32)
	sa, kad, ab, _, signer := newTestKademlia(t, conns, nil, kademlia.Options{})
	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	a := test.RandomAddress()
	addOne(t, signer, kad, ab, a)

	waitConn(t, conns)

	snap := kad.Snapshot()

	if snap.Connected != 1 {
		t.Errorf("expected %d connected peers but got %d", 1, snap.Connected)
	}
	if snap.Population != 1 {
		t.Errorf("expected population %d but got %d", 1, snap.Population)
	}

	po := swarm.Proximity(sa.Bytes(), a.Bytes())

	if binP := getBinPopulation(&snap.Bins, po); binP != 1 {
		t.Errorf("expected bin(%d) to have population %d but got %d", po, 1, snap.Population)
	}
}

func getBinPopulation(bins *topology.KadBins, po uint8) uint64 {
	rv := reflect.ValueOf(bins)
	bin := fmt.Sprintf("Bin%d", po)
	b0 := reflect.Indirect(rv).FieldByName(bin)
	bp := b0.FieldByName("BinPopulation")
	return bp.Uint()
}

func TestStart(t *testing.T) {
	var bootnodes []ma.Multiaddr
	for i := 0; i < 10; i++ {
		multiaddr, err := ma.NewMultiaddr(underlayBase + test.RandomAddress().String())
		if err != nil {
			t.Fatal(err)
		}

		bootnodes = append(bootnodes, multiaddr)
	}

	t.Run("non-empty addressbook", func(t *testing.T) {
		t.Skip("test flakes")
		var conns, failedConns int32 // how many connect calls were made to the p2p mock
		_, kad, ab, _, signer := newTestKademlia(t, &conns, &failedConns, kademlia.Options{Bootnodes: bootnodes})
		defer kad.Close()

		for i := 0; i < 3; i++ {
			peer := test.RandomAddress()
			multiaddr, err := ma.NewMultiaddr(underlayBase + peer.String())
			if err != nil {
				t.Fatal(err)
			}
			bzzAddr, err := bzz.NewAddress(signer, multiaddr, peer, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := ab.Put(peer, *bzzAddr); err != nil {
				t.Fatal(err)
			}
		}

		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}

		waitCounter(t, &conns, 3)
		waitCounter(t, &failedConns, 0)
	})

	t.Run("empty addressbook", func(t *testing.T) {
		var conns, failedConns int32 // how many connect calls were made to the p2p mock
		_, kad, _, _, _ := newTestKademlia(t, &conns, &failedConns, kademlia.Options{Bootnodes: bootnodes})
		defer kad.Close()

		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}

		waitCounter(t, &conns, 3)
		waitCounter(t, &failedConns, 0)
	})
}

func TestOutofDepthPrune(t *testing.T) {

	defer func(p int) {
		*kademlia.SaturationPeers = p
	}(*kademlia.SaturationPeers)

	defer func(p int) {
		*kademlia.OverSaturationPeers = p
	}(*kademlia.OverSaturationPeers)

	*kademlia.SaturationPeers = 4
	*kademlia.OverSaturationPeers = 8

	var (
		conns, failedConns       int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, &failedConns, kademlia.Options{})
	)

	kad.SetRadius(swarm.MaxPO) // don't use radius for checks

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer kad.Close()

	for i := 0; i < 6; i++ {
		for j := 0; j < *kademlia.OverSaturationPeers*2; j++ {
			addr := test.RandomAddressAt(base, i)
			addOne(t, signer, kad, ab, addr)
		}
		time.Sleep(time.Millisecond * 5)
		kDepth(t, kad, i)
	}

	bins := map[uint8]int{}

	_ = kad.EachPeer(func(a swarm.Address, u uint8) (stop bool, jumpToNext bool, err error) {
		bins[u]++
		return false, false, nil
	})

	for i := uint8(0); i < 5; i++ {
		if bins[i] != *kademlia.OverSaturationPeers {
			t.Fatalf("bin %d, got %d, want %d", i, bins[i], *kademlia.OverSaturationPeers)
		}
	}
}

func newTestKademlia(t *testing.T, connCounter, failedConnCounter *int32, kadOpts kademlia.Options) (swarm.Address, *kademlia.Kad, addressbook.Interface, *mock.Discovery, beeCrypto.Signer) {
	t.Helper()

	metricsDB, err := shed.NewDB("", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := metricsDB.Close(); err != nil {
			t.Fatal(err)
		}
	})
	var (
		pk, _  = beeCrypto.GenerateSecp256k1Key()                              // random private key
		signer = beeCrypto.NewDefaultSigner(pk)                                // signer
		base   = test.RandomAddress()                                          // base address
		ab     = addressbook.New(mockstate.NewStateStore())                    // address book
		p2p    = p2pMock(ab, signer, connCounter, failedConnCounter)           // p2p mock
		logger = logging.New(ioutil.Discard, 0)                                // logger
		disc   = mock.NewDiscovery()                                           // mock discovery protocol
		kad    = kademlia.New(base, ab, disc, p2p, metricsDB, logger, kadOpts) // kademlia instance
	)

	p2p.SetPickyNotifier(kad)

	return base, kad, ab, disc, signer
}

func p2pMock(ab addressbook.Interface, signer beeCrypto.Signer, counter, failedCounter *int32) p2p.Service {
	p2ps := p2pmock.New(
		p2pmock.WithConnectFunc(func(ctx context.Context, addr ma.Multiaddr) (*bzz.Address, error) {
			if addr.Equal(nonConnectableAddress) {
				_ = atomic.AddInt32(failedCounter, 1)
				return nil, errors.New("non reachable node")
			}
			if counter != nil {
				_ = atomic.AddInt32(counter, 1)
			}

			addresses, err := ab.Addresses()
			if err != nil {
				return nil, errors.New("could not fetch addresbook addresses")
			}

			for _, a := range addresses {
				if a.Underlay.Equal(addr) {
					return &a, nil
				}
			}

			address := test.RandomAddress()
			bzzAddr, err := bzz.NewAddress(signer, addr, address, 0, nil)
			if err != nil {
				return nil, err
			}

			if err := ab.Put(address, *bzzAddr); err != nil {
				return nil, err
			}

			return bzzAddr, nil
		}),
		p2pmock.WithDisconnectFunc(func(swarm.Address) error {
			if counter != nil {
				_ = atomic.AddInt32(counter, -1)
			}
			return nil
		}),
	)

	return p2ps
}

func removeOne(k *kademlia.Kad, peer swarm.Address) {
	k.Disconnected(p2p.Peer{Address: peer})
}

const underlayBase = "/ip4/127.0.0.1/tcp/1634/dns/"

func connectOne(t *testing.T, signer beeCrypto.Signer, k *kademlia.Kad, ab addressbook.Putter, peer swarm.Address, expErr error) {
	t.Helper()
	multiaddr, err := ma.NewMultiaddr(underlayBase + peer.String())
	if err != nil {
		t.Fatal(err)
	}

	bzzAddr, err := bzz.NewAddress(signer, multiaddr, peer, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(peer, *bzzAddr); err != nil {
		t.Fatal(err)
	}
	err = k.Connected(context.Background(), p2p.Peer{Address: peer}, false)

	if !errors.Is(err, expErr) {
		t.Fatalf("expected error %v , got %v", expErr, err)
	}
}

func addOne(t *testing.T, signer beeCrypto.Signer, k *kademlia.Kad, ab addressbook.Putter, peer swarm.Address) {
	t.Helper()
	multiaddr, err := ma.NewMultiaddr(underlayBase + peer.String())
	if err != nil {
		t.Fatal(err)
	}
	bzzAddr, err := bzz.NewAddress(signer, multiaddr, peer, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(peer, *bzzAddr); err != nil {
		t.Fatal(err)
	}
	k.AddPeers(peer)
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
	waitCounter(t, conns, 1)
}

// waits for counter for some time. resets the pointer value
// if the correct number  have been reached.
func waitCounter(t *testing.T, conns *int32, exp int32) {
	t.Helper()
	var got int32
	if exp == 0 {
		// sleep for some time before checking for a 0.
		// this gives some time for unwanted counter increments happen

		time.Sleep(50 * time.Millisecond)
	}
	for i := 0; i < 50; i++ {
		if got = atomic.LoadInt32(conns); got == exp {
			atomic.StoreInt32(conns, 0)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for counter to reach expected value. got %d want %d", got, exp)
}

func waitPeers(t *testing.T, k *kademlia.Kad, peers int) {
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for peers")
		default:
		}
		i := 0
		_ = k.EachPeer(func(_ swarm.Address, _ uint8) (bool, bool, error) {
			i++
			return false, false, nil
		})
		if i == peers {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// wait for discovery BroadcastPeers to happen
func waitBcast(t *testing.T, d *mock.Discovery, pivot swarm.Address, addrs ...swarm.Address) {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
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

// waitBalanced waits for kademlia to be balanced for specified bin.
func waitBalanced(t *testing.T, k *kademlia.Kad, bin uint8) {
	t.Helper()

	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting to be balanced for bin: %d", int(bin))
		default:
		}

		if balanced := k.IsBalanced(bin); balanced {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}
}
