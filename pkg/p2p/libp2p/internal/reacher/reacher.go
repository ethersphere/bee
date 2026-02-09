// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reacher runs a background worker that will ping peers
// from an internal queue and report back the reachability to the notifier.
package reacher

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	pingTimeout        = time.Second * 15
	workers            = 16
	retryAfterDuration = time.Minute * 5
)

type peer struct {
	overlay    swarm.Address
	addr       ma.Multiaddr
	retryAfter time.Time
	index      int // index in the heap
}

// peerHeap is a min-heap of peers ordered by retryAfter time.
type peerHeap []*peer

func (h peerHeap) Len() int           { return len(h) }
func (h peerHeap) Less(i, j int) bool { return h[i].retryAfter.Before(h[j].retryAfter) }
func (h peerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *peerHeap) Push(x any) {
	n := len(*h)
	p := x.(*peer)
	p.index = n
	*h = append(*h, p)
}

func (h *peerHeap) Pop() any {
	old := *h
	n := len(old)
	p := old[n-1]
	old[n-1] = nil // avoid memory leak
	p.index = -1   // for safety
	*h = old[0 : n-1]
	return p
}

type reacher struct {
	mu        sync.Mutex
	peerHeap  peerHeap         // min-heap ordered by retryAfter
	peerIndex map[string]*peer // lookup by overlay for O(1) access

	newPeer chan struct{}
	quit    chan struct{}

	pinger   p2p.Pinger
	notifier p2p.ReachableNotifier

	wg sync.WaitGroup

	options *Options
	logger  log.Logger
}

type Options struct {
	PingTimeout        time.Duration
	Workers            int
	RetryAfterDuration time.Duration
}

func New(streamer p2p.Pinger, notifier p2p.ReachableNotifier, o *Options, log log.Logger) *reacher {
	r := &reacher{
		newPeer:   make(chan struct{}, 1),
		quit:      make(chan struct{}),
		pinger:    streamer,
		peerHeap:  make(peerHeap, 0),
		peerIndex: make(map[string]*peer),
		notifier:  notifier,
		logger:    log.WithName("reacher").Register(),
	}

	if o == nil {
		o = &Options{
			PingTimeout:        pingTimeout,
			Workers:            workers,
			RetryAfterDuration: retryAfterDuration,
		}
	}
	r.options = o

	r.wg.Add(1)
	go r.manage()

	return r
}

func (r *reacher) manage() {

	defer r.wg.Done()

	c := make(chan *peer)
	defer close(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.wg.Add(r.options.Workers)
	for i := 0; i < r.options.Workers; i++ {
		go r.ping(c, ctx)
	}

	for {

		p, tryAfter := r.tryAcquirePeer()

		// if no peer is returned,
		// wait until either more work or the closest retry-after time.

		// wait for work and tryAfter
		if tryAfter > 0 {
			select {
			case <-r.quit:
				return
			case <-r.newPeer:
				continue
			case <-time.After(tryAfter):
				continue
			}
		}

		// wait for work
		if p == nil {
			select {
			case <-r.quit:
				return
			case <-r.newPeer:
				continue
			}
		}

		// ping peer
		select {
		case <-r.quit:
			return
		case c <- p:
		}
	}
}

func (r *reacher) ping(c chan *peer, ctx context.Context) {
	defer r.wg.Done()
	for p := range c {
		func() {
			ctxt, cancel := context.WithTimeout(ctx, r.options.PingTimeout)
			defer cancel()
			rtt, err := r.pinger.Ping(ctxt, p.addr)
			if err != nil {
				r.logger.Debug("ping failed", "peer", p.overlay.String(), "addr", p.addr.String(), "error", err)
				r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPrivate)
			} else {
				r.logger.Debug("ping succeeded", "peer", p.overlay.String(), "addr", p.addr.String(), "rtt", rtt)
				r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPublic)
			}
		}()
	}
}

func (r *reacher) tryAcquirePeer() (*peer, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.peerHeap) == 0 {
		return nil, 0
	}

	now := time.Now()

	// Peek at the peer with the earliest retryAfter
	p := r.peerHeap[0]

	// If retryAfter has not expired, return time to wait
	if now.Before(p.retryAfter) {
		return nil, time.Until(p.retryAfter)
	}

	// Update retryAfter and fix heap position
	p.retryAfter = time.Now().Add(r.options.RetryAfterDuration)
	heap.Fix(&r.peerHeap, p.index)

	return p, 0
}

// Connected adds a new peer to the queue for testing reachability.
// If the peer already exists, its address is updated.
func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := overlay.ByteString()
	if existing, ok := r.peerIndex[key]; ok {
		existing.addr = addr              // Update address for reconnecting peer
		existing.retryAfter = time.Time{} // Reset to trigger immediate re-ping
		heap.Fix(&r.peerHeap, existing.index)
	} else {
		p := &peer{overlay: overlay, addr: addr}
		r.peerIndex[key] = p
		heap.Push(&r.peerHeap, p)
	}

	select {
	case r.newPeer <- struct{}{}:
	default:
	}
}

// Disconnected removes a peer from the queue.
func (r *reacher) Disconnected(overlay swarm.Address) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := overlay.ByteString()
	if p, ok := r.peerIndex[key]; ok {
		heap.Remove(&r.peerHeap, p.index)
		delete(r.peerIndex, key)
	}
}

// Close stops the worker. Must be called once.
func (r *reacher) Close() error {
	select {
	case <-r.quit:
		return nil
	default:
	}

	close(r.quit)
	r.wg.Wait()
	return nil
}
