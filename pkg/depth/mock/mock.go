// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mock provides a depth.Provider whose value a test sets directly.
package mock

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/depth"
	"github.com/prometheus/client_golang/prometheus"
)

var _ depth.Provider = (*Provider)(nil)

// Provider is a depth.Provider driven by Set.
type Provider struct {
	mu    sync.Mutex
	depth uint8
	state depth.State
	ready chan struct{}
	subs  map[chan uint8]struct{}
}

// Option configures the initial value of a Provider.
type Option func(*Provider)

// WithDepth sets the initial depth.
func WithDepth(d uint8) Option { return func(p *Provider) { p.depth = d } }

// WithState sets the initial state.
func WithState(s depth.State) Option { return func(p *Provider) { p.state = s } }

// New returns a Provider at depth 0 and depth.StateNoData unless options say otherwise.
func New(opts ...Option) *Provider {
	p := &Provider{
		state: depth.StateNoData,
		ready: make(chan struct{}),
		subs:  make(map[chan uint8]struct{}),
	}
	for _, o := range opts {
		o(p)
	}
	if p.state != depth.StateNoData {
		close(p.ready)
	}
	return p
}

// Set updates the depth and the state, wakes every Wait when the state is not
// depth.StateNoData and sends the depth to every subscriber when the state is depth.StateKnown.
func (p *Provider) Set(d uint8, s depth.State) {
	p.mu.Lock()
	defer p.mu.Unlock()

	wasReady := p.state != depth.StateNoData
	p.depth, p.state = d, s

	switch {
	case s != depth.StateNoData && !wasReady:
		close(p.ready)
	case s == depth.StateNoData && wasReady:
		p.ready = make(chan struct{})
	}
	if s == depth.StateKnown {
		for ch := range p.subs {
			send(ch, d)
		}
	}
}

func (p *Provider) Depth() (uint8, depth.State) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.depth, p.state
}

func (p *Provider) Wait(ctx context.Context) (uint8, depth.State, error) {
	for {
		p.mu.Lock()
		d, s, ch := p.depth, p.state, p.ready
		p.mu.Unlock()

		if s != depth.StateNoData {
			return d, s, nil
		}

		select {
		case <-ch:
		case <-ctx.Done():
			d, s := p.Depth()
			return d, s, ctx.Err()
		}
	}
}

func (p *Provider) Subscribe() (<-chan uint8, func()) {
	ch := make(chan uint8, 1)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.subs[ch] = struct{}{}
	if p.state == depth.StateKnown {
		send(ch, p.depth)
	}

	return ch, func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		delete(p.subs, ch)
	}
}

func (p *Provider) SetComparison(string, uint8) {}

func (p *Provider) Metrics() []prometheus.Collector { return nil }

func (p *Provider) Close() error { return nil }

// send puts d on ch, replacing a pending value. It never blocks.
func send(ch chan uint8, d uint8) {
	select {
	case <-ch:
	default:
	}
	ch <- d
}
