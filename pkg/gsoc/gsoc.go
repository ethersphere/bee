// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gsoc

import (
	"sync"

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
	handlersMu sync.Mutex
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

	return func() {
		l.handlersMu.Lock()
		defer l.handlersMu.Unlock()

		h := l.handlers[address.ByteString()]
		for i := 0; i < len(h); i++ {
			if h[i] == &handler {
				l.handlers[address.ByteString()] = append(h[:i], h[i+1:]...)
				return
			}
		}
	}
}

// Handle is called by push/pull sync and passes the chunk its registered handler
func (l *listener) Handle(c *soc.SOC) {
	addr, err := c.Address()
	if err != nil {
		return // no handler
	}
	h := l.getHandlers(addr)
	if h == nil {
		return // no handler
	}
	l.logger.Debug("new incoming GSOC message", "GSOC Address", addr, "wrapped chunk address", c.WrappedChunk().Address())

	for _, hh := range h {
		go func(hh Handler) {
			hh(c.WrappedChunk().Data()[swarm.SpanSize:])
		}(*hh)
	}
}

func (p *listener) getHandlers(address swarm.Address) []*Handler {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	return p.handlers[address.ByteString()]
}

func (l *listener) Close() error {
	close(l.quit)
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers = make(map[string][]*Handler) //unset handlers on shutdown

	return nil
}
