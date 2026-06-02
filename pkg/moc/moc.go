// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package moc provides subscriptions to single-owner chunks by their
// identifier (id). A MOC (Mined Owner Chunk) subscription is matched against
// the id of every incoming single-owner chunk, regardless of its owner.
package moc

import (
	"encoding/hex"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Handler defines code to be executed upon reception of a MOC message.
// It is used as a parameter definition.
type Handler func([]byte)

type Listener interface {
	Subscribe(id []byte, handler Handler) (cleanup func())
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

// New returns a new MOC listener service.
func New(logger log.Logger) Listener {
	return &listener{
		logger:   logger,
		handlers: make(map[string][]*Handler),
		quit:     make(chan struct{}),
	}
}

// Subscribe allows the definition of a Handler func on a specific SOC id.
func (l *listener) Subscribe(id []byte, handler Handler) (cleanup func()) {
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	key := string(id)
	l.handlers[key] = append(l.handlers[key], &handler)
	l.subCount.Add(1)

	return func() {
		l.handlersMu.Lock()
		defer l.handlersMu.Unlock()

		h := l.handlers[key]
		for i := range h {
			if h[i] == &handler {
				l.handlers[key] = append(h[:i], h[i+1:]...)
				l.subCount.Add(-1)
				return
			}
		}
	}
}

// Handle is called by push/pull sync and passes the chunk to the handlers
// registered on its id.
func (l *listener) Handle(c *soc.SOC) {
	if l.subCount.Load() == 0 {
		return // no subscriptions, skip lock
	}

	h := l.getHandlers(c.ID())
	if h == nil {
		return // no handler
	}
	l.logger.Debug("new incoming MOC message", "soc id", hex.EncodeToString(c.ID()), "wrapped chunk address", c.WrappedChunk().Address())

	for _, hh := range h {
		go func(hh Handler) {
			hh(c.WrappedChunk().Data()[swarm.SpanSize:])
		}(*hh)
	}
}

func (l *listener) getHandlers(id []byte) []*Handler {
	l.handlersMu.RLock()
	defer l.handlersMu.RUnlock()

	return l.handlers[string(id)]
}

func (l *listener) Close() error {
	close(l.quit)
	l.handlersMu.Lock()
	defer l.handlersMu.Unlock()

	l.handlers = make(map[string][]*Handler) // unset handlers on shutdown

	return nil
}
