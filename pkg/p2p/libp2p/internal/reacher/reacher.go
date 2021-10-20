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

type peer struct {
	overlay    swarm.Address
	addr       ma.Multiaddr
	retryAfter time.Time
	attempts   int
}

type reacher struct {
	mu    sync.Mutex
	queue []*peer

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

		p, sleepForNext := r.popNextPeer()

		if p == nil {

			if sleepForNext == 0 {
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
				case <-time.After(sleepForNext):
					continue
				}
			}
		}

		now := time.Now()

		ctxt, cancel := context.WithTimeout(r.ctx, pingTimeout)
		_, err := r.pinger.Ping(ctxt, p.addr)
		cancel()

		p.attempts++

		if err == nil {
			r.metrics.Pings.WithLabelValues("success").Inc()
			r.metrics.PingTime.WithLabelValues("success").Observe(time.Since(now).Seconds())
			r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPublic)
			continue
		}

		r.metrics.Pings.WithLabelValues("failure").Inc()
		r.metrics.PingTime.WithLabelValues("failure").Observe(time.Since(now).Seconds())

		if p.attempts >= pingMaxAttempts {
			r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPrivate)
			continue
		}

		// re-add peer to the queue
		p.retryAfter = time.Now().Add(retryAfterDuration * time.Duration(p.attempts))
		r.mu.Lock()
		r.queue = append(r.queue, p)
		r.mu.Unlock()

	}
}

func (r *reacher) popNextPeer() (*peer, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.queue) == 0 {
		return nil, 0
	}

	now := time.Now()
	nextClosest := r.queue[0].retryAfter

	for i, p := range r.queue {
		if now.After(p.retryAfter) {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			return p, 0
		}

		// find the peer with the earliest retry after
		if p.retryAfter.Before(nextClosest) {
			nextClosest = p.retryAfter
		}
	}

	return nil, nextClosest.Sub(now)
}

// Connected adds a new peer to the queue for testing reachability.
func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range r.queue {
		if p.overlay.Equal(overlay) {
			return
		}
	}

	r.queue = append(r.queue, &peer{overlay: overlay, addr: addr})

	select {
	case r.work <- struct{}{}:
	default:
	}
}

// Disconnected removes a peer from the queue.
func (r *reacher) Disconnected(overlay swarm.Address) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, p := range r.queue {
		if p.overlay.Equal(overlay) {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			return
		}
	}
}

// Close stops the worker. Must be called once.
func (r *reacher) Close() error {
	r.ctxCancel()
	r.wg.Wait()
	return nil
}
