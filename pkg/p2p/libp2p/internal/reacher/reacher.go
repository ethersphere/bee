// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reacher runs a background worker that will ping peers
// from an internal queue and report back the reachability to the notifier.
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
	pingTimeout        = time.Second * 5
	pingMaxAttempts    = 3
	workers            = 8
	retryAfterDuration = time.Second * 15
)

type peerState int

const (
	waiting peerState = iota
	inProgress
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

	work chan struct{}
	quit chan struct{}

	pinger   p2p.Pinger
	notifier p2p.ReachableNotifier

	wg      sync.WaitGroup
	metrics metrics

	options *Options
}

type Options struct {
	PingTimeout        time.Duration
	PingMaxAttempts    int
	Workers            int
	RetryAfterDuration time.Duration
}

func New(streamer p2p.Pinger, notifier p2p.ReachableNotifier, o *Options) *reacher {

	r := &reacher{
		work:     make(chan struct{}, 1),
		quit:     make(chan struct{}),
		pinger:   streamer,
		peers:    make(map[string]*peer),
		notifier: notifier,
		metrics:  newMetrics(),
	}

	if o == nil {
		o = &Options{
			PingTimeout:        pingTimeout,
			PingMaxAttempts:    pingMaxAttempts,
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
			case <-r.work:
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
			case <-r.work:
				continue
			}
		}

		// send p to channel
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

		r.mu.Lock()
		p.attempts++
		var (
			overlay  = p.overlay
			attempts = p.attempts
		)
		r.mu.Unlock()

		now := time.Now()

		ctxt, cancel := context.WithTimeout(ctx, r.options.PingTimeout)
		_, err := r.pinger.Ping(ctxt, p.addr)
		cancel()

		// ping was successful
		if err == nil {
			r.metrics.Pings.WithLabelValues("success").Inc()
			r.metrics.PingTime.WithLabelValues("success").Observe(time.Since(now).Seconds())
			r.notifier.Reachable(overlay, p2p.ReachabilityStatusPublic)
			r.deletePeer(p)
			continue
		}

		r.metrics.Pings.WithLabelValues("failure").Inc()
		r.metrics.PingTime.WithLabelValues("failure").Observe(time.Since(now).Seconds())

		// max attempts have been reached
		if attempts >= r.options.PingMaxAttempts {
			r.notifier.Reachable(overlay, p2p.ReachabilityStatusPrivate)
			r.deletePeer(p)
			continue
		}

		// mark peer as 'waiting', increase retry-after duration, and notify workers about more work
		r.mu.Lock()
		p.state = waiting
		p.retryAfter = time.Now().Add(r.options.RetryAfterDuration * time.Duration(attempts))
		r.mu.Unlock()

		r.notifyManage()
	}
}

func (r *reacher) tryAcquirePeer() (*peer, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	nextClosest := time.Time{}

	for _, p := range r.peers {

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

	// return the time to wait until the closest retry after
	return nil, time.Until(nextClosest)
}

func (r *reacher) notifyManage() {
	select {
	case r.work <- struct{}{}:
	default:
	}
}

func (r *reacher) deletePeer(p *peer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.peers, p.overlay.ByteString())
}

// Connected adds a new peer to the queue for testing reachability.
func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.peers[overlay.ByteString()]; !ok {
		r.peers[overlay.ByteString()] = &peer{overlay: overlay, addr: addr}
	}

	r.notifyManage()
}

// Disconnected removes a peer from the queue.
func (r *reacher) Disconnected(overlay swarm.Address) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.peers, overlay.ByteString())
}

// Close stops the worker. Must be called once.
func (r *reacher) Close() error {
	close(r.quit)
	r.wg.Wait()
	return nil
}
