// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"context"
	"errors"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/mock"
)

const hopRadius = 2

// hopNet is an origin with first hops whose receipts the test controls.
// The origin knows each first hop by an address outside the neighborhood, so
// the order of the first hops is the same for every chunk. The node behind a
// first hop signs with a key whose overlay is inside the neighborhood (valid
// receipt) or outside of it (shallow receipt).
type hopNet struct {
	t        *testing.T
	anchor   swarm.Address // an address inside the neighborhood of the chunks
	topo     interface{ Disconnected(p2p.Peer) }
	hops     []swarm.Address // first hops as the origin knows them, closest first
	origin   *pushsync.PushSync
	recorder *streamtest.Recorder
	handlers map[string]p2p.ProtocolSpec
	clock    *testClock
	nodes    byte
}

// flipBits returns a copy of the address with the given bits flipped.
func flipBits(a swarm.Address, bits ...int) swarm.Address {
	b := a.Clone().Bytes()
	for _, i := range bits {
		b[i/8] ^= 0x80 >> (i % 8)
	}
	return swarm.NewAddress(b)
}

// hopSigner returns a signer whose overlay is inside (near) or far outside
// the neighborhood of the anchor.
func hopSigner(t *testing.T, anchor swarm.Address, near bool) crypto.Signer {
	t.Helper()

	for {
		key, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(key)
		pubKey, err := signer.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		overlay, err := crypto.NewOverlayAddress(*pubKey, 1, blockHash.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		po := swarm.Proximity(overlay.Bytes(), anchor.Bytes())
		if (near && po > hopRadius) || (!near && po == 0) {
			return signer
		}
	}
}

// newHopNet makes an origin with n first hops; a lower index is closer to
// the chunks. insideAOR puts the origin inside the neighborhood.
func newHopNet(t *testing.T, insideAOR bool, hops int) *hopNet {
	t.Helper()

	n := &hopNet{
		t:        t,
		anchor:   swarm.RandAddress(t),
		handlers: make(map[string]p2p.ProtocolSpec),
		clock:    newTestClock(),
	}

	// all first hops differ from the neighborhood in bit 0; the next bits
	// set their order
	for _, bits := range [][]int{{0}, {0, 2}, {0, 1}}[:hops] {
		n.hops = append(n.hops, flipBits(n.anchor, bits...))
	}

	originAddr := flipBits(n.anchor, 0, 255)
	if insideAOR {
		originAddr = flipBits(n.anchor, 255)
	}

	topo := mock.NewTopologyDriver(mock.WithPeers(n.hops...))
	n.topo = topo

	n.recorder = streamtest.New(streamtest.WithPeerProtocols(n.handlers), streamtest.WithBaseAddr(originAddr))
	n.origin = pushsync.New(
		originAddr, 1, blockHash.Bytes(),
		streamtest.NewRecorderDisconnecter(n.recorder),
		&testStorer{chunksPut: make(map[string]swarm.Chunk), chunksReported: make(map[string]int)},
		func() (uint8, error) { return hopRadius, nil },
		topo, true,
		func(swarm.Chunk) {}, func(*soc.SOC) {},
		func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil },
		log.Noop, accountingmock.NewAccounting(),
		pricermock.NewMockService(defaultPrices.price, defaultPrices.peerPrice),
		nil, nil, stabilmock.NewSubscriber(true),
	)
	t.Cleanup(func() { n.origin.Close() })
	n.origin.SetHopClock(n.clock.Now)

	return n
}

// setReceipt makes the first hop answer with a valid or a shallow receipt.
func (n *hopNet) setReceipt(hop swarm.Address, valid bool) {
	n.t.Helper()

	n.nodes++
	self := n.anchor.Clone().Bytes()
	self[swarm.HashSize-1] ^= n.nodes

	node, _ := createPushSyncNodeWithRadius(n.t, swarm.NewAddress(self), defaultPrices, nil, nil, hopSigner(n.t, n.anchor, valid), hopRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))
	n.handlers[hop.String()] = node.Protocol()
}

// setGone makes every push to the first hop return an error.
func (n *hopNet) setGone(hop swarm.Address) {
	delete(n.handlers, hop.String())
}

func (n *hopNet) chunk() swarm.Chunk {
	n.t.Helper()
	return testingc.GenerateValidRandomChunkAt(n.t, n.anchor, hopRadius)
}

// streams returns the number of pushes the origin sent to the first hop.
func (n *hopNet) streams(hop swarm.Address) int {
	recs, _ := n.recorder.Records(hop, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName)
	return len(recs)
}

func (n *hopNet) wantStreams(hop swarm.Address, want int) {
	n.t.Helper()
	if got := n.streams(hop); got != want {
		n.t.Fatalf("first hop %s: got %d pushes, want %d", hop.String()[:4], got, want)
	}
}

func (n *hopNet) wantFirstHop(source, result string, want float64) {
	n.t.Helper()
	if got := n.origin.FirstHopCount(source, result); got != want {
		n.t.Fatalf("first hop metric %s/%s: got %v, want %v", source, result, got, want)
	}
}

func (n *hopNet) push() error {
	_, err := n.origin.PushChunkToClosest(context.Background(), n.chunk())
	return err
}

func (n *hopNet) pushValid() {
	n.t.Helper()
	if err := n.push(); err != nil {
		n.t.Fatalf("got %v, want a valid receipt", err)
	}
}

func (n *hopNet) pushShallow() {
	n.t.Helper()
	if err := n.push(); !errors.Is(err, pushsync.ErrShallowReceipt) {
		n.t.Fatalf("got %v, want %v", err, pushsync.ErrShallowReceipt)
	}
}

// TestOriginPrefersKnownValidPeer checks that the next chunk goes to the
// first hop with a valid record, not to a closer one that gave a shallow receipt.
func TestOriginPrefersKnownValidPeer(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 2)
	shallow, valid := n.hops[0], n.hops[1]
	n.setReceipt(shallow, false)
	n.setReceipt(valid, true)

	n.pushValid()
	n.wantStreams(shallow, 1)
	n.wantStreams(valid, 1)

	n.pushValid()
	n.wantStreams(shallow, 1)
	n.wantStreams(valid, 2)

	n.wantFirstHop("unknown", "shallow", 1)
	n.wantFirstHop("unknown", "valid", 1)
	n.wantFirstHop("known_valid", "valid", 1)
}

// TestOriginPausedPeerLast checks that a first hop in a shallow pause is not
// used while another first hop is left.
func TestOriginPausedPeerLast(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 2)
	shallow, other := n.hops[0], n.hops[1]
	n.setReceipt(shallow, false)

	// the farther first hop gives an error, so it gets no record
	n.pushShallow()
	n.wantStreams(shallow, 1)
	n.wantFirstHop("unknown", "error", 1)

	n.setReceipt(other, true)
	n.clock.Add(59 * time.Second)
	n.pushValid()
	n.wantStreams(shallow, 1)
	n.wantStreams(other, 1)
	n.wantFirstHop("unknown", "valid", 1)
	n.wantFirstHop("paused", "shallow", 0)
}

// TestOriginOtherNeighborhoodNotAffected checks that a shallow receipt for
// one neighborhood does not leave the first hop out for another one.
func TestOriginOtherNeighborhoodNotAffected(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 1)
	shallow := n.hops[0]
	n.setReceipt(shallow, false)

	n.pushShallow()
	n.wantStreams(shallow, 1)

	// a chunk that differs from the neighborhood in bit 1
	other := testingc.GenerateValidRandomChunkAt(t, flipBits(n.anchor, 1), hopRadius)
	_, _ = n.origin.PushChunkToClosest(context.Background(), other)
	n.wantStreams(shallow, 2)
}

// TestOriginAllPeersPaused checks that the origin sends through the first
// hops in a shallow pause when no other first hop is left, and that these
// sends do not make the pauses longer.
func TestOriginAllPeersPaused(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 2)
	n.setReceipt(n.hops[0], false)
	n.setReceipt(n.hops[1], false)

	n.pushShallow()
	n.wantStreams(n.hops[0], 1)
	n.wantStreams(n.hops[1], 1)

	n.clock.Add(30 * time.Second)
	n.pushShallow()
	n.wantStreams(n.hops[0], 2)
	n.wantStreams(n.hops[1], 2)
	n.wantFirstHop("paused", "shallow", 2)

	// the pauses still end 60s after the first shallow receipts
	n.setReceipt(n.hops[1], true)
	n.clock.Add(30 * time.Second)
	n.pushValid()
	n.wantStreams(n.hops[0], 3)
	n.wantStreams(n.hops[1], 3)
	n.wantFirstHop("unknown", "shallow", 3)
	n.wantFirstHop("unknown", "valid", 1)
}

// TestOriginAllBlockedInsideAOR checks that an origin inside the
// neighborhood stores the chunk itself when every first hop is paused.
func TestOriginAllBlockedInsideAOR(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, true, 1)
	n.setReceipt(n.hops[0], false)

	n.pushShallow()
	n.wantStreams(n.hops[0], 1)

	if err := n.push(); !errors.Is(err, topology.ErrWantSelf) {
		t.Fatalf("got %v, want %v", err, topology.ErrWantSelf)
	}
	n.wantStreams(n.hops[0], 1)
}

// TestOriginValidPeerError checks that an error removes a valid record and
// that the chunk gets a valid receipt from another first hop in the same call.
func TestOriginValidPeerError(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 2)
	first, second := n.hops[0], n.hops[1]
	n.setReceipt(first, true)
	n.setReceipt(second, true)

	n.pushValid()
	n.wantStreams(first, 1)
	n.wantStreams(second, 0)

	n.setGone(first)

	n.pushValid()
	n.wantStreams(second, 1)
	n.wantFirstHop("known_valid", "error", 1)
	n.wantFirstHop("unknown", "valid", 2)
	if got := n.origin.HopRecords(); got != 1 {
		t.Fatalf("got %d records, want 1", got)
	}
}

// TestOriginSkipsDisconnectedValidPeer checks that a disconnected first hop
// with a valid record is not picked and loses the record.
func TestOriginSkipsDisconnectedValidPeer(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 2)
	valid, shallow := n.hops[0], n.hops[1]
	n.setReceipt(valid, true)
	n.setReceipt(shallow, false)

	n.pushValid()
	n.wantStreams(valid, 1)
	n.wantStreams(shallow, 0)

	n.topo.Disconnected(p2p.Peer{Address: valid})

	n.pushShallow()
	n.wantStreams(valid, 1)
	n.wantStreams(shallow, 1)
	if got := n.origin.HopRecords(); got != 1 {
		t.Fatalf("got %d records, want 1", got)
	}
}

// TestOriginProbe checks that a closer first hop is probed after its pause
// and that the chunk still gets the valid receipt of the known first hop.
func TestOriginProbe(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 2)
	shallow, valid := n.hops[0], n.hops[1]
	n.setReceipt(shallow, false)
	n.setReceipt(valid, true)

	n.pushValid()
	n.wantStreams(shallow, 1)
	n.wantStreams(valid, 1)

	// the pause of the closer first hop is not over: no probe
	n.clock.Add(59 * time.Second)
	n.pushValid()
	n.wantStreams(shallow, 1)
	n.wantStreams(valid, 2)

	n.clock.Add(time.Second)
	n.pushValid()
	n.wantStreams(shallow, 2)
	n.wantStreams(valid, 3)
	n.wantFirstHop("probe", "shallow", 1)

	// the second shallow receipt in a row doubles the pause
	n.clock.Add(119 * time.Second)
	n.pushValid()
	n.wantStreams(shallow, 2)
	n.wantStreams(valid, 4)
}

// TestOriginProbeInterval checks that a neighborhood gets at most one probe
// per 30s.
func TestOriginProbeInterval(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 3)
	n.setReceipt(n.hops[0], false)
	n.setReceipt(n.hops[1], false)
	n.setReceipt(n.hops[2], true)

	n.pushValid()
	n.wantStreams(n.hops[0], 1)
	n.wantStreams(n.hops[1], 1)
	n.wantStreams(n.hops[2], 1)

	// both pauses are over: the first chunk probes the closest first hop
	n.clock.Add(time.Minute)
	n.pushValid()
	n.wantStreams(n.hops[0], 2)
	n.wantStreams(n.hops[1], 1)

	// the next chunk comes before the end of the probe interval
	n.clock.Add(29 * time.Second)
	n.pushValid()
	n.wantStreams(n.hops[1], 1)

	n.clock.Add(time.Second)
	n.pushValid()
	n.wantStreams(n.hops[0], 2)
	n.wantStreams(n.hops[1], 2)
	n.wantFirstHop("probe", "shallow", 2)
	n.wantStreams(n.hops[2], 4)
}

// TestOriginProbeBecomesValid checks that a closer first hop that gives a
// valid receipt on a probe is used first from then on.
func TestOriginProbeBecomesValid(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 2)
	closer, valid := n.hops[0], n.hops[1]
	n.setReceipt(closer, false)
	n.setReceipt(valid, true)

	n.pushValid()
	n.wantStreams(closer, 1)
	n.wantStreams(valid, 1)

	n.setReceipt(closer, true)
	n.clock.Add(time.Minute)

	n.pushValid()
	n.wantStreams(closer, 2)
	n.wantStreams(valid, 1)
	n.wantFirstHop("probe", "valid", 1)

	n.pushValid()
	n.wantStreams(closer, 3)
	n.wantStreams(valid, 1)
}

// TestForwarderDoesNotUseHopMemory checks that a forwarder neither reads
// nor writes first-hop records.
func TestForwarderDoesNotUseHopMemory(t *testing.T) {
	t.Parallel()

	n := newHopNet(t, false, 1)
	storer := flipBits(n.anchor, 254)

	storerNode, _ := createPushSyncNodeWithRadius(t, storer, defaultPrices, nil, nil, hopSigner(t, n.anchor, true), hopRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))

	// the forwarder and its two peers are outside the neighborhood; every
	// push of the forwarder gets to the storer
	forwarderAddr := flipBits(n.anchor, 0, 254)
	closer, farther := flipBits(n.anchor, 1), flipBits(n.anchor, 0, 2)
	forwarderRecorder := streamtest.New(streamtest.WithProtocols(storerNode.Protocol()), streamtest.WithBaseAddr(forwarderAddr))
	forwarder, _ := createPushSyncNodeWithRadius(t, forwarderAddr, defaultPrices, forwarderRecorder, nil, nil, hopRadius, mock.WithPeers(closer, farther))
	n.handlers[n.hops[0].String()] = forwarder.Protocol()

	// an origin with these records sends to the farther peer first
	chunk := n.chunk()
	forwarder.Hops().View(hopRadius, chunk.Address())
	forwarder.Hops().RecordValid(hopRadius, chunk.Address(), farther)
	forwarder.Hops().ClaimProbe(hopRadius, chunk.Address())

	if _, err := n.origin.PushChunkToClosest(context.Background(), chunk); err != nil {
		t.Fatalf("got %v, want a valid receipt", err)
	}

	if got := n.streams(n.hops[0]); got != 1 {
		t.Fatalf("got %d pushes to the forwarder, want 1", got)
	}
	if recs, _ := forwarderRecorder.Records(closer, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName); len(recs) != 1 {
		t.Fatalf("got %d pushes to the closest peer, want 1", len(recs))
	}
	if recs, _ := forwarderRecorder.Records(farther, pushsync.ProtocolName, pushsync.ProtocolVersion, pushsync.StreamName); len(recs) != 0 {
		t.Fatalf("got %d pushes to the peer with a valid record, want 0", len(recs))
	}
	if got := forwarder.HopRecords(); got != 1 {
		t.Fatalf("forwarder has %d first-hop records, want the 1 that the test made", got)
	}
	if got := n.origin.HopRecords(); got != 1 {
		t.Fatalf("origin has %d first-hop records, want 1", got)
	}
}
