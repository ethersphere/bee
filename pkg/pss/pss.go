// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss

import (
	"context"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/trojan"
)

// Pss is the top-level struct, which takes care of message sending
type Pss struct {
	storer     storage.Storer
	tags       *tags.Tags
	handlers   map[trojan.Topic]Handler
	handlersMu sync.RWMutex
	//metrics metrics
	//logger  logging.Logger
}

// Monitor is used for tracking status changes in sent trojan chunks
type Monitor struct {
	// returns the state of the trojan chunk that is being monitored
	State chan tags.State
}

// NewPss inits the Pss struct with the localstore
func NewPss(localStore storage.Storer, tags *tags.Tags) *Pss {
	return &Pss{
		storer:   localStore,
		tags:     tags,
		handlers: make(map[trojan.Topic]Handler),
	}
}

// Handler defines code to be executed upon reception of a trojan message
type Handler func(trojan.Message)

// Send constructs a padded message with topic and payload,
// wraps it in a trojan chunk such that one of the targets is a prefix of the chunk address
// stores this in localstore for push-sync to pick up and deliver
func (p *Pss) Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) (*Monitor, error) {
	// TODO RESOLVE METRICS
	//metrics.GetOrRegisterCounter("trojanchunk/send", nil).Inc(1)

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

	// SAVE trojanChunk to localstore, if it exists do nothing as it's already peristed
	if _, err = p.storer.Put(ctx, storage.ModePutUpload, tc.WithTagID(tag.Uid)); err != nil {
		return nil, err
	}
	tag.Total = 1

	monitor := &Monitor{
		State: make(chan tags.State, 3),
	}

	go monitor.updateState(tag)

	return monitor, nil
}

// updateState sends the change of state thru the State channel
// this is what enables monitoring the trojan chunk after it's sent
func (m *Monitor) updateState(tag *tags.Tag) {
	for _, state := range []tags.State{tags.StateStored, tags.StateSent, tags.StateSynced} {
		for {
			n, total, err := tag.Status(state)
			if err == nil && n == total {
				m.State <- state
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
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
		m, _ := trojan.Unwrap(c) // if err occurs unwrapping, there will be no handler
		h := p.GetHandler(m.Topic)
		if h != nil {
			//TODO replace with logger
			//log.Debug("executing handler for trojan", "process", "global-pinning", "chunk", hex.EncodeToString(c.Address()))
			h(*m)
			return
		}
	}
	//TODO replace with logger
	//log.Debug("chunk not trojan or no handler found", "process", "global-pinning", "chunk", hex.EncodeToString(c.Address()))
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
