// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gsoc

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Handler defines code to be executed upon reception of a GSOC sub message.
// it is used as a parameter definition. It receives the recovered single owner
// chunk so the consumer has access to all of its properties.
type Handler func(*soc.SOC)

type Listener interface {
	Subscribe(address swarm.Address, handler Handler) (cleanup func())
	Handle(c *soc.SOC)
	Close() error
}

type listener struct {
	handlers   map[string][]*Handler
	handlersMu sync.RWMutex
	subCount   atomic.Int32
	quit       chan struct{}
	logger     log.Logger
}

// New returns a new GSOC listener service.
func New(logger log.Logger) Listener {
	return &listener{
		logger:   logger,
		handlers: make(map[string][]*Handler),
		quit:     make(chan struct{}),
	}
}

// Subscribe allows the definition of a Handler func on a specific GSOC address.
//
// Handle iterates the handlers of an address without holding handlersMu, so a
// slice that has been handed out must never be written to again. Subscribing
// and unsubscribing therefore publish a new slice instead of appending to, or
// shifting elements within, the backing array a concurrent Handle may be
// reading.
func (l *listener) Subscribe(address swarm.Address, handler Handler) (cleanup func()) {
	key := address.ByteString()

	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers[key] = append(slices.Clone(l.handlers[key]), &handler)

	return func() {
		l.handlersMu.Lock()
		defer l.handlersMu.Unlock()

		h := l.handlers[key]
		for i := range h {
			if h[i] == &handler {
				if len(h) == 1 {
					// drop the entry with its last subscriber, so that
					// addresses subscribed to briefly do not accumulate in
					// the map for the lifetime of the node.
					delete(l.handlers, key)
				} else {
					l.handlers[key] = slices.Delete(slices.Clone(h), i, i+1)
				}
				return
			}
		}
	}
}

// Handle is called by push/pull sync and passes the chunk its registered handler
func (l *listener) Handle(c *soc.SOC) {
	if l.subCount.Load() == 0 {
		return // no subscriptions, skip lock
	}

	addr, err := c.Address()
	if err != nil {
		return // no handler
	}
	h := l.getHandlers(addr)
	if len(h) == 0 {
		return // no handler
	}
	l.logger.Debug("new incoming GSOC message", "GSOC Address", addr, "wrapped chunk address", c.WrappedChunk().Address())

	for _, hh := range h {
		(*hh)(c)
	}
}

// getHandlers returns the handlers currently subscribed to address. The
// returned slice is shared with the subscription bookkeeping and must only be
// read, see Subscribe.
func (p *listener) getHandlers(address swarm.Address) []*Handler {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	return p.handlers[address.ByteString()]
}

func (l *listener) Close() error {
	close(l.quit)
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers = make(map[string][]*Handler) // unset handlers on shutdown

	return nil
}
