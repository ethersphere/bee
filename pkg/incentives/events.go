// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package incentives

import (
	"context"
	"sync"
)

type phaseType int

const (
	commit phaseType = iota
	reveal
	claim
	sample
	sampleEnd
)

func (p phaseType) string() string {
	switch p {
	case commit:
		return "commit"
	case reveal:
		return "reveal"
	case claim:
		return "claim"
	case sample:
		return "sample"
	case sampleEnd:
		return "sampleEnd"
	default:
		return "unknown"
	}
}

type events struct {
	on       map[phaseType][]func(context.Context)
	ctx      map[phaseType]context.Context
	cancelF  map[phaseType]context.CancelFunc
	mtx      sync.Mutex
	previous phaseType
}

func newEvents() *events {
	return &events{
		on:       make(map[phaseType][]func(context.Context)),
		ctx:      make(map[phaseType]context.Context),
		cancelF:  make(map[phaseType]context.CancelFunc),
		previous: -1,
	}
}

func (e *events) On(phase phaseType, f func(context.Context)) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	e.on[phase] = append(e.on[phase], f)

	if _, ok := e.ctx[phase]; !ok {
		ctx, cancel := context.WithCancel(context.Background())
		e.ctx[phase] = ctx
		e.cancelF[phase] = cancel
	}
}

func (e *events) Last() phaseType {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	return e.previous
}

func (e *events) Publish(phase phaseType) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	e.previous = phase

	for _, v := range e.on[phase] {
		go v(e.ctx[phase])
	}
}

func (e *events) Cancel(phases ...phaseType) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	for _, phase := range phases {

		if cancel, ok := e.cancelF[phase]; ok {
			cancel()
		}

		ctx, cancel := context.WithCancel(context.Background())
		e.ctx[phase] = ctx
		e.cancelF[phase] = cancel
	}
}

func (e *events) Close() {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	for k, cancel := range e.cancelF {
		cancel()
		delete(e.cancelF, k)
	}

	for k := range e.ctx {
		delete(e.cancelF, k)
	}

	for k := range e.on {
		delete(e.cancelF, k)
	}
}
