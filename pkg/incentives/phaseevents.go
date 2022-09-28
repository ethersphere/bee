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
	default:
		return "unknown"
	}
}

type phaseEvents struct {
	subs    map[phaseType][]func(context.Context)
	ctx     map[phaseType]context.Context
	cancelF map[phaseType]context.CancelFunc
	mtx     sync.Mutex
}

func newPhaseEvents() *phaseEvents {
	return &phaseEvents{
		subs:    make(map[phaseType][]func(context.Context)),
		ctx:     make(map[phaseType]context.Context),
		cancelF: make(map[phaseType]context.CancelFunc),
	}
}

func (ps *phaseEvents) On(phase phaseType, f func(context.Context)) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.subs[phase] = append(ps.subs[phase], f)
	if _, ok := ps.ctx[phase]; !ok {
		ctx, cancel := context.WithCancel(context.Background())
		ps.ctx[phase] = ctx
		ps.cancelF[phase] = cancel
	}
}

func (ps *phaseEvents) Publish(phase phaseType) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ctx := ps.ctx[phase]
	for _, v := range ps.subs[phase] {
		go v(ctx)
	}
}

func (ps *phaseEvents) Cancel(phases ...phaseType) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	for _, phase := range phases {
		cancel := ps.cancelF[phase]
		cancel()

		ctx, cancel := context.WithCancel(context.Background())
		ps.ctx[phase] = ctx
		ps.cancelF[phase] = cancel
	}
}
