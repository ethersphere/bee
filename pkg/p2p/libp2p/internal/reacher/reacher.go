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
	pingTimeoutMin  = time.Second * 2
	pingTimeoutMax  = time.Second * 5
	pingMaxAttempts = 3
	workers         = 8
)

type peer struct {
	overlay swarm.Address
	addr    ma.Multiaddr
}

type reacher struct {
	mu    sync.Mutex
	queue []peer

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

		select {
		case <-r.ctx.Done():
			return
		case <-r.work:
		}

		r.mu.Lock()
		if len(r.queue) == 0 {
			r.mu.Unlock()
			continue
		}
		p := r.queue[0]
		r.queue = r.queue[1:]
		r.mu.Unlock()

		timeout := pingTimeoutMin
		attempts := 0

		for {

			select {
			case <-r.ctx.Done():
				return
			default:
			}

			attempts++

			now := time.Now()

			ctxt, cancel := context.WithTimeout(r.ctx, timeout)
			_, err := r.pinger.Ping(ctxt, p.addr)
			cancel()

			if err == nil {
				r.metrics.Pings.WithLabelValues("success").Inc()
				r.metrics.PingTime.WithLabelValues("success").Observe(time.Since(now).Seconds())
				r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPublic)
				break
			}

			r.metrics.Pings.WithLabelValues("failure").Inc()
			r.metrics.PingTime.WithLabelValues("failure").Observe(time.Since(now).Seconds())

			if attempts == pingMaxAttempts {
				r.notifier.Reachable(p.overlay, p2p.ReachabilityStatusPrivate)
				break
			}

			timeout *= 2
			if timeout > pingTimeoutMax {
				timeout = pingTimeoutMax
			}
		}
	}
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

	r.queue = append(r.queue, peer{overlay: overlay, addr: addr})

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
