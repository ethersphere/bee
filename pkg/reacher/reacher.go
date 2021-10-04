// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Reacher package runs a background worker that will ping peers
// from an internal queue and report back the reachability to some notifier
package reacher

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	pingTimeoutMin = time.Second * 2
	pingTimeoutMax = time.Second * 5
	pingMaxAttemps = 3
	maxWorkers     = 16
)

type peer struct {
	overlay swarm.Address
	addr    ma.Multiaddr
}

type reacher struct {
	mu    sync.Mutex
	queue []peer

	ctx      context.Context
	ctxClose context.CancelFunc
	run      chan struct{}

	pinger   p2p.Pinger
	notifier p2p.ReachableNotifier

	wg  sync.WaitGroup
	sem *semaphore.Weighted
}

func New(streamer p2p.Pinger, notifier p2p.ReachableNotifier) *reacher {

	ctx, cancel := context.WithCancel(context.Background())

	r := &reacher{
		run:      make(chan struct{}, 1),
		pinger:   streamer,
		notifier: notifier,
		wg:       sync.WaitGroup{},
		sem:      semaphore.NewWeighted(maxWorkers),
		ctx:      ctx,
		ctxClose: cancel,
	}

	r.wg.Add(1)
	go r.worker()

	return r
}

func (r *reacher) worker() {

	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.run:
			if err := r.sem.Acquire(r.ctx, 1); err == nil {
				r.wg.Add(1)
				go r.ping()
			}
		}
	}
}

func (r *reacher) ping() {

	defer r.wg.Done()
	defer r.sem.Release(1)

	for {

		r.mu.Lock()
		if len(r.queue) == 0 {
			r.mu.Unlock()
			return
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

			ctxt, cancel := context.WithTimeout(r.ctx, timeout)
			_, err := r.pinger.Ping(ctxt, p.addr)
			cancel()

			if err == nil {
				r.notifier.Reachable(p.overlay, true)
				break
			}

			if attempts == pingMaxAttemps {
				r.notifier.Reachable(p.overlay, false)
				break
			}

			timeout *= 2
			if timeout > pingTimeoutMax {
				timeout = pingTimeoutMax
			}
		}
	}
}

func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	r.mu.Lock()
	r.queue = append(r.queue, peer{overlay: overlay, addr: addr})
	r.mu.Unlock()

	select {
	case r.run <- struct{}{}:
	default:
	}
}

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

func (r *reacher) Close() error {
	r.ctxClose()
	r.wg.Wait()
	return nil
}
