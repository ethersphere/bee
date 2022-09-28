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

type phaseEvents struct {
	on map[phaseType][]func(context.Context)
	// once     map[phaseType][]func(context.Context)
	ctx      map[phaseType]context.Context
	cancelF  map[phaseType]context.CancelFunc
	mtx      sync.Mutex
	previous phaseType
}

func newEvents() *phaseEvents {
	return &phaseEvents{
		on: make(map[phaseType][]func(context.Context)),
		// once:     make(map[phaseType][]func(context.Context)),
		ctx:      make(map[phaseType]context.Context),
		cancelF:  make(map[phaseType]context.CancelFunc),
		previous: -1,
	}
}

func (ps *phaseEvents) On(phase phaseType, f func(context.Context)) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.on[phase] = append(ps.on[phase], f)

	if _, ok := ps.ctx[phase]; !ok {
		ctx, cancel := context.WithCancel(context.Background())
		ps.ctx[phase] = ctx
		ps.cancelF[phase] = cancel
	}
}

// func (ps *phaseEvents) Once(phase phaseType, f func(context.Context)) {
// 	ps.mtx.Lock()
// 	defer ps.mtx.Unlock()

// 	if _, ok := ps.ctx[phase]; !ok {
// 		ctx, cancel := context.WithCancel(context.Background())
// 		ps.ctx[phase] = ctx
// 		ps.cancelF[phase] = cancel
// 	}

// 	if ps.previous == phase {
// 		go f(ps.ctx[phase])
// 		return
// 	}

// 	ps.once[phase] = append(ps.once[phase], f)
// }

func (ps *phaseEvents) Last() phaseType {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return ps.previous
}

func (ps *phaseEvents) Publish(phase phaseType) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.previous = phase

	for _, v := range ps.on[phase] {
		go v(ps.ctx[phase])
	}

	// for _, v := range ps.once[phase] {
	// 	go v(ps.ctx[phase])
	// }

	// delete(ps.once, phase)
}

func (ps *phaseEvents) Cancel(phases ...phaseType) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	for _, phase := range phases {

		if cancel, ok := ps.cancelF[phase]; ok {
			cancel()
		}

		ctx, cancel := context.WithCancel(context.Background())
		ps.ctx[phase] = ctx
		ps.cancelF[phase] = cancel
	}
}

func (ps *phaseEvents) Close() {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	for k, cancel := range ps.cancelF {
		cancel()
		delete(ps.cancelF, k)
	}

	for k := range ps.ctx {
		delete(ps.cancelF, k)
	}

	for k := range ps.on {
		delete(ps.cancelF, k)
	}
}
