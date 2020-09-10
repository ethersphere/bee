// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

var (
	_            Interface = (*pss)(nil)
	ErrNoHandler           = errors.New("no handler found")
)

type Interface interface {
	// Send arbitrary byte slice with the given topic to Targets.
	Send(context.Context, trojan.Targets, trojan.Topic, []byte) error
	// Register a Handler for a given Topic.
	Register(trojan.Topic, Handler) func()
	// TryUnwrap tries to unwrap a wrapped trojan message.
	TryUnwrap(context.Context, swarm.Chunk) error

	SetPushSyncer(pushSyncer pushsync.PushSyncer)
}

type pss struct {
	pusher        pushsync.PushSyncer
	handlers      map[trojan.Topic]map[int]Handler
	handlersCount int // monotonically increasing counter
	handlersMu    sync.Mutex
	metrics       metrics
	logger        logging.Logger
}

// New returns a new pss service.
func New(logger logging.Logger) Interface {
	return &pss{
		logger:   logger,
		handlers: make(map[trojan.Topic]map[int]Handler),
		metrics:  newMetrics(),
	}
}

func (ps *pss) SetPushSyncer(pushSyncer pushsync.PushSyncer) {
	ps.pusher = pushSyncer
}

// Handler defines code to be executed upon reception of a trojan message.
type Handler func(context.Context, *trojan.Message)

// Send constructs a padded message with topic and payload,
// wraps it in a trojan chunk such that one of the targets is a prefix of the chunk address.
// Uses push-sync to deliver message.
func (p *pss) Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) error {
	p.metrics.TotalMessagesSentCounter.Inc()

	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		return err
	}

	var tc swarm.Chunk
	tc, err = m.Wrap(ctx, targets)
	if err != nil {
		return err
	}

	// push the chunk using push sync so that it reaches it destination in network
	if _, err = p.pusher.PushChunkToClosest(ctx, tc); err != nil {
		return err
	}

	return nil
}

// Register allows the definition of a Handler func for a specific topic on the pss struct.
func (p *pss) Register(topic trojan.Topic, handler Handler) (cleanup func()) {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	if p.handlers[topic] == nil {
		p.handlers[topic] = make(map[int]Handler)
	}

	p.handlers[topic][p.handlersCount] = handler
	cleanup = func(id int) func() {
		return func() {
			p.handlersMu.Lock()
			defer p.handlersMu.Unlock()

			h := p.handlers[topic]
			delete(h, id)
		}
	}(p.handlersCount)
	p.handlersCount++

	return cleanup
}

// TryUnwrap allows unwrapping a chunk as a trojan message and calling its handlers based on the topic.
func (p *pss) TryUnwrap(ctx context.Context, c swarm.Chunk) error {
	if !trojan.IsPotential(c) {
		return nil
	}
	m, err := trojan.Unwrap(c)
	if err != nil {
		return err
	}
	h := p.getHandlers(m.Topic)
	if h == nil {
		return fmt.Errorf("topic %v, %w", m.Topic, ErrNoHandler)
	}
	for _, hh := range h {
		hh(ctx, m)
	}
	return nil
}

func (p *pss) getHandlers(topic trojan.Topic) (handlers []Handler) {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	for _, v := range p.handlers[topic] {
		v := v
		handlers = append(handlers, v)
	}

	return handlers
}
