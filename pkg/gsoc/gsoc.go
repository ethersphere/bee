// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gsoc

import (
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Handler defines code to be executed upon reception of a GSOC sub message.
// it is used as a parameter definition.
type Handler func([]byte)

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
func (l *listener) Subscribe(address swarm.Address, handler Handler) (cleanup func()) {
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers[address.ByteString()] = append(l.handlers[address.ByteString()], &handler)
	l.subCount.Add(1)

	return func() {
		l.handlersMu.Lock()
		defer l.handlersMu.Unlock()

		h := l.handlers[address.ByteString()]
		for i := range h {
			if h[i] == &handler {
				l.handlers[address.ByteString()] = append(h[:i], h[i+1:]...)
				l.subCount.Add(-1)
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
	payload := c.WrappedChunk().Data()[swarm.SpanSize:]

	// The read lock is held for the whole iteration so that a concurrent
	// Subscribe cleanup (write lock) cannot mutate the handlers slice while it
	// is being ranged over. Each handler is dereferenced and dispatched to its
	// own goroutine, so the lock is never held while a handler runs.
	l.handlersMu.RLock()
	defer l.handlersMu.RUnlock()

	h := l.handlers[addr.ByteString()]
	if h == nil {
		return // no handler
	}
	l.logger.Debug("new incoming GSOC message", "GSOC Address", addr, "wrapped chunk address", c.WrappedChunk().Address())

	for _, hh := range h {
		go func(hh Handler) {
			hh(payload)
		}(*hh)
	}
}

func (l *listener) Close() error {
	close(l.quit)
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers = make(map[string][]*Handler) // unset handlers on shutdown

	return nil
}
