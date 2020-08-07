// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/trojan"
)

// Pss is the top-level struct, which takes care of message sending
type Pss struct {
	pusher     pushsync.PushSyncer
	tags       *tags.Tags
	handlers   map[trojan.Topic]Handler
	handlersMu sync.RWMutex
	metrics    metrics
	logger     logging.Logger
}

type Options struct {
	Logger     logging.Logger
	PushSyncer pushsync.PushSyncer
	Tags       *tags.Tags
}

// NewPss inits the Pss struct with the storer
func NewPss(o Options) *Pss {
	return &Pss{
		pusher:   o.PushSyncer,
		tags:     o.Tags,
		handlers: make(map[trojan.Topic]Handler),
		metrics:  newMetrics(),
		logger:   o.Logger,
	}
}

func (ps *Pss) WithPushSyncer(pushSyncer pushsync.PushSyncer) {
	ps.pusher = pushSyncer
}

// Handler defines code to be executed upon reception of a trojan message
type Handler func(context.Context, trojan.Message) error

// Send constructs a padded message with topic and payload,
// wraps it in a trojan chunk such that one of the targets is a prefix of the chunk address
// uses push-sync to deliver message
func (p *Pss) Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) (*tags.Tag, error) {
	p.metrics.TotalMessagesSentCounter.Inc()

	//construct Trojan Chunk
	m, err := trojan.NewMessage(topic, payload)
	if err != nil {
		return nil, err
	}
	var tc swarm.Chunk
	tc, err = m.Wrap(targets)
	if err != nil {
		return nil, err
	}

	tag, err := p.tags.Create("pss-chunks-tag", 1, false)
	if err != nil {
		return nil, err
	}

	// push the chunk using push sync so that it reaches it destination in network
	if _, err = p.pusher.PushChunkToClosest(ctx, tc.WithTagID(tag.Uid)); err != nil {
		return nil, err
	}

	tag.Total = 1

	return tag, nil
}

// Register allows the definition of a Handler func for a specific topic on the pss struct
func (p *Pss) Register(topic trojan.Topic, hndlr Handler) {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()
	p.handlers[topic] = hndlr
}

// Deliver allows unwrapping a chunk as a trojan message and calling its handler func based on its topic
func (p *Pss) Deliver(c swarm.Chunk) {
	if trojan.IsPotential(c) {
		ctx := context.Background()
		m, _ := trojan.Unwrap(c) // if err occurs unwrapping, there will be no handler
		h := p.GetHandler(m.Topic)
		if h != nil {
			p.logger.Debug("executing handler for trojan", "process", "global-pinning", "chunk", c.Address().ByteString())
			h(ctx, *m)
			return
		}
	}
	p.logger.Debug("chunk not trojan or no handler found", "process", "global-pinning", "chunk", c.Address().ByteString())
}

// GetHandler returns the Handler func registered in pss for the given topic
func (p *Pss) GetHandler(topic trojan.Topic) Handler {
	p.handlersMu.RLock()
	defer p.handlersMu.RUnlock()
	return p.handlers[topic]
}

// GetAllHandlers returns all the Handler funcs registered in pss
func (p *Pss) GetAllHandlers() map[trojan.Topic]Handler {
	p.handlersMu.RLock()
	defer p.handlersMu.RUnlock()
	return p.handlers
}
