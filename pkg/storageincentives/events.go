// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"sync"
)

type PhaseType int

const (
	commit PhaseType = iota + 1
	reveal
	claim
	sample
	phaseCount
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
	mtx    sync.Mutex
	events [phaseCount]event
	quit   [phaseCount]chan struct{}
}

type event []func(quit chan struct{}, blockNumber uint64)

func NewEvents() *events {
	return &events{}
}

func (e *events) On(phase PhaseType, f func(quit chan struct{}, blockNumber uint64)) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	fs := e.events[phase]
	if len(fs) == 0 {
		e.quit[phase] = make(chan struct{})
	}
	e.events[phase] = append(fs, f)
}

func (e *events) Publish(phase PhaseType, blockNumber uint64) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	for _, v := range e.events[phase] {
		go v(e.quit[phase], blockNumber)
	}
}

func (e *events) Cancel(phases ...PhaseType) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	for _, phase := range phases {
		close(e.quit[phase])
		e.quit[phase] = make(chan struct{})
	}
}

func (e *events) Close() {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	for _, ch := range e.quit {
		close(ch)
	}
}
