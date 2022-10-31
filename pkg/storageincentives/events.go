// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"sync"
)

type PhaseType int

const (
	commit PhaseType = iota + 1
	reveal
	claim
	sample
	sampleEnd
)

func (p PhaseType) String() string {
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
	mtx      sync.Mutex
	previous PhaseType
	ev       map[PhaseType]*event
}

type event struct {
	funcs  []func(context.Context, uint64, PhaseType)
	ctx    context.Context
	cancel context.CancelFunc
}

func newEvents() *events {
	return &events{
		ev: make(map[PhaseType]*event),
	}
}

func (e *events) On(phase PhaseType, f func(context.Context, uint64, PhaseType)) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if _, ok := e.ev[phase]; !ok {
		ctx, cancel := context.WithCancel(context.Background())
		e.ev[phase] = &event{ctx: ctx, cancel: cancel}
	}

	e.ev[phase].funcs = append(e.ev[phase].funcs, f)
}

func (e *events) Publish(phase PhaseType, block uint64) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if ev, ok := e.ev[phase]; ok {
		for _, v := range ev.funcs {
			go v(ev.ctx, block, e.previous)
		}
	}
	e.previous = phase
}

func (e *events) Cancel(phases ...PhaseType) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	for _, phase := range phases {
		if ev, ok := e.ev[phase]; ok {
			ev.cancel()
			ctx, cancel := context.WithCancel(context.Background())
			ev.ctx = ctx
			ev.cancel = cancel
		}
	}
}

func (e *events) Close() {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	for k, ev := range e.ev {
		ev.cancel()
		delete(e.ev, k)
	}
}
