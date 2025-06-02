// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reacher runs a background worker that will ping peers
// from an internal queue and report back the reachability to the notifier.
package reacher

import (
	"context"
	"time"

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
}

type Options struct {
	PingTimeout        time.Duration
	Workers            int
	RetryAfterDuration time.Duration
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

func (r *reacher) tryAcquirePeer() (*peer, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var (
		now         = time.Now()
		nextClosest time.Time
	)

	for _, p := range r.peers {

		// retry after has expired, retry
		if now.After(p.retryAfter) {
			p.retryAfter = time.Now().Add(r.options.RetryAfterDuration)
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

// Connected adds a new peer to the queue for testing reachability.
func (r *reacher) Connected(overlay swarm.Address, addr ma.Multiaddr) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.peers[overlay.ByteString()]; !ok {
		r.peers[overlay.ByteString()] = &peer{overlay: overlay, addr: addr}
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

	delete(r.peers, overlay.ByteString())
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
