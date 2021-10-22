// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Reacher package runs a background worker that will ping peers
// from an internal queue and report back the reachability to some notifier.
package reacher

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	pingTimeout     = time.Second * 5
	pingMaxAttempts = 3
	workers         = 8
)

var retryAfterDuration = time.Second * 15

type peerState int

const (
	waiting peerState = iota
	inProgress
	cleanup
)

type peer struct {
	overlay    swarm.Address
	addr       ma.Multiaddr
	retryAfter time.Time
	attempts   int
	state      peerState
}

type reacher struct {
	mu    sync.Mutex
	peers map[string]*peer

	ctx       context.Context
	ctxCancel context.CancelFunc
	work      chan struct{}

	pinger   p2p.Pinger
	notifier p2p.ReachableNotifier

	wg sync.WaitGroup

	metrics metrics
}

func New(streamer p2p.Pinger, notifier p2p.ReachableNotifier) *reacher {

	ctx, cancel := context.WithCancel(context.Background())

	r := &reacher{
		work:      make(chan struct{}, 1),
		pinger:    streamer,
		peers:     make(map[string]*peer),
		notifier:  notifier,
		ctx:       ctx,
		ctxCancel: cancel,
		metrics:   newMetrics(),
	}

	r.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go r.ping()
	}

	return r
}

func (r *reacher) ping() {

	defer r.wg.Done()

	for {

		p, tryAfter := r.tryAcquirePeer()

		// if no peer is returned, wait until either the
		// next entry to the queue or the closest retry-after time.
		if p == nil {
			if tryAfter == 0 {
				// wait for a new entry to the queue
				select {
				case <-r.ctx.Done():
					return
				case <-r.work:
					continue
				}
			} else {
				// wait for the next peer retry after
				select {
				case <-r.ctx.Done():
					return
				case <-r.work:
					continue
				case <-time.After(tryAfter):
					continue
				}
			}
		}

		r.mu.Lock()
		p.attempts++
		var (
			overlay  = p.overlay
			attempts = p.attempts
			now      = time.Now()
		)
		r.mu.Unlock()

		ctxt, cancel := context.WithTimeout(r.ctx, pingTimeout)
		_, err := r.pinger.Ping(ctxt, p.addr)
		cancel()

		// ping was successful
		if err == nil {
			r.metrics.Pings.WithLabelValues("success").Inc()
			r.metrics.PingTime.WithLabelValues("success").Observe(time.Since(now).Seconds())
			r.notifier.Reachable(overlay, p2p.ReachabilityStatusPublic)
			r.peerState(p, cleanup)
			continue
		}

		r.metrics.Pings.WithLabelValues("failure").Inc()
		r.metrics.PingTime.WithLabelValues("failure").Observe(time.Since(now).Seconds())

		// max attempts have been reached
		if attempts >= pingMaxAttempts {
			r.notifier.Reachable(overlay, p2p.ReachabilityStatusPrivate)
			r.peerState(p, cleanup)
			continue
		}

		// mark peer as 'waiting', increase retry-after duration, and notify workers about more work
		r.mu.Lock()
		if p.state != cleanup { // check if there was a Disconnected call
			p.state = waiting
			p.retryAfter = time.Now().Add(retryAfterDuration * time.Duration(attempts))
		}
		r.mu.Unlock()
	}
}

func (r *reacher) peerState(p *peer, s peerState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.state = s
}

func (r *reacher) tryAcquirePeer() (*peer, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.peers) == 0 {
		return nil, 0
	}

	now := time.Now()
	nextClosest := time.Time{}

	for o, p := range r.peers {

		if p.state == cleanup {
			delete(r.peers, o)
			continue
		}

		if p.state == inProgress {
			continue
		}

		// here, retry after is in the past so we can ping this peer
		if now.After(p.retryAfter) {
			p.state = inProgress
			return p, 0
		}

		// here, we find the peer with the earliest retry after
		if nextClosest.IsZero() || p.retryAfter.Before(nextClosest) {
			nextClosest = p.retryAfter
		}
	}

	if nextClosest.IsZero() {
		return nil, 0
	}

	return nil, nextClosest.Sub(now)

	// return the time to wait until the closest retry after
}

// Connected adds a new peer to the queue for testing reachability.
func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.peers[overlay.ByteString()]; !ok {
		r.peers[overlay.ByteString()] = &peer{overlay: overlay, addr: addr}
	}

	select {
	case r.work <- struct{}{}:
	default:
	}
}

// Disconnected removes a peer from the queue.
func (r *reacher) Disconnected(overlay swarm.Address) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p, ok := r.peers[overlay.ByteString()]; ok {
		p.state = cleanup
	}
}

// Close stops the worker. Must be called once.
func (r *reacher) Close() error {
	r.ctxCancel()
	r.wg.Wait()
	return nil
}
