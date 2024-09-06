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

type Listener interface {
	Subscribe(address [32]byte, handler handler) (cleanup func())
	Handle(c soc.SOC)
	Close() error
}

type listener struct {
	handlers   map[[32]byte][]*handler
	handlersMu sync.Mutex
	quit       chan struct{}
	logger     log.Logger
}

// New returns a new pss service.
func New(logger log.Logger) Listener {
	return &listener{
		logger:   logger,
		handlers: make(map[[32]byte][]*handler),
		quit:     make(chan struct{}),
	}
}

// Subscribe allows the definition of a Handler func for a specific topic on the pss struct.
func (l *listener) Subscribe(address [32]byte, handler handler) (cleanup func()) {
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers[address] = append(l.handlers[address], &handler)

	return func() {
		l.handlersMu.Lock()
		defer l.handlersMu.Unlock()

		h := l.handlers[address]
		for i := 0; i < len(h); i++ {
			if h[i] == &handler {
				l.handlers[address] = append(h[:i], h[i+1:]...)
				return
			}
		}
	}
}

// Handle is called by push/pull sync and passes the chunk its registered handler
func (l *listener) Handle(c soc.SOC) {
	addr, err := c.Address()
	if err != nil {
		return // no handler
	}
	h := l.getHandlers([32]byte(addr.Bytes()))
	if h == nil {
		return // no handler
	}
	l.logger.Info("new incoming GSOC message",
		"GSOC Address", addr,
		"wrapped chunk address", c.WrappedChunk().Address())

	var wg sync.WaitGroup
	for _, hh := range h {
		wg.Add(1)
		go func(hh handler) {
			defer wg.Done()
			hh(c.WrappedChunk().Data()[swarm.SpanSize:])
		}(*hh)
	}
}

func (p *listener) getHandlers(address [32]byte) []*handler {
	p.handlersMu.Lock()
	defer p.handlersMu.Unlock()

	return p.handlers[address]
}

func (l *listener) Close() error {
	close(l.quit)
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers = make(map[[32]byte][]*handler) //unset handlers on shutdown

	return nil
}

// handler defines code to be executed upon reception of a GSOC sub message.
type handler func([]byte)
