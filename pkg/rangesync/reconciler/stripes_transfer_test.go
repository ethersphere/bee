// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// stripeNode is a rangesync service over a mock reserve.
type stripeNode struct {
	addr  swarm.Address
	store *mockstorer.ReserveStore
	rec   *streamtest.RecorderDisconnecter
	svc   *rangesync.Service
}

func newStripeNode(t *testing.T, rec *streamtest.Recorder, a swarm.Address, chunks ...swarm.Chunk) *stripeNode {
	t.Helper()
	n := &stripeNode{
		addr:  a,
		store: mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithChunks(chunks...)),
		rec:   streamtest.NewRecorderDisconnecter(rec),
	}
	accept := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	n.svc = rangesync.New(n.rec, n.store, n.addr, nil, nil, accept, log.Noop, rangesync.Options{LeafBits: 8})
	t.Cleanup(func() {
		if err := n.svc.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := n.svc.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "index", func() bool {
		_, err := n.svc.LocalStatus()
		return err == nil
	})
	return n
}

// stripedChunks returns valid chunks, perStripe in each of the 1<<bits
// top-level stripes of the address space.
func stripedChunks(bits uint8, perStripe int) []swarm.Chunk {
	counts := make([]int, 1<<bits)
	var out []swarm.Chunk
	for len(out) < perStripe<<bits {
		ch := chunktesting.GenerateTestRandomChunk()
		i := ch.Address().Bytes()[0] >> (8 - bits)
		if counts[i] < perStripe {
			counts[i]++
			out = append(out, ch)
		}
	}
	return out
}

func chunkIDOf(t *testing.T, ch swarm.Chunk) storer.ChunkID {
	t.Helper()
	sh, err := ch.Stamp().Hash()
	if err != nil {
		t.Fatal(err)
	}
	return storer.ChunkID{Address: ch.Address(), StampHash: sh}
}

// streamRecords returns the records of n's streams named name to peer.
func streamRecords(t *testing.T, n *stripeNode, peer swarm.Address, name string) []*streamtest.Record {
	t.Helper()
	recs, err := n.rec.Records(peer, rangesync.ProtocolName, rangesync.ProtocolVersion, name)
	if err != nil && !errors.Is(err, streamtest.ErrRecordsNotFound) {
		t.Fatal(err)
	}
	return recs
}

func TestTransferStripesEmptyNode(t *testing.T) {
	t.Parallel()

	const (
		k         = 4
		perStripe = 12 // streamtest deadlocks past 16 deliveries per Want
	)
	chunks := stripedChunks(2, perStripe)

	// the empty node routes each peer's streams to that peer's service
	emptyAddr := swarm.RandAddress(t)
	routes := make(map[string]p2p.ProtocolSpec)
	peers := make([]*stripeNode, k)
	for i := range peers {
		a := swarm.RandAddress(t)
		peers[i] = newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(a)), a, chunks...)
		routes[peers[i].addr.String()] = peers[i].svc.Protocol()
	}
	empty := newStripeNode(t, streamtest.New(streamtest.WithBaseAddr(emptyAddr), streamtest.WithPeerProtocols(routes)), emptyAddr)
	for _, p := range peers {
		p.rec.SetProtocols(empty.svc.Protocol())
		if err := empty.svc.SendStatus(context.Background(), p.addr); err != nil {
			t.Fatal(err)
		}
		if err := p.svc.SendStatus(context.Background(), empty.addr); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "status", func() bool {
			_, a := empty.svc.LastStatus(p.addr)
			_, b := p.svc.LastStatus(empty.addr)
			return a && b
		})
	}

	own, err := empty.svc.LocalStatus()
	if err != nil {
		t.Fatal(err)
	}
	statuses := make([]reconciler.PeerStatus, 0, len(peers))
	for _, p := range peers {
		st, _ := empty.svc.LastStatus(p.addr)
		statuses = append(statuses, reconciler.PeerStatus{Peer: p.addr, Status: st})
	}
	if !reconciler.IsBootstrap(own, statuses) {
		t.Fatal("empty node not bootstrapping")
	}
	stripes := reconciler.PlanStripes(own.Root, statuses)
	if len(stripes) != k {
		t.Fatalf("got %d stripes, want %d", len(stripes), k)
	}

	r := reconciler.New(empty.addr, nil, empty.svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	results := r.TransferStripes(context.Background(), stripes)
	listed, stored := 0, 0
	for _, res := range results {
		if res.Err != nil {
			t.Fatalf("stripe %s: %v", res.Range, res.Err)
		}
		if res.Listed != perStripe || res.Stored != perStripe {
			t.Fatalf("stripe %s: listed %d, stored %d, want %d", res.Range, res.Listed, res.Stored, perStripe)
		}
		listed += res.Listed
		stored += res.Stored
	}
	for _, ch := range chunks {
		id := chunkIDOf(t, ch)
		if has, err := empty.store.ReserveHasID(id.Address, id.StampHash); err != nil || !has {
			t.Fatalf("chunk %s not stored (%v)", ch.Address(), err)
		}
	}

	// every chunk delivered once, and every ID listed once: the metadata
	// received is about one ID per chunk, not one per chunk and peer
	// (§7.4: ~270 MB rather than ~3.4 GB at 2^22 chunks and 8 neighbors)
	const idBytes = 2 * swarm.HashSize
	deliveries, metadata := 0, 0
	for _, p := range peers {
		for _, rec := range streamRecords(t, empty, p.addr, rangesync.FetchStreamName) {
			ms, err := protobuf.ReadMessages(bytes.NewReader(rec.Out()), func() protobuf.Message { return new(pb.Delivery) })
			if err != nil {
				t.Fatal(err)
			}
			deliveries += len(ms)
		}
		for _, rec := range streamRecords(t, empty, p.addr, rangesync.ReconcileStreamName) {
			metadata += len(rec.Out())
		}
	}
	if deliveries != len(chunks) {
		t.Fatalf("%d deliveries of %d chunks", deliveries, len(chunks))
	}
	if metadata < len(chunks)*idBytes || metadata > 2*len(chunks)*idBytes {
		t.Fatalf("received %d bytes of metadata for %d IDs of %d bytes", metadata, len(chunks), idBytes)
	}
	if got := r.StripesTotal("done"); got != k {
		t.Fatalf("stripes done %v, want %d", got, k)
	}

	// a stripe transferred again (a restart) lists and fetches nothing
	for _, res := range r.TransferStripes(context.Background(), stripes) {
		if res.Err != nil || res.Listed != 0 || res.Stored != 0 {
			t.Fatalf("restart of %s: listed %d, stored %d, err %v", res.Range, res.Listed, res.Stored, res.Err)
		}
	}
}

func TestTransferStripesTimeout(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	svc.reconcileGate = make(chan struct{}) // never opens
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{StripeTimeout: 20 * time.Millisecond})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	results := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})
	if !errors.Is(results[0].Err, reconciler.ErrStripeTimeout) {
		t.Fatalf("got %v, want %v", results[0].Err, reconciler.ErrStripeTimeout)
	}
	if got := r.StripesTotal("timeout"); got != 1 {
		t.Fatalf("stripes timeout %v, want 1", got)
	}
	if got := r.StripesTotal("done"); got != 0 {
		t.Fatalf("stripes done %v, want 0", got)
	}
}

func TestTransferStripesNoPeer(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	results := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root}})
	if !errors.Is(results[0].Err, reconciler.ErrNoStripePeer) {
		t.Fatalf("got %v, want %v", results[0].Err, reconciler.ErrNoStripePeer)
	}
	if n := len(svc.sessionOpens(higher)); n != 0 {
		t.Fatalf("%d sessions", n)
	}
}

func TestTransferStripesSessionLimitRetried(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	svc.reconErr[higher.ByteString()] = rangesync.ErrSessionLimit // once
	svc.wants[higher.ByteString()] = 3
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	if res.Err != nil || res.Listed != 3 || res.Stored != 3 {
		t.Fatalf("listed %d, stored %d, err %v", res.Listed, res.Stored, res.Err)
	}
	if n := len(svc.sessionOpens(higher)); n != 2 {
		t.Fatalf("%d sessions, want 2", n)
	}
}

func TestTransferStripesFetchError(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	svc.wants[higher.ByteString()] = 3
	svc.fetchErr = errors.New("fetch broke")
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	if res.Err == nil || !errors.Is(res.Err, svc.fetchErr) {
		t.Fatalf("got %v, want %v", res.Err, svc.fetchErr)
	}
	if got := r.StripesTotal("done"); got != 0 {
		t.Fatalf("stripes done %v, want 0", got)
	}
}

// TestSessionStripesLargeRange checks the Striper wiring: a range a session
// hands to its Striper is planned across the peers and transferred, one
// session per stripe whose opening is the stripe.
func TestSessionStripesLargeRange(t *testing.T) {
	t.Parallel()

	other := addr(0xf1, 0)
	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	svc := newFakeService(status(1))
	svc.remote[higher.ByteString()] = status(2)
	svc.remote[other.ByteString()] = status(2)
	svc.offer[higher.ByteString()] = root
	newHarness(t, svc, higher, other)

	waitFor(t, "stripe sessions", func() bool {
		got := make(map[string]bool)
		for _, p := range []swarm.Address{higher, other} {
			for _, open := range svc.sessionOpens(p) {
				if len(open) == 1 {
					got[open[0].String()] = true
				}
			}
		}
		return len(got) == 2 && got[root.Child(1, 0).String()] && got[root.Child(1, 1).String()]
	})
}

// TestTransferStripesBackpressure fetches slower than the session lists:
// more batches than the queue holds wait in Wants and the stripe completes.
func TestTransferStripesBackpressure(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	svc.wants[higher.ByteString()] = 12
	svc.wantBatches = 12 // above stripeQueue
	svc.fetchDelay = 10 * time.Millisecond
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})
	t.Cleanup(func() { _ = r.Close() })

	root := fingerprint.NewRange(swarm.ZeroAddress, 0)
	res := r.TransferStripes(context.Background(), []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	if res.Err != nil || res.Listed != 12 || res.Stored != 12 {
		t.Fatalf("listed %d, stored %d, err %v", res.Listed, res.Stored, res.Err)
	}
	if got := r.StripesTotal("done"); got != 1 {
		t.Fatalf("stripes done %v, want 1", got)
	}
}

// TestTransferStripesShutdown cancels a transfer whose fetch never returns:
// TransferStripes and Close return, and the dropped batches are not listed.
func TestTransferStripesShutdown(t *testing.T) {
	t.Parallel()

	svc := newFakeService(status(0))
	svc.wants[higher.ByteString()] = 12
	svc.wantBatches = 12
	svc.fetchGate = make(chan struct{}) // never opens
	r := reconciler.New(self, nil, svc, log.Noop, reconciler.Options{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan reconciler.StripeResult)
	go func() {
		root := fingerprint.NewRange(swarm.ZeroAddress, 0)
		done <- r.TransferStripes(ctx, []reconciler.Stripe{{Range: root, Peers: []swarm.Address{higher}}})[0]
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case res := <-done:
		if res.Err == nil || res.Stored != 0 {
			t.Fatalf("stored %d, err %v", res.Stored, res.Err)
		}
		// the first batch is fetched, stripeQueue more are queued, the rest dropped
		if res.Listed >= 12 {
			t.Fatalf("listed %d dropped batches", res.Listed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TransferStripes did not return")
	}
	closed := make(chan error)
	go func() { closed <- r.Close() }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
	if got := r.StripesTotal("done"); got != 0 {
		t.Fatalf("stripes done %v, want 0", got)
	}
}
