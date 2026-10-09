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

// emptyBinReserve is a reserve with an empty bin. Each subscription waits for
// chunks that never arrive, like a live sync of a bin without new chunks.
type emptyBinReserve struct {
	*mockstorer.ReserveStore

	mu           sync.Mutex
	subscribed   map[uint64]chan struct{}
	unsubscribed atomic.Int64
	want         int64
	released     chan struct{} // closed when want subscriptions have ended
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

// subscription returns a channel that is closed when the server subscribes to the bin from start.
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

// TestPullsyncAbandonedStreams checks over a real libp2p connection that pullsync
// streams which the client abandons while the server waits on an empty bin do not
// use up the inbound stream limit of the server. A stream reset does not cancel the
// handler context, so the server must see the reset in some other way.
//
// The test is not parallel: it opens thousands of streams, which would slow down
// the parallel connection tests that use short timeouts.
func TestPullsyncAbandonedStreams(t *testing.T) {
	ctx := t.Context()

	server, serverOverlay := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{FullNode: true}})
	client, _ := newService(t, 1, libp2pServiceOpts{libp2pOpts: libp2p.Options{FullNode: true}})

	// abandon as many requests as the inbound stream limit allows
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

	// each request is canceled after the server starts to wait for chunks, so the client resets the stream
	var (
		wg   sync.WaitGroup
		sem  = make(chan struct{}, concurrency)
		errC = make(chan error, 1)
	)
	for i := range requests {
		start := uint64(i + 1) // a different interval for each request, so that the requests do not share a wait
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()

			reqCtx, cancel := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() {
				_, _, err := clientSyncer.Sync(reqCtx, serverOverlay, 0, start)
				done <- err
			}()

			select {
			case <-reserve.subscription(start):
			case err := <-done:
				cancel()
				select {
				case errC <- err:
				default:
				}
				return
			}
			cancel()
			<-done
		})
	}
	wg.Wait()
	select {
	case err := <-errC:
		t.Fatalf("sync request ended before it was abandoned: %v", err)
	default:
	}

	// the server must stop waiting for all abandoned requests
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
