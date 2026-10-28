// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// emptyBinReserve is a reserve with an empty bin. Each subscription waits for chunks
// that do not arrive, as in a live sync of a bin that gets no new chunks.
type emptyBinReserve struct {
	*mockstorer.ReserveStore

	mu           sync.Mutex
	subscribed   map[uint64]chan struct{}
	unsubscribed atomic.Int64
	want         int64
	released     chan struct{} // closed when want subscriptions end
}

func newEmptyBinReserve(want int64) *emptyBinReserve {
	return &emptyBinReserve{
		ReserveStore: mockstorer.NewReserve(),
		subscribed:   make(map[uint64]chan struct{}),
		want:         want,
		released:     make(chan struct{}),
	}
}

func (r *emptyBinReserve) SubscribeBin(_ context.Context, _ uint8, start uint64) (<-chan *storer.BinC, func(), <-chan error) {
	close(r.subscription(start))
	unsubscribe := func() {
		if r.unsubscribed.Add(1) == r.want {
			close(r.released)
		}
	}
	return make(chan *storer.BinC), unsubscribe, make(chan error)
}

// subscription returns a channel. The channel closes when the server subscribes to the bin from start.
func (r *emptyBinReserve) subscription(start uint64) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.subscribed[start]
	if !ok {
		c = make(chan struct{})
		r.subscribed[start] = c
	}
	return c
}

// TestPullsyncAbandonedStreams uses a real libp2p connection. The client stops pullsync
// requests while the server waits on an empty bin. The test checks that these streams
// do not use all inbound streams of the server. A stream reset does not cancel the handler
// context, so the server must find the reset in a different way.
//
// The test is not parallel. It opens thousands of streams, and these make the parallel
// connection tests slower. Those tests use short timeouts.
func TestPullsyncAbandonedStreams(t *testing.T) {
	ctx := t.Context()

	server, serverOverlay := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{FullNode: true}})
	client, _ := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{FullNode: true}})

	// stop as many requests as the inbound stream limit
	const (
		requests    = libp2p.IncomingStreamCountLimit
		concurrency = 100
	)

	reserve := newEmptyBinReserve(requests)
	validStamp := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	serverSyncer := pullsync.New(server, reserve, func(swarm.Chunk) {}, func(*soc.SOC) {}, validStamp, log.Noop, pullsync.DefaultMaxPage)
	t.Cleanup(func() { _ = serverSyncer.Close() })
	if err := server.AddProtocol(serverSyncer.Protocol()); err != nil {
		t.Fatal(err)
	}
	clientSyncer := pullsync.New(client, mockstorer.NewReserve(), func(swarm.Chunk) {}, func(*soc.SOC) {}, validStamp, log.Noop, pullsync.DefaultMaxPage)
	t.Cleanup(func() { _ = clientSyncer.Close() })

	if _, err := client.Connect(ctx, serviceUnderlayAddress(t, server)); err != nil {
		t.Fatal(err)
	}

	// cancel each request after the server starts to wait for chunks. Then the client resets the stream.
	var (
		wg   sync.WaitGroup
		sem  = make(chan struct{}, concurrency)
		errC = make(chan error, 1)
	)
	for i := range requests {
		start := uint64(i + 1) // each request uses a different interval, so the requests do not share a wait
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()

			reqCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			var syncErr error
			go func() {
				defer close(done)
				_, _, syncErr = clientSyncer.Sync(reqCtx, serverOverlay, 0, start)
			}()

			select {
			case <-reserve.subscription(start):
			case <-done:
				cancel()
				select {
				case errC <- syncErr:
				default:
				}
				return
			}
			cancel()
			<-done // wait for the abandoned request to end; its error is the cancellation
		})
	}
	wg.Wait()
	select {
	case err := <-errC:
		t.Fatalf("sync request ended before it was abandoned: %v", err)
	default:
	}

	// the server must stop the wait for all stopped requests
	select {
	case <-reserve.released:
	case <-time.After(10 * time.Second):
		t.Errorf("server still waits for %d of %d abandoned requests", requests-reserve.unsubscribed.Load(), requests)
	}

	// the server must accept a new stream
	spec := serverSyncer.Protocol()
	stream, err := client.NewStream(ctx, serverOverlay, nil, spec.Name, spec.Version, spec.StreamSpecs[0].Name)
	if err != nil {
		t.Fatalf("server refused a new stream after %d abandoned requests: %v", requests, err)
	}
	_ = stream.Reset()
}
