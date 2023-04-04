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
	default:
		return "unknown"
	}
}

type events struct {
	mtx sync.Mutex
	ev  map[PhaseType]*event
}

type event struct {
	funcs  []func(context.Context)
	ctx    context.Context
	cancel context.CancelFunc
}

func newEvents() *events {
	return &events{
		ev: make(map[PhaseType]*event),
	}
}

func (e *events) On(phase PhaseType, f func(context.Context)) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if _, ok := e.ev[phase]; !ok {
		ctx, cancel := context.WithCancel(context.Background())
		e.ev[phase] = &event{ctx: ctx, cancel: cancel}
	}

	e.ev[phase].funcs = append(e.ev[phase].funcs, f)
}

func (e *events) Publish(phase PhaseType) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	if ev, ok := e.ev[phase]; ok {
		for _, v := range ev.funcs {
			go v(ev.ctx)
		}
	}
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
