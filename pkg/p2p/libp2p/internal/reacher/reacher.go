// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reacher runs a background worker that will ping peers
// from an internal queue and report back the reachability to the notifier.
package reacher

import (
	"container/heap"
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	pingTimeout               = time.Second * 15
	workers                   = 4
	retryAfterDuration        = time.Minute * 5
	maxFailBackoffExponent    = 4   // caps failure backoff at retryAfterDuration * 2^4 = 80 min
	maxSuccessBackoffExponent = 2   // caps success backoff at retryAfterDuration * 2^2 = 20 min
	jitterFactor              = 0.2 // ±20% randomization on retry intervals
)

type peer struct {
	overlay      swarm.Address
	addr         ma.Multiaddr
	retryAfter   time.Time
	failCount    int // consecutive ping failures for exponential backoff
	successCount int // consecutive ping successes for exponential backoff
	index        int // index in the heap
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

	c := make(chan peer)
	defer close(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.wg.Add(r.options.Workers)
	for i := 0; i < r.options.Workers; i++ {
		go r.ping(c, ctx)
	}

	for {
		p, ok, tryAfter := r.tryAcquirePeer()

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
		if !ok {
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

func (r *reacher) ping(c chan peer, ctx context.Context) {
	defer r.wg.Done()
	for p := range c {
		func() {
			ctxt, cancel := context.WithTimeout(ctx, r.options.PingTimeout)
			defer cancel()
			rtt, err := r.pinger.Ping(ctxt, p.addr)
			if err != nil {
				r.logger.Debug("ping failed", "peer", p.overlay.String(), "addr", p.addr.String(), "error", err)
				r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPrivate)
				r.notifyResult(p.overlay, false)
			} else {
				r.logger.Debug("ping succeeded", "peer", p.overlay.String(), "addr", p.addr.String(), "rtt", rtt)
				r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPublic)
				r.notifyResult(p.overlay, true)
			}
		}()
	}
}

func (r *reacher) tryAcquirePeer() (peer, bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.peerHeap) == 0 {
		return peer{}, false, 0
	}

	now := time.Now()

	// Peek at the peer with the earliest retryAfter
	p := r.peerHeap[0]

	// If retryAfter has not expired, return time to wait
	if now.Before(p.retryAfter) {
		return peer{}, false, time.Until(p.retryAfter)
	}

	// Set a temporary far-future retryAfter to prevent the manage loop from
	// re-dispatching this peer while the ping is in flight. The actual
	// retryAfter will be set by notifyResult after the ping completes.
	p.retryAfter = now.Add(time.Hour)
	heap.Fix(&r.peerHeap, p.index)

	// Return a copy so callers can read fields without holding the lock.
	return *p, true, 0
}

// Connected adds a new peer to the queue for testing reachability.
// If the peer already exists, its address is updated.
func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	if addr == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := overlay.ByteString()
	if existing, ok := r.peerIndex[key]; ok {
		existing.addr = addr              // Update address for reconnecting peer
		existing.retryAfter = time.Time{} // Reset to trigger immediate re-ping
		existing.failCount = 0            // Fresh start on reconnect
		existing.successCount = 0
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

// notifyResult updates the peer's retry schedule based on the ping outcome.
// Both success and failure use exponential backoff with different caps:
//   - Success: 5m → 10m → 20m (capped at 2^2), resets failCount
//   - Failure: 5m → 10m → 20m → 40m → 80m (capped at 2^4), resets successCount
func (r *reacher) notifyResult(overlay swarm.Address, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.peerIndex[overlay.ByteString()]
	if !ok {
		return // peer was disconnected while ping was in flight
	}

	if success {
		p.failCount = 0
		p.successCount++
		backoff := min(p.successCount, maxSuccessBackoffExponent)
		p.retryAfter = time.Now().Add(jitter(r.options.RetryAfterDuration * time.Duration(1<<backoff)))
	} else {
		p.successCount = 0
		p.failCount++
		backoff := min(p.failCount, maxFailBackoffExponent)
		p.retryAfter = time.Now().Add(jitter(r.options.RetryAfterDuration * time.Duration(1<<backoff)))
	}
	heap.Fix(&r.peerHeap, p.index)

	// Wake the manage loop so it recalculates the next retry time.
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

// jitter adds ±20% randomization to a duration to prevent peers from
// synchronizing their retry times and causing burst traffic.
func jitter(d time.Duration) time.Duration {
	// rand.Float64() returns [0.0, 1.0), scale to [-jitterFactor, +jitterFactor)
	j := 1.0 + jitterFactor*(2*rand.Float64()-1)
	return time.Duration(float64(d) * j)
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
