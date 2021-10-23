// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

type inflight struct {
	mtx      sync.Mutex
	inflight map[string]struct{}
	slots    chan struct{}
}

func newInflight() *inflight {
	return &inflight{
		inflight: make(map[string]struct{}),
		slots:    make(chan struct{}, concurrentPushes),
	}
}

func (i *inflight) delete(ch swarm.Chunk) {
	i.mtx.Lock()
	delete(i.inflight, ch.Address().ByteString())
	i.mtx.Unlock()
	<-i.slots
}

func (i *inflight) set(addr []byte) bool {
	i.mtx.Lock()
	key := string(addr)
	if _, ok := i.inflight[key]; ok {
		i.mtx.Unlock()
		return true
	}
	i.inflight[key] = struct{}{}
	i.mtx.Unlock()
	i.slots <- struct{}{}
	return false
}

type attempts struct {
	mtx      sync.Mutex
	attempts map[string]int
}

// try to log a chunk sync attempt. returns false when
// maximum amount of attempts have been reached.
func (a *attempts) try(ch swarm.Address) bool {
	key := ch.ByteString()
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.attempts[key]++
	if a.attempts[key] == retryCount {
		delete(a.attempts, key)
		return false
	}
	return true
}
