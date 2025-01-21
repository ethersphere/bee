// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type inflight struct {
	mtx      sync.Mutex
	inflight map[[64]byte]struct{}
}

func newInflight() *inflight {
	return &inflight{
		inflight: make(map[[64]byte]struct{}),
	}
}

func (i *inflight) delete(idAddress swarm.Address, batchID []byte) {
	var key [64]byte
	copy(key[:32], idAddress.Bytes())
	copy(key[32:], batchID)

	i.mtx.Lock()
	delete(i.inflight, key)
	i.mtx.Unlock()
}

func (i *inflight) set(idAddress swarm.Address, batchID []byte) bool {
	var key [64]byte
	copy(key[:32], idAddress.Bytes())
	copy(key[32:], batchID)

	i.mtx.Lock()
	defer i.mtx.Unlock()

	if _, ok := i.inflight[key]; ok {
		return true
	}

	i.inflight[key] = struct{}{}
	return false
}

type attempts struct {
	mtx        sync.Mutex
	retryCount int
	attempts   map[string]int
}

// try to log a chunk sync attempt. returns false when
// maximum amount of attempts have been reached.
func (a *attempts) try(idAddress swarm.Address) bool {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	key := idAddress.ByteString()
	a.attempts[key]++
	return a.attempts[key] < a.retryCount
}

func (a *attempts) delete(idAddress swarm.Address) {
	a.mtx.Lock()
	delete(a.attempts, idAddress.ByteString())
	a.mtx.Unlock()
}
