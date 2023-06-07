// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package events

import (
	"sync"
)

type Subscriber struct {
	mtx  sync.Mutex
	subs map[string][]chan struct{}
}

func NewSubscriber() *Subscriber {
	return &Subscriber{
		subs: make(map[string][]chan struct{}),
	}
}

func (b *Subscriber) Subscribe(str string) (<-chan struct{}, func()) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	c := make(chan struct{}, 1)
	b.subs[str] = append(b.subs[str], c)

	return c, func() {
		b.mtx.Lock()
		defer b.mtx.Unlock()

		for i, s := range b.subs[str] {
			if s == c {
				b.subs[str][i] = nil
				b.subs[str] = append(b.subs[str][:i], b.subs[str][i+1:]...)
				break
			}
		}
	}
}

func (b *Subscriber) Trigger(str string) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	for _, s := range b.subs[str] {
		select {
		case s <- struct{}{}:
		default:
		}
	}
}
