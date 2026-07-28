// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	statestoremock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kadmock "github.com/ethersphere/bee/v2/pkg/topology/kademlia/mock"
)

// acceptAllStamp is the accept-all ValidStampFn used by every node: the
// simulator does not verify postage cryptography.
func acceptAllStamp() postage.ValidStampFn {
	return func(chunk swarm.Chunk) (swarm.Chunk, error) { return chunk, nil }
}

// Node bundles one synthetic node's real protocol components with its in-memory
// shims.
type Node struct {
	Index     int
	Addr      swarm.Address
	Reserve   *SimReserve
	Transport *Transport
	Syncer    *pullsync.Syncer
	Kad       *simKad
	Puller    *puller.Puller
}

// newNode builds the reserve, transport, and Syncer for a node. Handler
// wiring, kademlia peers, and the puller are set up by the Network once every
// node's Syncer exists.
func newNode(
	index int,
	addr swarm.Address,
	bins, radius uint8,
	epoch uint64,
	latency time.Duration,
	maxPage uint64,
	logger log.Logger,
	putHook func(PutEvent),
	hooks TransportHooks,
) *Node {
	reserve := NewSimReserve(addr, bins, radius, epoch, putHook)
	transport := NewTransport(addr, latency, hooks)
	syncer := pullsync.New(
		transport,
		reserve,
		func(swarm.Chunk) {}, // unwrap: no-op
		func(*soc.SOC) {},    // gsoc handler: no-op
		acceptAllStamp(),
		logger,
		maxPage,
	)
	return &Node{
		Index:     index,
		Addr:      addr,
		Reserve:   reserve,
		Transport: transport,
		Syncer:    syncer,
	}
}

// attachPuller wires the kademlia mock, statestore, instrumented syncer, and
// puller. It must be called after every node's Syncer exists and handlers are
// registered.
func (nd *Node) attachPuller(
	bins uint8,
	logger log.Logger,
	peers []kadmock.AddrTuple,
	onSync func(SyncEvent),
) {
	nd.Kad = newSimKad(peers)

	state := statestoremock.NewStateStore()
	wrapped := newSyncWrap(nd.Syncer, onSync)

	nd.Puller = puller.New(
		nd.Addr,
		state,
		nd.Kad,
		nd.Reserve,
		wrapped,
		nil, // blockLister: never called by the puller
		logger,
		puller.Options{Bins: bins},
	)
}
