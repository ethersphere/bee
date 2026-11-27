// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
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
	nodes    byte
	radius   atomic.Uint32 // storage radius of the origin
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
	}
	n.radius.Store(hopRadius)

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
		func() (uint8, error) { return uint8(n.radius.Load()), nil },
		topo, true,
		func(swarm.Chunk) {}, func(*soc.SOC) {},
		func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil },
		log.Noop, accountingmock.NewAccounting(),
		pricermock.NewMockService(defaultPrices.price, defaultPrices.peerPrice),
		nil, nil, stabilmock.NewSubscriber(true),
	)
	t.Cleanup(func() { n.origin.Close() })

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

// setOutOfDepth makes the first hop a forwarder outside of the AOR of the
// chunks that has no closer peer, so that it answers ErrOutOfDepthStoring.
func (n *hopNet) setOutOfDepth(hop swarm.Address) {
	n.t.Helper()

	node, _ := createPushSyncNodeWithRadius(n.t, hop, defaultPrices, nil, nil, hopSigner(n.t, n.anchor, false), hopRadius, mock.WithClosestPeerErr(topology.ErrWantSelf))
	n.handlers[hop.String()] = node.Protocol()
}

// setGone makes every push to the first hop return an error.
func (n *hopNet) setGone(hop swarm.Address) {
	delete(n.handlers, hop.String())
}

// beforeReceipt runs f at the first hop before it handles each push.
func (n *hopNet) beforeReceipt(hop swarm.Address, f func()) {
	spec := n.handlers[hop.String()]
	handler := spec.StreamSpecs[0].Handler
	spec.StreamSpecs = []p2p.StreamSpec{spec.StreamSpecs[0]}
	spec.StreamSpecs[0].Handler = func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
		f()
		return handler(ctx, p, s)
	}
	n.handlers[hop.String()] = spec
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

// hopRecords returns the number of records in the first-hop memory of the node.
func hopRecords(ps *pushsync.PushSync) int {
	_, records := ps.FirstHops().Size()
	return records
}

// TestOriginPrefersKnownValidPeer checks that the next chunk goes to the
// first hop with a valid record, not to a closer one that gave a shallow receipt.
func TestOriginPrefersKnownValidPeer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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
	})
}

// TestOriginPausedPeerLast checks that a first hop in a shallow pause is not
// used while another first hop is left.
func TestOriginPausedPeerLast(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 2)
		shallow, other := n.hops[0], n.hops[1]
		n.setReceipt(shallow, false)
		n.setReceipt(other, true)

		n.pushValid()
		n.wantStreams(shallow, 1)
		n.wantStreams(other, 1)

		// the error removes the valid record of the farther first hop, so
		// that it has no record; only the paused first hop is left
		n.setGone(other)
		n.pushShallow()
		n.wantStreams(shallow, 2)
		n.wantFirstHop("known_valid", "error", 1)
		n.wantFirstHop("paused", "shallow", 1)

		n.setReceipt(other, true)
		time.Sleep(59 * time.Second)
		n.pushValid()
		n.wantStreams(shallow, 2)
		n.wantStreams(other, 2)
		n.wantFirstHop("unknown", "valid", 2)
	})
}

// TestOriginAllPeersPaused checks that the origin sends through the first
// hops in a shallow pause when no other first hop is left, and that these
// sends do not make the pauses longer.
func TestOriginAllPeersPaused(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 2)
		n.setReceipt(n.hops[0], false)
		n.setReceipt(n.hops[1], false)

		n.pushShallow()
		n.wantStreams(n.hops[0], 1)
		n.wantStreams(n.hops[1], 1)

		time.Sleep(30 * time.Second)
		n.pushShallow()
		n.wantStreams(n.hops[0], 2)
		n.wantStreams(n.hops[1], 2)
		n.wantFirstHop("paused", "shallow", 2)

		// the pauses still end 60s after the first shallow receipts
		n.setReceipt(n.hops[1], true)
		time.Sleep(30 * time.Second)
		n.pushValid()
		n.wantStreams(n.hops[0], 3)
		n.wantStreams(n.hops[1], 3)
		n.wantFirstHop("unknown", "shallow", 3)
		n.wantFirstHop("unknown", "valid", 1)
	})
}

// TestOriginAllPausedInsideAOR checks that an origin inside the
// neighborhood stores the chunk itself when every first hop is paused.
func TestOriginAllPausedInsideAOR(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, true, 1)
		n.setReceipt(n.hops[0], false)

		n.pushShallow()
		n.wantStreams(n.hops[0], 1)

		if err := n.push(); !errors.Is(err, topology.ErrWantSelf) {
			t.Fatalf("got %v, want %v", err, topology.ErrWantSelf)
		}
		n.wantStreams(n.hops[0], 1)
	})
}

// TestOriginValidPeerError checks that an error removes a valid record and
// that the chunk gets a valid receipt from another first hop in the same call.
func TestOriginValidPeerError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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
		if got := hopRecords(n.origin); got != 1 {
			t.Fatalf("got %d records, want 1", got)
		}
	})
}

// TestOriginErrorPause checks that a first hop without a valid record that
// gives an error is not picked again for one minute.
func TestOriginErrorPause(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 2)
		gone, valid := n.hops[0], n.hops[1]
		n.setReceipt(valid, true)

		n.pushValid()
		n.wantFirstHop("unknown", "error", 1)

		n.setReceipt(gone, true)
		time.Sleep(59 * time.Second)
		n.pushValid()
		n.wantStreams(gone, 0)

		time.Sleep(time.Second)
		n.pushValid()
		n.wantStreams(gone, 1)
		n.wantFirstHop("probe", "valid", 1)
	})
}

// TestOriginErrorPauseAllPeers checks that the origin sends through a first
// hop that is paused after an error when no other first hop is left.
func TestOriginErrorPauseAllPeers(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 1)
		if err := n.push(); err == nil {
			t.Fatal("got no error")
		}

		n.setReceipt(n.hops[0], true)
		n.pushValid()
		n.wantFirstHop("paused", "valid", 1)
	})
}

// TestOriginOutOfDepthFirstHop checks that a closer first hop outside of the
// AOR that has no closer peer is not probed every probe interval: each error
// doubles its pause.
func TestOriginOutOfDepthFirstHop(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 2)
		outOfDepth, valid := n.hops[0], n.hops[1]
		n.setOutOfDepth(outOfDepth)
		n.setReceipt(valid, true)

		// the first error is at 0s, the probes are at 60s and 180s
		for range 30 {
			n.pushValid()
			time.Sleep(10 * time.Second)
		}
		n.wantStreams(valid, 30)
		n.wantStreams(outOfDepth, 3)
		n.wantFirstHop("unknown", "error", 1)
		n.wantFirstHop("probe", "error", 2)
	})
}

// TestOriginProbeNotStarved checks that a closer first hop that always gives
// an error does not take every probe of the neighborhood: the next first hop
// is probed too, and is used first when it gives a valid receipt.
func TestOriginProbeNotStarved(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 3)
		outOfDepth, behind, known := n.hops[0], n.hops[1], n.hops[2]
		n.setOutOfDepth(outOfDepth)
		n.setReceipt(behind, false)
		n.setReceipt(known, true)

		// the two closer first hops are paused
		n.pushValid()
		n.wantStreams(outOfDepth, 1)
		n.wantStreams(behind, 1)
		n.wantStreams(known, 1)

		// both pauses are over: the probe goes to the closest first hop
		n.setReceipt(behind, true)
		time.Sleep(time.Minute)
		n.pushValid()
		n.wantStreams(outOfDepth, 2)
		n.wantStreams(behind, 1)
		n.wantStreams(known, 2)

		// the next probe goes to the first hop behind it
		time.Sleep(30 * time.Second)
		n.pushValid()
		n.wantStreams(outOfDepth, 2)
		n.wantStreams(behind, 2)
		n.wantStreams(known, 2)
		n.wantFirstHop("probe", "valid", 1)

		n.pushValid()
		n.wantStreams(behind, 3)
		n.wantStreams(known, 2)
	})
}

// TestOriginSkipsDisconnectedValidPeer checks that a disconnected first hop
// with a valid record is not picked and loses the record.
func TestOriginSkipsDisconnectedValidPeer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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
		if got := hopRecords(n.origin); got != 1 {
			t.Fatalf("got %d records, want 1", got)
		}
	})
}

// TestOriginSkipsValidPeerInFlight checks that a push that waits for a first
// hop with a valid record sends to another first hop, not to the same one again.
func TestOriginSkipsValidPeerInFlight(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 2)
		slow, other := n.hops[0], n.hops[1]
		n.setReceipt(slow, true)
		n.setReceipt(other, true)

		n.pushValid()
		n.wantStreams(slow, 1)

		// the answer takes longer than the time to the next attempt
		n.beforeReceipt(slow, func() { time.Sleep(6 * time.Second) })

		n.pushValid()
		n.wantStreams(slow, 2)
		n.wantStreams(other, 1)
	})
}

// TestOriginProbe checks that a closer first hop is probed after its pause,
// that the chunk still gets the valid receipt of the known first hop, and
// that a closer first hop that gives a valid receipt on a probe is used first
// from then on.
func TestOriginProbe(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 2)
		shallow, valid := n.hops[0], n.hops[1]
		n.setReceipt(shallow, false)
		n.setReceipt(valid, true)

		n.pushValid()
		n.wantStreams(shallow, 1)
		n.wantStreams(valid, 1)

		// the pause of the closer first hop is not over: no probe
		time.Sleep(59 * time.Second)
		n.pushValid()
		n.wantStreams(shallow, 1)
		n.wantStreams(valid, 2)

		time.Sleep(time.Second)
		n.pushValid()
		n.wantStreams(shallow, 2)
		n.wantStreams(valid, 3)
		n.wantFirstHop("probe", "shallow", 1)

		// the second shallow receipt in a row doubles the pause
		time.Sleep(119 * time.Second)
		n.pushValid()
		n.wantStreams(shallow, 2)
		n.wantStreams(valid, 4)

		// a valid receipt on a probe: the closer first hop is used first
		// from then on
		n.setReceipt(shallow, true)
		time.Sleep(time.Second)
		n.pushValid()
		n.wantStreams(shallow, 3)
		n.wantStreams(valid, 4)
		n.wantFirstHop("probe", "valid", 1)

		n.pushValid()
		n.wantStreams(shallow, 4)
		n.wantStreams(valid, 4)
	})
}

// TestOriginProbeInterval checks that a neighborhood gets at most one probe
// per 30s.
func TestOriginProbeInterval(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 3)
		n.setReceipt(n.hops[0], false)
		n.setReceipt(n.hops[1], false)
		n.setReceipt(n.hops[2], true)

		n.pushValid()
		n.wantStreams(n.hops[0], 1)
		n.wantStreams(n.hops[1], 1)
		n.wantStreams(n.hops[2], 1)

		// both pauses are over: the first chunk probes the closest first hop
		time.Sleep(time.Minute)
		n.pushValid()
		n.wantStreams(n.hops[0], 2)
		n.wantStreams(n.hops[1], 1)

		// the next chunk comes before the end of the probe interval
		time.Sleep(29 * time.Second)
		n.pushValid()
		n.wantStreams(n.hops[1], 1)

		time.Sleep(time.Second)
		n.pushValid()
		n.wantStreams(n.hops[0], 2)
		n.wantStreams(n.hops[1], 2)
		n.wantFirstHop("probe", "shallow", 2)
		n.wantStreams(n.hops[2], 4)
	})
}

// TestOriginRadiusChangeBeforeNextAttempt checks that an attempt that starts
// without a result of the attempt before it selects its first hop at the
// radius of that moment, so that its result is recorded at the new radius.
func TestOriginRadiusChangeBeforeNextAttempt(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 2)
		slow, other := n.hops[0], n.hops[1]
		n.setReceipt(slow, true)
		n.setReceipt(other, true)

		// the radius of the origin grows while the first attempt is on its
		// way, and the answer takes longer than the time to the next attempt
		n.beforeReceipt(slow, func() {
			n.radius.Store(hopRadius + 1)
			time.Sleep(6 * time.Second)
		})

		ch := testingc.GenerateValidRandomChunkAt(t, n.anchor, hopRadius+1)
		if _, err := n.origin.PushChunkToClosest(context.Background(), ch); err != nil {
			t.Fatalf("got %v, want a valid receipt", err)
		}
		n.wantStreams(slow, 1)
		n.wantStreams(other, 1)

		if got := hopRecords(n.origin); got != 1 {
			t.Fatalf("got %d records, want 1", got)
		}
		view := n.origin.FirstHops().View(hopRadius+1, ch.Address())
		if len(view.Valid) != 1 || !view.Valid[0].Equal(other) || len(view.Paused) != 0 {
			t.Fatalf("got valid %v paused %v, want the valid record of the second first hop at the new radius", view.Valid, view.Paused)
		}
	})
}

// TestOriginRadiusChangeBeforeResult checks that a result is recorded at the
// radius of the moment it arrives: a result of an attempt that began at the
// old radius is not recorded at it, and the next push records at the new radius.
func TestOriginRadiusChangeBeforeResult(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		n := newHopNet(t, false, 1)
		hop := n.hops[0]
		n.setReceipt(hop, true)

		// the radius of the origin grows while the only attempt is on its way
		n.beforeReceipt(hop, func() { n.radius.Store(hopRadius + 1) })

		ch := testingc.GenerateValidRandomChunkAt(t, n.anchor, hopRadius+1)
		if _, err := n.origin.PushChunkToClosest(context.Background(), ch); err != nil {
			t.Fatalf("got %v, want a valid receipt", err)
		}
		n.wantStreams(hop, 1)
		if got := hopRecords(n.origin); got != 0 {
			t.Fatalf("got %d records of a push that began at the old radius, want 0", got)
		}

		if _, err := n.origin.PushChunkToClosest(context.Background(), ch); err != nil {
			t.Fatalf("got %v, want a valid receipt", err)
		}
		n.wantStreams(hop, 2)
		if got := hopRecords(n.origin); got != 1 {
			t.Fatalf("got %d records, want 1", got)
		}
		view := n.origin.FirstHops().View(hopRadius+1, ch.Address())
		if len(view.Valid) != 1 || !view.Valid[0].Equal(hop) || len(view.Paused) != 0 {
			t.Fatalf("got valid %v paused %v, want the valid record of the first hop at the new radius", view.Valid, view.Paused)
		}
	})
}

// TestForwarderDoesNotUseHopMemory checks that a forwarder neither reads
// nor writes first-hop records.
func TestForwarderDoesNotUseHopMemory(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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
		forwarder.FirstHops().View(hopRadius, chunk.Address())
		forwarder.FirstHops().Valid(hopRadius, chunk.Address(), farther)
		forwarder.FirstHops().ClaimProbe(hopRadius, chunk.Address())

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
		if got := hopRecords(forwarder); got != 1 {
			t.Fatalf("forwarder has %d first-hop records, want the 1 that the test made", got)
		}
		if got := hopRecords(n.origin); got != 1 {
			t.Fatalf("origin has %d first-hop records, want 1", got)
		}
	})
}
