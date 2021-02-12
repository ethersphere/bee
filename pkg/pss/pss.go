// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pss exposes functionalities needed to communicate
// with other peers on the network. Pss uses pushsync and
// pullsync for message delivery and mailboxing. All messages are disguised as content-addressed chunks. Sending and
// receiving of messages is exposed over the HTTP API, with
// websocket subscriptions for incoming messages.
package pss

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"io"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_            Interface = (*pss)(nil)
	ErrNoHandler           = errors.New("no handler found")
)

type Sender interface {
	// Send arbitrary byte slice with the given topic to Targets.
	Send(context.Context, Topic, []byte, *ecdsa.PublicKey, Targets) error
}

type Interface interface {
	Sender
	// Register a Handler for a given Topic.
	Register(Topic, Handler) func()
	// TryUnwrap tries to unwrap a wrapped trojan message.
	TryUnwrap(swarm.Chunk)

	SetPushSyncer(pushSyncer pushsync.PushSyncer)
	io.Closer
}

type pss struct {
	key        *ecdsa.PrivateKey
	pusher     pushsync.PushSyncer
	handlers   map[Topic][]*Handler
	handlersMu sync.Mutex
	metrics    metrics
	logger     logging.Logger
	quit       chan struct{}
}

// New returns a new pss service.
func New(key *ecdsa.PrivateKey, logger logging.Logger) Interface {
	return &pss{
		key:      key,
		logger:   logger,
		handlers: make(map[Topic][]*Handler),
		metrics:  newMetrics(),
		quit:     make(chan struct{}),
	}
}

func (ps *pss) Close() error {
	close(ps.quit)
	ps.handlersMu.Lock()
	defer ps.handlersMu.Unlock()

	ps.handlers = make(map[Topic][]*Handler) //unset handlers on shutdown

	return nil
}

func (ps *pss) SetPushSyncer(pushSyncer pushsync.PushSyncer) {
	ps.pusher = pushSyncer
}

// Handler defines code to be executed upon reception of a trojan message.
type Handler func(context.Context, []byte)

// Send constructs a padded message with topic and payload,
// wraps it in a trojan chunk such that one of the targets is a prefix of the chunk address.
// Uses push-sync to deliver message.
func (p *pss) Send(ctx context.Context, topic Topic, payload []byte, recipient *ecdsa.PublicKey, targets Targets) error {
	p.metrics.TotalMessagesSentCounter.Inc()

	tc, err := Wrap(ctx, topic, payload, recipient, targets)
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
func (p *pss) Register(topic Topic, handler Handler) (cleanup func()) {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	p.handlers[topic] = append(p.handlers[topic], &handler)

	return func() {
		p.handlersMu.Lock()
		defer p.handlersMu.Unlock()

		h := p.handlers[topic]
		for i := 0; i < len(h); i++ {
			if h[i] == &handler {
				p.handlers[topic] = append(h[:i], h[i+1:]...)
				return
			}
		}
	}
}

func (p *pss) topics() []Topic {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	ts := make([]Topic, 0, len(p.handlers))
	for t := range p.handlers {
		ts = append(ts, t)
	}

	return ts
}

// TryUnwrap allows unwrapping a chunk as a trojan message and calling its handlers based on the topic.
func (p *pss) TryUnwrap(c swarm.Chunk) {
	if len(c.Data()) < swarm.ChunkWithSpanSize {
		return // chunk not full
	}
	ctx := context.Background()
	topic, msg, err := Unwrap(ctx, p.key, c, p.topics())
	if err != nil {
		return // cannot unwrap
	}
	h := p.getHandlers(topic)
	if h == nil {
		return // no handler
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var wg sync.WaitGroup
	go func() {
		defer cancel()
		select {
		case <-p.quit:
		case <-done:
		}
	}()
	for _, hh := range h {
		wg.Add(1)
		go func(hh Handler) {
			defer wg.Done()
			hh(ctx, msg)
		}(*hh)
	}
	go func() {
		wg.Wait()
		close(done)
	}()
}

func (p *pss) getHandlers(topic Topic) []*Handler {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	return p.handlers[topic]
}
