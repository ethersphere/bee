// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	beeCrypto "github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/discovery/mock"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/v2/pkg/p2p/mock"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	mockstate "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/kademlia"
	im "github.com/ethersphere/bee/v2/pkg/topology/kademlia/internal/metrics"
	"github.com/ethersphere/bee/v2/pkg/topology/pslice"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

const spinLockWaitTime = time.Second * 5

var nonConnectableAddress, _ = ma.NewMultiaddr(underlayBase + "16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
var defaultExcludeFunc kademlia.ExcludeFunc = func(...im.ExcludeOp) kademlia.PeerExcludeFunc {
	return func(swarm.Address) bool { return false }
}

// TestNeighborhoodDepth tests that the kademlia depth changes correctly
// according to the change to known peers slice. This inadvertently tests
// the functionality in `manage()` method, however this is not the main aim of the
// test, since depth calculation happens there and in the disconnect method.
// A more in depth testing of the functionality in `manage()` is explicitly
// tested in TestManage below.
func TestNeighborhoodDepth(t *testing.T) {
	t.Parallel()

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			SaturationPeers: ptrInt(4),
			ExcludeFunc:     defaultExcludeFunc,
		})
	)
	kad.SetStorageRadius(0)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// add 2 peers in bin 8
	for i := 0; i < 2; i++ {
		addr := swarm.RandAddressAt(t, base, 8)
		addOne(t, signer, kad, ab, addr)

		// wait for one connection
		waitConn(t, &conns)
	}
	// depth is 0
	kDepth(t, kad, 0)

	var shallowPeers []swarm.Address
	// add two first peers (po0,po1)
	for i := 0; i < 2; i++ {
		addr := swarm.RandAddressAt(t, base, i)
		addOne(t, signer, kad, ab, addr)
		shallowPeers = append(shallowPeers, addr)

		// wait for one connection
		waitConn(t, &conns)
	}

	for _, a := range shallowPeers {
		if !kad.IsWithinConnectionDepth(a) {
			t.Fatal("expected address to be within depth")
		}
	}

	kad.SetStorageRadius(0)

	// depth 0 - bin 0 is unsaturated
	kDepth(t, kad, 0)

	for i := 2; i < 8; i++ {
		addr := swarm.RandAddressAt(t, base, i)
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
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, signer, kad, ab, addr)
			waitConn(t, &conns)
		}
		kDepth(t, kad, i+1)
	}

	// depth is 7 because bin 7 is unsaturated (1 peer)
	kDepth(t, kad, 7)

	// set the radius to be lower than unsaturated, expect radius as depth
	kad.SetStorageRadius(6)
	kRadius(t, kad, 6)

	kad.SetStorageRadius(0)

	// expect shallow peers not in depth
	for _, a := range shallowPeers {
		if kad.IsWithinConnectionDepth(a) {
			t.Fatal("expected address to outside of depth")
		}
	}

	// now add another ONE peer at depth, and expect the depth to still
	// stay 8, because the counter for nnLowWatermark would be reached only at the next
	// depth iteration when calculating depth
	addr := swarm.RandAddressAt(t, base, 8)
	addOne(t, signer, kad, ab, addr)
	waitConn(t, &conns)
	kDepth(t, kad, 7)

	// now fill bin 7 so that it is saturated, expect depth 8
	for i := 0; i < 3; i++ {
		addr := swarm.RandAddressAt(t, base, 7)
		addOne(t, signer, kad, ab, addr)
		waitConn(t, &conns)
	}
	kDepth(t, kad, 8)

	// saturate bin 8
	addr = swarm.RandAddressAt(t, base, 8)
	addOne(t, signer, kad, ab, addr)
	waitConn(t, &conns)
	kDepth(t, kad, 8)

	var addrs []swarm.Address
	// fill the rest up to the bin before last and check that everything works at the edges
	for i := 9; i < int(swarm.MaxBins); i++ {
		for j := 0; j < 4; j++ {
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, signer, kad, ab, addr)
			waitConn(t, &conns)
			addrs = append(addrs, addr)
		}
		kDepth(t, kad, i)
	}

	// add a whole bunch of peers in the last bin, expect depth to stay at 31
	for i := 0; i < 15; i++ {
		addr = swarm.RandAddressAt(t, base, int(swarm.MaxPO))
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

	if !kad.IsWithinConnectionDepth(addrs[0]) {
		t.Fatal("expected address to be within depth")
	}

}

// Run the same test with reachability filter and setting the peers are reachable
func TestNeighborhoodDepthWithReachability(t *testing.T) {
	t.Parallel()

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			SaturationPeers: ptrInt(4),
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	kad.SetStorageRadius(0)

	// add 2 peers in bin 8
	for i := 0; i < 2; i++ {
		addr := swarm.RandAddressAt(t, base, 8)
		addOne(t, signer, kad, ab, addr)
		kad.Reachable(addr, p2p.ReachabilityStatusPublic)

		// wait for one connection
		waitConn(t, &conns)
	}
	// depth is 0
	kDepth(t, kad, 0)

	var shallowPeers []swarm.Address
	// add two first peers (po0,po1)
	for i := 0; i < 2; i++ {
		addr := swarm.RandAddressAt(t, base, i)
		addOne(t, signer, kad, ab, addr)
		kad.Reachable(addr, p2p.ReachabilityStatusPublic)
		shallowPeers = append(shallowPeers, addr)

		// wait for one connection
		waitConn(t, &conns)
	}

	for _, a := range shallowPeers {
		if !kad.IsWithinConnectionDepth(a) {
			t.Fatal("expected address to be within depth")
		}
	}

	// depth 0 - bin 0 is unsaturated
	kDepth(t, kad, 0)

	for i := 2; i < 8; i++ {
		addr := swarm.RandAddressAt(t, base, i)
		addOne(t, signer, kad, ab, addr)
		kad.Reachable(addr, p2p.ReachabilityStatusPublic)

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
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, signer, kad, ab, addr)
			kad.Reachable(addr, p2p.ReachabilityStatusPublic)
			waitConn(t, &conns)
		}
		kDepth(t, kad, i+1)
	}

	// depth is 7 because bin 7 is unsaturated (1 peer)
	kDepth(t, kad, 7)

	// expect shallow peers not in depth
	for _, a := range shallowPeers {
		if kad.IsWithinConnectionDepth(a) {
			t.Fatal("expected address to outside of depth")
		}
	}

	// now add another ONE peer at depth, and expect the depth to still
	// stay 8, because the counter for nnLowWatermark would be reached only at the next
	// depth iteration when calculating depth
	addr := swarm.RandAddressAt(t, base, 8)
	addOne(t, signer, kad, ab, addr)
	kad.Reachable(addr, p2p.ReachabilityStatusPublic)
	waitConn(t, &conns)
	kDepth(t, kad, 7)

	// now fill bin 7 so that it is saturated, expect depth 8
	for i := 0; i < 3; i++ {
		addr := swarm.RandAddressAt(t, base, 7)
		addOne(t, signer, kad, ab, addr)
		kad.Reachable(addr, p2p.ReachabilityStatusPublic)
		waitConn(t, &conns)
	}
	kDepth(t, kad, 8)

	// saturate bin 8
	addr = swarm.RandAddressAt(t, base, 8)
	addOne(t, signer, kad, ab, addr)
	kad.Reachable(addr, p2p.ReachabilityStatusPublic)
	waitConn(t, &conns)
	kDepth(t, kad, 8)

	var addrs []swarm.Address
	// fill the rest up to the bin before last and check that everything works at the edges
	for i := 9; i < int(swarm.MaxBins); i++ {
		for j := 0; j < 4; j++ {
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, signer, kad, ab, addr)
			kad.Reachable(addr, p2p.ReachabilityStatusPublic)
			waitConn(t, &conns)
			addrs = append(addrs, addr)
		}
		kDepth(t, kad, i)
	}

	// add a whole bunch of peers in the last bin, expect depth to stay at 31
	for i := 0; i < 15; i++ {
		addr = swarm.RandAddressAt(t, base, int(swarm.MaxPO))
		addOne(t, signer, kad, ab, addr)
		kad.Reachable(addr, p2p.ReachabilityStatusPublic)
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

	if !kad.IsWithinConnectionDepth(addrs[0]) {
		t.Fatal("expected address to be within depth")
	}
}

func TestManage(t *testing.T) {
	t.Parallel()

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		saturation               = kademlia.DefaultSaturationPeers
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			BitSuffixLength: ptrInt(-1),
			ExcludeFunc:     defaultExcludeFunc,
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	kad.SetStorageRadius(0)

	// first, we add peers to bin 0
	for i := 0; i < saturation; i++ {
		addr := swarm.RandAddressAt(t, base, 0)
		addOne(t, signer, kad, ab, addr)
	}

	waitCounter(t, &conns, int32(saturation))

	// next, we add peers to the next bin
	for i := 0; i < saturation; i++ {
		addr := swarm.RandAddressAt(t, base, 1)
		addOne(t, signer, kad, ab, addr)
	}

	waitCounter(t, &conns, int32(saturation))

	kad.SetStorageRadius(1)

	// here, we attempt to add to bin 0, but bin is saturated, so no new peers should connect to it
	for i := 0; i < saturation; i++ {
		addr := swarm.RandAddressAt(t, base, 0)
		addOne(t, signer, kad, ab, addr)
	}

	waitCounter(t, &conns, 0)
}

func TestManageWithBalancing(t *testing.T) {
	t.Parallel()

	var (
		conns int32 // how many connect calls were made to the p2p mock

		saturationFuncImpl *func(bin uint8, connected *pslice.PSlice, _ kademlia.PeerExcludeFunc) bool
		saturationFunc     = func(bin uint8, connected *pslice.PSlice, filter kademlia.PeerExcludeFunc) bool {
			f := *saturationFuncImpl
			return f(bin, connected, filter)
		}
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			SaturationFunc:  saturationFunc,
			SaturationPeers: ptrInt(4),
			BitSuffixLength: ptrInt(2),
			ExcludeFunc:     defaultExcludeFunc,
		})
	)

	// implement saturation function (while having access to Kademlia instance)
	sfImpl := func(bin uint8, connected *pslice.PSlice, _ kademlia.PeerExcludeFunc) bool {
		return false
	}
	saturationFuncImpl = &sfImpl

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// add peers for bin '0', enough to have balanced connections
	add(t, signer, kad, ab, mineBin(t, base, 0, 20, true), 0, 20)

	waitBalanced(t, kad, 0)

	// add peers for other bins, enough to have balanced connections
	for i := 1; i <= int(swarm.MaxPO); i++ {
		add(t, signer, kad, ab, mineBin(t, base, i, 20, true), 0, 20)
		// sanity check depth
		kDepth(t, kad, i)
	}

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
	t.Parallel()

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			SaturationPeers: ptrInt(2),
			BitSuffixLength: ptrInt(-1),
			ExcludeFunc:     defaultExcludeFunc,
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	kad.SetStorageRadius(0)

	// add two peers in a few bins to generate some depth >= 0, this will
	// make the next iteration result in binSaturated==true, causing no new
	// connections to be made
	for i := 0; i < 5; i++ {
		for j := 0; j < 2; j++ {
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, signer, kad, ab, addr)
		}
	}
	waitCounter(t, &conns, 10)

	kad.SetStorageRadius(3)

	// add one more peer in each bin shallower than depth and
	// expect no connections due to saturation. if we add a peer within
	// depth, the short circuit will be hit and we will connect to the peer
	for i := 0; i < 3; i++ {
		addr := swarm.RandAddressAt(t, base, i)
		addOne(t, signer, kad, ab, addr)
	}
	waitCounter(t, &conns, 0)

	// add one peer in a bin higher (unsaturated) and expect one connection
	addr := swarm.RandAddressAt(t, base, 6)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

	// again, one bin higher
	addr = swarm.RandAddressAt(t, base, 7)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

}

func TestOversaturation(t *testing.T) {
	t.Parallel()

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			ExcludeFunc: defaultExcludeFunc,
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// Add maximum accepted number of peers up until bin 5 without problems
	for i := 0; i < 6; i++ {
		for j := 0; j < kademlia.DefaultOverSaturationPeers; j++ {
			addr := swarm.RandAddressAt(t, base, i)
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
			addr := swarm.RandAddressAt(t, base, k)
			// if error is not as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, topology.ErrOversaturated)
			// check that pick works correctly
			if kad.Pick(p2p.Peer{Address: addr, FullNode: true}) {
				t.Fatal("should not pick the peer")
			}
			if !kad.Pick(p2p.Peer{Address: addr, FullNode: false}) {
				t.Fatal("should pick the peer")
			}
		}
		// see depth is still as expected
		kDepth(t, kad, 5)
	}
}

func TestOversaturationBootnode(t *testing.T) {
	t.Parallel()

	var (
		overSaturationPeers      = 4
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			OverSaturationPeers: ptrInt(overSaturationPeers),
			SaturationPeers:     ptrInt(4),
			BootnodeMode:        true,
			ExcludeFunc:         defaultExcludeFunc,
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// Add maximum accepted number of peers up until bin 5 without problems
	for i := 0; i < 6; i++ {
		for j := 0; j < overSaturationPeers; j++ {
			addr := swarm.RandAddressAt(t, base, i)
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
			addr := swarm.RandAddressAt(t, base, k)
			// if error is not as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
			// check that pick works correctly
			if !kad.Pick(p2p.Peer{Address: addr}) {
				t.Fatal("should pick the peer but didn't")
			}
		}
		// see depth is still as expected
		kDepth(t, kad, 5)
	}

	// see we can still add / not limiting more peers in neighborhood depth
	for m := 0; m < 12; m++ {
		addr := swarm.RandAddressAt(t, base, 5)
		// if error is not nil as specified, connectOne goes fatal
		connectOne(t, signer, kad, ab, addr, nil)
		// see depth is still as expected
		kDepth(t, kad, 5)
	}
}

func TestBootnodeMaxConnections(t *testing.T) {
	t.Parallel()

	var (
		bootnodeOverSaturationPeers = 4
		conns                       int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer    = newTestKademlia(t, &conns, nil, kademlia.Options{
			BootnodeOverSaturationPeers: ptrInt(bootnodeOverSaturationPeers),
			SaturationPeers:             ptrInt(4),
			BootnodeMode:                true,
			ExcludeFunc:                 defaultExcludeFunc,
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// Add maximum accepted number of peers up until bin 5 without problems
	for i := 0; i < 6; i++ {
		for j := 0; j < bootnodeOverSaturationPeers; j++ {
			addr := swarm.RandAddressAt(t, base, i)
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
			addr := swarm.RandAddressAt(t, base, k)
			// if error is not as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
			// check that pick works correctly
			if !kad.Pick(p2p.Peer{Address: addr}) {
				t.Fatal("should pick the peer but didn't")
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
	t.Parallel()
	t.Skip("disabled due to kademlia inconsistencies hotfix")

	var (
		base, kad, ab, _, signer = newTestKademlia(t, nil, nil, kademlia.Options{})
		peer                     = swarm.RandAddressAt(t, base, 3)
		addr                     = swarm.RandAddressAt(t, peer, 4) // address which is closer to peer
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	connectOne(t, signer, kad, ab, peer, nil)

	p, err := kad.ClosestPeer(addr, true, topology.Select{})
	if err != nil {
		t.Fatal(err)
	}

	if !p.Equal(peer) {
		t.Fatal("got wrong peer address")
	}

	// disconnect the peer, expect error
	kad.Disconnected(p2p.Peer{Address: peer})
	_, err = kad.ClosestPeer(addr, true, topology.Select{})
	if !errors.Is(err, topology.ErrNotFound) {
		t.Fatalf("expected topology.ErrNotFound but got %v", err)
	}
}

// TestDiscoveryHooks check that a peer is gossiped to other peers
// once we establish a connection to this peer. This could be as a result of
// us proactively dialing in to a peer, or when a peer dials in.
func TestDiscoveryHooks(t *testing.T) {
	t.Parallel()

	var (
		conns                    int32
		_, kad, ab, disc, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			ExcludeFunc: defaultExcludeFunc,
		})
		p1, p2, p3 = swarm.RandAddress(t), swarm.RandAddress(t), swarm.RandAddress(t)
	)

	kad.SetStorageRadius(0)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

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
	t.Parallel()

	var (
		conns                    int32
		_, kad, ab, disc, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
		p1, p2                   = swarm.RandAddress(t), swarm.RandAddress(t)
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

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
	t.Parallel()

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{
			TimeToRetry: ptrDuration(500 * time.Millisecond),
		})
	)
	kad.SetStorageRadius(0)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// add one peer, wait for connection
	addr := swarm.RandAddressAt(t, base, 1)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

	// remove that peer
	removeOne(kad, addr)

	waitCounter(t, &conns, 0)

	// wait for 100ms, add another peer, expect just one more connection
	time.Sleep(100 * time.Millisecond)
	addr = swarm.RandAddressAt(t, base, 1)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 1)

	// wait for another 400ms, add another, expect 2 connections
	time.Sleep(400 * time.Millisecond)
	addr = swarm.RandAddressAt(t, base, 1)
	addOne(t, signer, kad, ab, addr)

	waitCounter(t, &conns, 2)
}

// test pruning addressbook after successive failed connect attempts
func TestAddressBookPrune(t *testing.T) {
	t.Parallel()

	var (
		conns, failedConns       int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, &failedConns, kademlia.Options{
			TimeToRetry: ptrDuration(0),
		})
	)

	kad.SetStorageRadius(0)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	nonConnPeer, err := bzz.NewAddress(signer, nonConnectableAddress, swarm.RandAddressAt(t, base, 1), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(nonConnPeer.Overlay, *nonConnPeer); err != nil {
		t.Fatal(err)
	}

	// add non connectable peer, check connection and failed connection counters
	kad.AddPeers(nonConnPeer.Overlay)

	kad.Trigger()
	kad.Trigger()

	waitCounter(t, &conns, 0)
	waitCounter(t, &failedConns, 3)

	_, err = ab.Get(nonConnPeer.Overlay)
	if !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatal(err)
	}

	addr := swarm.RandAddressAt(t, base, 1)
	addr1 := swarm.RandAddressAt(t, base, 1)
	addr2 := swarm.RandAddressAt(t, base, 1)

	// add one valid peer to initiate the retry, check connection and failed connection counters
	addOne(t, signer, kad, ab, addr)
	waitCounter(t, &conns, 1)
	waitCounter(t, &failedConns, 0)

	_, err = ab.Get(nonConnPeer.Overlay)
	if !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	// add one valid peer to initiate the retry, check connection and failed connection counters
	addOne(t, signer, kad, ab, addr1)
	waitCounter(t, &conns, 1)
	waitCounter(t, &failedConns, 0)

	_, err = ab.Get(nonConnPeer.Overlay)
	if !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	// add one valid peer to initiate the retry, check connection and failed connection counters
	addOne(t, signer, kad, ab, addr2)
	waitCounter(t, &conns, 1)
	waitCounter(t, &failedConns, 0)

	_, err = ab.Get(nonConnPeer.Overlay)
	if !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatal(err)
	}
}

// test pruning addressbook after successive failed connect attempts
func TestAddressBookQuickPrune_FLAKY(t *testing.T) {
	t.Parallel()

	var (
		conns, failedConns       int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, &failedConns, kademlia.Options{
			TimeToRetry: ptrDuration(time.Millisecond),
		})
	)
	kad.SetStorageRadius(2)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	time.Sleep(100 * time.Millisecond)

	nonConnPeer, err := bzz.NewAddress(signer, nonConnectableAddress, swarm.RandAddressAt(t, base, 1), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ab.Put(nonConnPeer.Overlay, *nonConnPeer); err != nil {
		t.Fatal(err)
	}

	addr := swarm.RandAddressAt(t, base, 1)
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

func TestClosestPeer(t *testing.T) {
	t.Parallel()
	t.Skip("disabled due to kademlia inconsistencies hotfix")

	_ = waitPeers

	logger := log.Noop
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

	kad, err := kademlia.New(base, ab, disc, p2pMock(t, ab, nil, nil, nil), logger, kademlia.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

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
		peer, err := kad.ClosestPeer(tc.chunkAddress, tc.includeSelf, topology.Select{})
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

func TestKademlia_SubscribeTopologyChange(t *testing.T) {
	t.Parallel()

	testSignal := func(t *testing.T, c <-chan struct{}) {
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
		t.Parallel()

		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, kad)

		c, u := kad.SubscribeTopologyChange()
		defer u()

		addr := swarm.RandAddressAt(t, base, 9)
		addOne(t, sg, kad, ab, addr)

		testSignal(t, c)
	})

	t.Run("single subscription, remove peer", func(t *testing.T) {
		t.Parallel()

		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, kad)

		c, u := kad.SubscribeTopologyChange()
		defer u()

		addr := swarm.RandAddressAt(t, base, 9)
		addOne(t, sg, kad, ab, addr)

		testSignal(t, c)

		removeOne(kad, addr)
		testSignal(t, c)
	})

	t.Run("multiple subscriptions", func(t *testing.T) {
		t.Parallel()

		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, kad)

		c1, u1 := kad.SubscribeTopologyChange()
		defer u1()

		c2, u2 := kad.SubscribeTopologyChange()
		defer u2()

		for i := 0; i < 4; i++ {
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, sg, kad, ab, addr)
		}
		testSignal(t, c1)
		testSignal(t, c2)
	})

	t.Run("multiple changes", func(t *testing.T) {
		t.Parallel()

		base, kad, ab, _, sg := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, kad)

		c, u := kad.SubscribeTopologyChange()
		defer u()

		for i := 0; i < 4; i++ {
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, sg, kad, ab, addr)
		}

		testSignal(t, c)

		for i := 0; i < 4; i++ {
			addr := swarm.RandAddressAt(t, base, i)
			addOne(t, sg, kad, ab, addr)
		}

		testSignal(t, c)
	})

	t.Run("no depth change", func(t *testing.T) {
		t.Parallel()

		_, kad, _, _, _ := newTestKademlia(t, nil, nil, kademlia.Options{})
		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, kad)

		c, u := kad.SubscribeTopologyChange()
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

func TestSnapshot_FLAKY(t *testing.T) {
	t.Parallel()

	var conns = new(int32)
	sa, kad, ab, _, signer := newTestKademlia(t, conns, nil, kademlia.Options{})
	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	a := swarm.RandAddress(t)
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
	t.Parallel()

	var bootnodes []ma.Multiaddr
	var bootnodesOverlays []swarm.Address
	for i := 0; i < 10; i++ {
		overlay := swarm.RandAddress(t)

		multiaddr, err := ma.NewMultiaddr(underlayBase + overlay.String())
		if err != nil {
			t.Fatal(err)
		}

		bootnodes = append(bootnodes, multiaddr)
		bootnodesOverlays = append(bootnodesOverlays, overlay)
	}

	t.Run("non-empty addressbook", func(t *testing.T) {
		t.Parallel()
		t.Skip("test flakes")

		var conns, failedConns int32 // how many connect calls were made to the p2p mock
		_, kad, ab, _, signer := newTestKademlia(t, &conns, &failedConns, kademlia.Options{Bootnodes: bootnodes})

		for i := 0; i < 3; i++ {
			peer := swarm.RandAddress(t)
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
		testutil.CleanupCloser(t, kad)

		waitCounter(t, &conns, 3)
		waitCounter(t, &failedConns, 0)
	})

	t.Run("empty addressbook", func(t *testing.T) {
		t.Parallel()

		var conns, failedConns int32 // how many connect calls were made to the p2p mock
		_, kad, _, _, _ := newTestKademlia(t, &conns, &failedConns, kademlia.Options{Bootnodes: bootnodes})

		if err := kad.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		testutil.CleanupCloser(t, kad)

		waitCounter(t, &conns, 3)
		waitCounter(t, &failedConns, 0)

		err := kad.EachConnectedPeer(func(addr swarm.Address, bin uint8) (stop bool, jumpToNext bool, err error) {
			for _, b := range bootnodesOverlays {
				if b.Equal(addr) {
					return false, false, errors.New("did not expect bootnode address from the iterator")
				}
			}
			return false, false, nil

		}, topology.Select{})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestOutofDepthPrune(t *testing.T) {
	t.Parallel()

	var (
		conns, failedConns int32 // how many connect calls were made to the p2p mock

		saturationPeers     = 4
		overSaturationPeers = 16
		pruneWakeup         = time.Millisecond * 100
		pruneFuncImpl       *func(uint8)
		pruneMux            = sync.Mutex{}
		pruneFunc           = func(depth uint8) {
			pruneMux.Lock()
			defer pruneMux.Unlock()
			f := *pruneFuncImpl
			f(depth)
		}

		base, kad, ab, _, signer = newTestKademlia(t, &conns, &failedConns, kademlia.Options{
			SaturationPeers:     ptrInt(saturationPeers),
			OverSaturationPeers: ptrInt(overSaturationPeers),
			PruneFunc:           pruneFunc,
			ExcludeFunc:         defaultExcludeFunc,
			PruneWakeup:         &pruneWakeup,
		})
	)

	kad.SetStorageRadius(0)

	// implement empty prune func
	pruneMux.Lock()
	pruneImpl := func(uint8) {}
	pruneFuncImpl = &(pruneImpl)
	pruneMux.Unlock()

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// bin 0,1 balanced, rest not
	for i := 0; i < 6; i++ {
		var peers []swarm.Address
		if i < 2 {
			peers = mineBin(t, base, i, 20, true)
		} else {
			peers = mineBin(t, base, i, 20, false)
		}
		for _, peer := range peers {
			addOne(t, signer, kad, ab, peer)
		}
		time.Sleep(time.Millisecond * 10)
		kDepth(t, kad, i)
	}

	// check that bin 0, 1 are balanced, but not 2
	waitBalanced(t, kad, 0)
	waitBalanced(t, kad, 1)
	if kad.IsBalanced(2) {
		t.Fatal("bin 2 should not be balanced")
	}

	// wait for kademlia connectors and pruning to finish
	time.Sleep(time.Millisecond * 500)

	// check that no pruning has happened
	bins := binSizes(kad)
	for i := 0; i < 6; i++ {
		if bins[i] <= overSaturationPeers {
			t.Fatalf("bin %d, got %d, want more than %d", i, bins[i], overSaturationPeers)
		}
	}

	kad.SetStorageRadius(6)

	// set prune func to the default
	pruneMux.Lock()
	pruneImpl = func(depth uint8) {
		kademlia.PruneOversaturatedBinsFunc(kad)(depth)
	}
	pruneFuncImpl = &(pruneImpl)
	pruneMux.Unlock()

	// add a peer to kick start pruning
	addr := swarm.RandAddressAt(t, base, 6)
	addOne(t, signer, kad, ab, addr)

	// wait for kademlia connectors and pruning to finish
	time.Sleep(time.Millisecond * 500)

	// check bins have been pruned
	bins = binSizes(kad)
	for i := uint8(0); i < 5; i++ {
		if bins[i] != overSaturationPeers {
			t.Fatalf("bin %d, got %d, want %d", i, bins[i], overSaturationPeers)
		}
	}

	// check that bin 0,1 remains balanced after pruning
	waitBalanced(t, kad, 0)
	waitBalanced(t, kad, 1)
}

// TestPruneExcludeOps tests that the prune bin func counts peers in each bin correctly using the peer's reachability status and does not over prune.
func TestPruneExcludeOps(t *testing.T) {
	t.Parallel()

	var (
		conns, failedConns int32 // how many connect calls were made to the p2p mock

		saturationPeers     = 4
		overSaturationPeers = 16
		perBin              = 20
		pruneFuncImpl       *func(uint8)
		pruneMux            = sync.Mutex{}
		pruneFunc           = func(depth uint8) {
			pruneMux.Lock()
			defer pruneMux.Unlock()
			f := *pruneFuncImpl
			f(depth)
		}

		base, kad, ab, _, signer = newTestKademlia(t, &conns, &failedConns, kademlia.Options{
			SaturationPeers:     ptrInt(saturationPeers),
			OverSaturationPeers: ptrInt(overSaturationPeers),
			PruneFunc:           pruneFunc,
		})
	)

	kad.SetStorageRadius(0)

	// implement empty prune func
	pruneMux.Lock()
	pruneImpl := func(uint8) {}
	pruneFuncImpl = &(pruneImpl)
	pruneMux.Unlock()

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// bin 0,1 balanced, rest not
	for i := 0; i < 6; i++ {
		var peers []swarm.Address
		if i < 2 {
			peers = mineBin(t, base, i, perBin, true)
		} else {
			peers = mineBin(t, base, i, perBin, false)
		}
		for i, peer := range peers {
			addOne(t, signer, kad, ab, peer)
			if i < 4 {
				kad.Reachable(peers[i], p2p.ReachabilityStatusPrivate)
			} else {
				kad.Reachable(peers[i], p2p.ReachabilityStatusPublic)
			}
		}
		for i := 0; i < 4; i++ {
		}
		time.Sleep(time.Millisecond * 10)
		kDepth(t, kad, i)
	}

	// check that bin 0, 1 are balanced, but not 2
	waitBalanced(t, kad, 0)
	waitBalanced(t, kad, 1)
	if kad.IsBalanced(2) {
		t.Fatal("bin 2 should not be balanced")
	}

	// wait for kademlia connectors and pruning to finish
	time.Sleep(time.Millisecond * 500)

	// check that no pruning has happened
	bins := binSizes(kad)
	for i := 0; i < 6; i++ {
		if bins[i] <= overSaturationPeers {
			t.Fatalf("bin %d, got %d, want more than %d", i, bins[i], overSaturationPeers)
		}
	}

	kad.SetStorageRadius(6)

	// set prune func to the default
	pruneMux.Lock()
	pruneImpl = func(depth uint8) {
		kademlia.PruneOversaturatedBinsFunc(kad)(depth)
	}
	pruneFuncImpl = &(pruneImpl)
	pruneMux.Unlock()

	// add a peer to kick start pruning
	addr := swarm.RandAddressAt(t, base, 6)
	addOne(t, signer, kad, ab, addr)

	// wait for kademlia connectors and pruning to finish
	time.Sleep(time.Millisecond * 500)

	// check bins have NOT been pruned because the peer count func excluded unreachable peers
	bins = binSizes(kad)
	for i := uint8(0); i < 5; i++ {
		if bins[i] != perBin {
			t.Fatalf("bin %d, got %d, want %d", i, bins[i], perBin)
		}
	}

	// check that bin 0,1 remains balanced after pruning
	waitBalanced(t, kad, 0)
	waitBalanced(t, kad, 1)
}

func TestBootnodeProtectedNodes(t *testing.T) {
	t.Parallel()

	// create base and protected nodes addresses
	base := swarm.RandAddress(t)
	protected := make([]swarm.Address, 6)
	for i := 0; i < 6; i++ {
		addr := swarm.RandAddressAt(t, base, i)
		protected[i] = addr
	}

	var (
		conns                 int32 // how many connect calls were made to the p2p mock
		overSaturationPeers   = 1
		_, kad, ab, _, signer = newTestKademliaWithAddr(t, base, &conns, nil, kademlia.Options{
			BootnodeOverSaturationPeers: ptrInt(1),
			OverSaturationPeers:         ptrInt(overSaturationPeers),
			SaturationPeers:             ptrInt(1),
			LowWaterMark:                ptrInt(0),
			BootnodeMode:                true,
			StaticNodes:                 protected,
			ExcludeFunc:                 defaultExcludeFunc,
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// Add maximum accepted number of peers up until bin 5 without problems
	for i := 0; i < 6; i++ {
		for j := 0; j < overSaturationPeers; j++ {
			// if error is not nil as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, protected[i], nil)
		}
		// see depth is limited to currently added peers proximity
		kDepth(t, kad, i)
	}

	// see depth is 5
	kDepth(t, kad, 5)

	for k := 0; k < 5; k++ {
		// further connections should succeed outside of depth
		addr := swarm.RandAddressAt(t, base, k)
		// if error is not as specified, connectOne goes fatal
		connectOne(t, signer, kad, ab, addr, nil)
		// see depth is still as expected
		kDepth(t, kad, 5)
	}
	// ensure protected node was not kicked out and we have more than oversaturation
	// amount
	sizes := binSizes(kad)
	for k := 0; k < 5; k++ {
		if sizes[k] != 2 {
			t.Fatalf("invalid bin size expected 2 found %d", sizes[k])
		}
	}
	for k := 0; k < 5; k++ {
		// further connections should succeed outside of depth
		for l := 0; l < 3; l++ {
			addr := swarm.RandAddressAt(t, base, k)
			// if error is not as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
		}
		// see depth is still as expected
		kDepth(t, kad, 5)
	}
	// ensure unprotected nodes are kicked out to make room for new peers and protected
	// nodes are still present
	sizes = binSizes(kad)
	for k := 0; k < 5; k++ {
		if sizes[k] != 2 {
			t.Fatalf("invalid bin size expected 2 found %d", sizes[k])
		}
	}
	for _, pn := range protected {
		found := false
		_ = kad.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
			if addr.Equal(pn) {
				found = true
				return true, false, nil
			}
			return false, false, nil
		}, topology.Select{})
		if !found {
			t.Fatalf("protected node %s not found in connected list", pn)
		}
	}
}

func TestAnnounceBgBroadcast_FLAKY(t *testing.T) {
	t.Parallel()

	var (
		conns  int32
		bgDone = make(chan struct{})
		p1, p2 = swarm.RandAddress(t), swarm.RandAddress(t)
		disc   = mock.NewDiscovery(
			mock.WithBroadcastPeers(func(ctx context.Context, p swarm.Address, _ ...swarm.Address) error {
				// For the broadcast back to connected peer return early
				if p.Equal(p2) {
					return nil
				}
				defer close(bgDone)
				<-ctx.Done()
				return ctx.Err()
			}),
		)
		_, kad, ab, _, signer = newTestKademliaWithDiscovery(t, disc, &conns, nil, kademlia.Options{
			ExcludeFunc: defaultExcludeFunc,
		})
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	// first add a peer from AddPeers, wait for the connection
	addOne(t, signer, kad, ab, p1)
	waitConn(t, &conns)

	// Create a context to cancel and call Announce manually. On the cancellation of
	// this context ensure that the BroadcastPeers call in the background is unaffected
	ctx, cancel := context.WithCancel(context.Background())

	if err := kad.Announce(ctx, p2, true); err != nil {
		t.Fatal(err)
	}

	// cancellation should not close background broadcast
	cancel()

	select {
	case <-bgDone:
		t.Fatal("background broadcast exited")
	case <-time.After(time.Millisecond * 100):
	}

	// All background broadcasts will be cancelled on Close. Ensure that the BroadcastPeers
	// call gets the context cancellation
	if err := kad.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-bgDone:
	case <-time.After(time.Millisecond * 100):
		t.Fatal("background broadcast did not exit on close")
	}
}

func TestAnnounceNeighborhoodToNeighbor(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})

	mtx := sync.Mutex{}

	var neighborAddr swarm.Address
	const broadCastSize = 18

	var (
		conns int32
		disc  = mock.NewDiscovery(
			mock.WithBroadcastPeers(func(ctx context.Context, p swarm.Address, addrs ...swarm.Address) error {
				mtx.Lock()
				defer mtx.Unlock()
				if p.Equal(neighborAddr) {
					if len(addrs) == broadCastSize {
						close(done)
					} else {
						t.Fatal("broadcasted size did not match neighborhood size", "got", len(addrs), "want", broadCastSize)
					}
				}
				return nil
			}),
		)
		base, kad, ab, _, signer = newTestKademliaWithDiscovery(t, disc, &conns, nil, kademlia.Options{
			ExcludeFunc:         defaultExcludeFunc,
			OverSaturationPeers: ptrInt(4),
			SaturationPeers:     ptrInt(4),
		})
	)

	kad.SetStorageRadius(0)

	neighborAddr = swarm.RandAddressAt(t, base, 2)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	// add some peers
	for bin := 0; bin < 2; bin++ {
		for i := 0; i < 4; i++ {
			addr := swarm.RandAddressAt(t, base, bin)
			addOne(t, signer, kad, ab, addr)
			waitCounter(t, &conns, 1)
		}
	}

	waitPeers(t, kad, 8)
	kDepth(t, kad, 1)

	// add many more neighbors
	for i := 0; i < 10; i++ {
		addr := swarm.RandAddressAt(t, base, 2)
		addOne(t, signer, kad, ab, addr)
		waitCounter(t, &conns, 1)
	}

	waitPeers(t, kad, broadCastSize)
	kDepth(t, kad, 2)

	// add one neighbor to track how many peers are broadcasted to it
	addOne(t, signer, kad, ab, neighborAddr)

	waitCounter(t, &conns, 1)
	waitPeers(t, kad, broadCastSize+1)
	kDepth(t, kad, 2)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast did not fire for broadcastTo peer")
	}
}

func TestIteratorOpts(t *testing.T) {
	t.Parallel()

	var (
		conns                    int32 // how many connect calls were made to the p2p mock
		base, kad, ab, _, signer = newTestKademlia(t, &conns, nil, kademlia.Options{})
		randBool                 = &boolgen{}
	)

	if err := kad.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, kad)

	for i := 0; i < 6; i++ {
		for j := 0; j < 4; j++ {
			addr := swarm.RandAddressAt(t, base, i)
			// if error is not nil as specified, connectOne goes fatal
			connectOne(t, signer, kad, ab, addr, nil)
		}
	}

	// randomly mark some nodes as reachable
	totalReachable := 0
	totalHealthy := 0
	reachable := make(map[string]struct{})
	healthy := make(map[string]struct{})

	_ = kad.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
		if randBool.Bool() {
			kad.Reachable(addr, p2p.ReachabilityStatusPublic)
			reachable[addr.ByteString()] = struct{}{}
			totalReachable++
		}
		if randBool.Bool() {
			healthy[addr.ByteString()] = struct{}{}
			totalHealthy++
			kad.UpdatePeerHealth(addr, true, 0)
		}
		return false, false, nil
	}, topology.Select{})

	t.Run("EachConnectedPeer reachable", func(t *testing.T) {
		t.Parallel()

		count := 0
		err := kad.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
			if _, exists := reachable[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			count++
			return false, false, nil
		}, topology.Select{Reachable: true})
		if err != nil {
			t.Fatal("iterator returned error")
		}
		if count != totalReachable {
			t.Fatal("iterator returned incorrect no of peers", count, "expected", totalReachable)
		}
	})

	t.Run("EachConnectedPeer healthy", func(t *testing.T) {
		t.Parallel()

		count := 0
		err := kad.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
			if _, exists := healthy[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			count++
			return false, false, nil
		}, topology.Select{Healthy: true})
		if err != nil {
			t.Fatal("iterator returned error")
		}
		if count != totalHealthy {
			t.Fatal("iterator returned incorrect no of peers", count, "expected", totalHealthy)
		}
	})

	t.Run("EachConnectedPeer reachable healthy", func(t *testing.T) {
		t.Parallel()
		err := kad.EachConnectedPeer(func(addr swarm.Address, _ uint8) (bool, bool, error) {
			if _, exists := reachable[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			if _, exists := healthy[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			return false, false, nil
		}, topology.Select{Reachable: true, Healthy: true})
		if err != nil {
			t.Fatal("iterator returned error")
		}
	})

	t.Run("EachConnectedPeerRev reachable", func(t *testing.T) {
		t.Parallel()

		count := 0
		err := kad.EachConnectedPeerRev(func(addr swarm.Address, _ uint8) (bool, bool, error) {
			if _, exists := reachable[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			count++
			return false, false, nil
		}, topology.Select{Reachable: true})
		if err != nil {
			t.Fatal("iterator returned error")
		}
		if count != totalReachable {
			t.Fatal("iterator returned incorrect no of peers", count, "expected", totalReachable)
		}
	})

	t.Run("EachConnectedPeerRev healthy", func(t *testing.T) {
		t.Parallel()

		count := 0
		err := kad.EachConnectedPeerRev(func(addr swarm.Address, _ uint8) (bool, bool, error) {
			if _, exists := healthy[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			count++
			return false, false, nil
		}, topology.Select{Healthy: true})
		if err != nil {
			t.Fatal("iterator returned error")
		}
		if count != totalHealthy {
			t.Fatal("iterator returned incorrect no of peers", count, "expected", totalHealthy)
		}
	})

	t.Run("EachConnectedPeerRev reachable healthy", func(t *testing.T) {
		t.Parallel()
		err := kad.EachConnectedPeerRev(func(addr swarm.Address, _ uint8) (bool, bool, error) {
			if _, exists := reachable[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			if _, exists := healthy[addr.ByteString()]; !exists {
				t.Fatal("iterator returned incorrect peer")
			}
			return false, false, nil
		}, topology.Select{Reachable: true, Healthy: true})
		if err != nil {
			t.Fatal("iterator returned error")
		}
	})
}

type boolgen struct {
	cache     int64
	remaining int
}

func (b *boolgen) Bool() bool {
	if b.remaining == 0 {
		b.cache, b.remaining = rand.Int63(), 63
	}

	result := b.cache&0x01 == 1
	b.cache >>= 1
	b.remaining--

	return result
}

func mineBin(t *testing.T, base swarm.Address, bin, count int, isBalanced bool) []swarm.Address {
	t.Helper()

	var rndAddrs = make([]swarm.Address, count)

	if count < 8 && isBalanced {
		t.Fatal("peersCount must be greater than 8 for balanced bins")
	}

	for i := 0; i < count; i++ {
		rndAddrs[i] = swarm.RandAddressAt(t, base, bin)
	}

	if isBalanced {
		prefixes := kademlia.GenerateCommonBinPrefixes(base, kademlia.DefaultBitSuffixLength)
		for i := 0; i < int(math.Pow(2, float64(kademlia.DefaultBitSuffixLength))); i++ {
			rndAddrs[i] = prefixes[bin][i]
		}
	} else {
		for i := range rndAddrs {
			bytes := rndAddrs[i].Bytes()
			setBits(bytes, bin+1, 3, 0) // set 3 bits to '000' to mess with balance
			rndAddrs[i] = swarm.NewAddress(bytes)
		}
	}

	return rndAddrs
}

func setBits(data []byte, startBit, bitCount int, b int) {

	index := startBit / 8
	bitPos := startBit % 8

	for bitCount > 0 {

		bitValue := b >> (bitCount - 1)

		if bitValue == 1 {
			data[index] = data[index] | (1 << (7 - bitPos)) // set bit to 1
		} else {
			data[index] = data[index] & (^(1 << (7 - bitPos))) // set bit to 0
		}

		bitCount--
		bitPos++

		if bitPos == 8 {
			bitPos = 0
			index++
		}
	}
}

func binSizes(kad *kademlia.Kad) []int {
	bins := make([]int, swarm.MaxBins)

	_ = kad.EachConnectedPeer(func(a swarm.Address, u uint8) (stop bool, jumpToNext bool, err error) {
		bins[u]++
		return false, false, nil
	}, topology.Select{})

	return bins
}

func newTestKademliaWithAddrDiscovery(
	t *testing.T,
	base swarm.Address,
	disc *mock.Discovery,
	connCounter, failedConnCounter *int32,
	kadOpts kademlia.Options,
) (swarm.Address, *kademlia.Kad, addressbook.Interface, *mock.Discovery, beeCrypto.Signer) {
	t.Helper()

	var (
		pk, _  = beeCrypto.GenerateSecp256k1Key()                       // random private key
		signer = beeCrypto.NewDefaultSigner(pk)                         // signer
		ssMock = mockstate.NewStateStore()                              // state store
		ab     = addressbook.New(ssMock)                                // address book
		p2p    = p2pMock(t, ab, signer, connCounter, failedConnCounter) // p2p mock
		logger = log.Noop                                               // logger
	)
	kad, err := kademlia.New(base, ab, disc, p2p, logger, kadOpts)
	if err != nil {
		t.Fatal(err)
	}

	p2p.SetPickyNotifier(kad)

	return base, kad, ab, disc, signer
}

func newTestKademlia(t *testing.T, connCounter, failedConnCounter *int32, kadOpts kademlia.Options) (swarm.Address, *kademlia.Kad, addressbook.Interface, *mock.Discovery, beeCrypto.Signer) {
	t.Helper()

	base := swarm.RandAddress(t)
	disc := mock.NewDiscovery() // mock discovery protocols
	return newTestKademliaWithAddrDiscovery(t, base, disc, connCounter, failedConnCounter, kadOpts)
}

func newTestKademliaWithAddr(t *testing.T, base swarm.Address, connCounter, failedConnCounter *int32, kadOpts kademlia.Options) (swarm.Address, *kademlia.Kad, addressbook.Interface, *mock.Discovery, beeCrypto.Signer) {
	t.Helper()

	disc := mock.NewDiscovery() // mock discovery protocol
	return newTestKademliaWithAddrDiscovery(t, base, disc, connCounter, failedConnCounter, kadOpts)
}

func newTestKademliaWithDiscovery(
	t *testing.T,
	disc *mock.Discovery,
	connCounter, failedConnCounter *int32,
	kadOpts kademlia.Options,
) (swarm.Address, *kademlia.Kad, addressbook.Interface, *mock.Discovery, beeCrypto.Signer) {
	t.Helper()

	base := swarm.RandAddress(t)
	return newTestKademliaWithAddrDiscovery(t, base, disc, connCounter, failedConnCounter, kadOpts)
}

func p2pMock(t *testing.T, ab addressbook.Interface, signer beeCrypto.Signer, counter, failedCounter *int32) p2p.Service {
	t.Helper()

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

			address := swarm.RandAddress(t)
			bzzAddr, err := bzz.NewAddress(signer, addr, address, 0, nil)
			if err != nil {
				return nil, err
			}

			if err := ab.Put(address, *bzzAddr); err != nil {
				return nil, err
			}

			return bzzAddr, nil
		}),
		p2pmock.WithDisconnectFunc(func(swarm.Address, string) error {
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
	err := spinlock.Wait(spinLockWaitTime, func() bool {
		depth = int(k.ConnectionDepth())
		return depth == d
	})
	if err != nil {
		t.Fatalf("timed out waiting for depth. want %d got %d", d, depth)
	}
}

func kRadius(t *testing.T, k *kademlia.Kad, d int) {
	t.Helper()

	var radius int
	err := spinlock.Wait(spinLockWaitTime, func() bool {
		radius = int(k.StorageRadius())
		return radius == d
	})
	if err != nil {
		t.Fatalf("timed out waiting for radius. want %d got %d", d, radius)
	}
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

	err := spinlock.Wait(spinLockWaitTime, func() bool {
		if got = atomic.LoadInt32(conns); got == exp {
			atomic.StoreInt32(conns, 0)
			return true
		}
		return false
	})
	if err != nil {
		t.Fatalf("timed out waiting for counter to reach expected value. got %d want %d", got, exp)
	}
}

func waitPeers(t *testing.T, k *kademlia.Kad, peers int) {
	t.Helper()

	err := spinlock.Wait(spinLockWaitTime, func() bool {
		i := 0
		_ = k.EachConnectedPeer(func(_ swarm.Address, _ uint8) (bool, bool, error) {
			i++
			return false, false, nil
		}, topology.Select{})
		return i == peers
	})
	if err != nil {
		t.Fatal("timed out waiting for peers")
	}
}

// wait for discovery BroadcastPeers to happen
func waitBcast(t *testing.T, d *mock.Discovery, pivot swarm.Address, addrs ...swarm.Address) {
	t.Helper()

	err := spinlock.Wait(spinLockWaitTime, func() bool {
		if d.Broadcasts() > 0 {
			recs, ok := d.AddresseeRecords(pivot)
			if !ok {
				return false
			}

			oks := 0
			for _, a := range addrs {
				if !swarm.ContainsAddress(recs, a) {
					t.Fatalf("address %s not found in discovery records: %s", a, addrs)
				}
				oks++
			}

			return oks == len(addrs)
		}
		return false
	})
	if err != nil {
		t.Fatalf("timed out waiting for broadcast to happen")
	}
}

// waitBalanced waits for kademlia to be balanced for specified bin.
func waitBalanced(t *testing.T, k *kademlia.Kad, bin uint8) {
	t.Helper()

	err := spinlock.Wait(spinLockWaitTime, func() bool {
		return k.IsBalanced(bin)
	})
	if err != nil {
		t.Fatalf("timed out waiting to be balanced for bin: %d", int(bin))
	}
}

func ptrInt(v int) *int {
	return &v
}

func ptrDuration(v time.Duration) *time.Duration {
	return &v
}
