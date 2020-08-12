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
	Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) error
	Register(topic trojan.Topic, hndlr Handler)
	GetHandler(topic trojan.Topic) Handler
	TryUnwrap(ctx context.Context, c swarm.Chunk) error
}

// pss is the top-level struct, which takes care of message sending
type pss struct {
	pusher     pushsync.PushSyncer
	handlers   map[trojan.Topic]Handler
	handlersMu sync.RWMutex
	metrics    metrics
	logger     logging.Logger
}

// New inits the pss struct with the storer
func New(logger logging.Logger, pusher pushsync.PushSyncer) Interface {
	return &pss{
		logger:   logger,
		pusher:   pusher,
		handlers: make(map[trojan.Topic]Handler),
		metrics:  newMetrics(),
	}
}

// Handler defines code to be executed upon reception of a trojan message
type Handler func(*trojan.Message)

// Send constructs a padded message with topic and payload,
// wraps it in a trojan chunk such that one of the targets is a prefix of the chunk address
// uses push-sync to deliver message
func (p *pss) Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) error {
	p.metrics.TotalMessagesSentCounter.Inc()

	//construct Trojan Chunk
	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		return err
	}
	var tc swarm.Chunk
	tc, err = m.Wrap(targets)
	if err != nil {

		return err
	}

	// push the chunk using push sync so that it reaches it destination in network
	if _, err = p.pusher.PushChunkToClosest(ctx, tc); err != nil {
		return err
	}

	return nil
}

// Register allows the definition of a Handler func for a specific topic on the pss struct
func (p *pss) Register(topic trojan.Topic, hndlr Handler) {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()
	p.handlers[topic] = hndlr
}

// TryUnwrap allows unwrapping a chunk as a trojan message and calling its handler func based on its topic
func (p *pss) TryUnwrap(ctx context.Context, c swarm.Chunk) error {

	if !trojan.IsPotential(c) {
		return nil
	}
	m, err := trojan.Unwrap(c) // if err occurs unwrapping, there will be no handler
	if err != nil {
		return err
	}
	h := p.GetHandler(m.Topic)
	if h == nil {
		return fmt.Errorf("topic %v, %w", m.Topic, ErrNoHandler)
	}
	h(m)
	return nil
}

// GetHandler returns the Handler func registered in pss for the given topic
func (p *pss) GetHandler(topic trojan.Topic) Handler {
	p.handlersMu.RLock()
	defer p.handlersMu.RUnlock()
	return p.handlers[topic]
}
