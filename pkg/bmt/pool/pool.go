// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"hash"
	"sync"

	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

// Pooler pools bmt Hashers.
// It provides the ability for the number of hashers to grow
// according to demand, but will shrink once the minimum defined
// hashers are put back into the pool.
type Pooler interface {
	// Get a bmt Hasher instance.
	// Instances are reset before being returned to the caller.
	Get() *bmtlegacy.Hasher
	// Put a bmt Hasher back into the pool
	Put(*bmtlegacy.Hasher)
	// Size of the pool.
	Size() int
}

type pool struct {
	p       sync.Pool
	mtx     sync.Mutex
	minimum int // minimum number of instances the pool should have
	size    int // size of the pool (only accounted for when items are put back)
	rented  int // number of video tapes on rent
}

// New returns a new HasherPool.
func New(minPool, branches int) Pooler {
	return &pool{
		p: sync.Pool{
			New: func() interface{} {
				return bmtlegacy.New(bmtlegacy.NewTreePool(hashFunc, branches, 1)) // one tree per hasher
			},
		},
		minimum: minPool,
	}
}

// Get gets a bmt Hasher from the pool.
func (h *pool) Get() *bmtlegacy.Hasher {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	v := h.p.Get().(*bmtlegacy.Hasher)
	h.rented++

	if h.size > 0 {
		h.size--
	}

	return v
}

// Put puts a Hasher back into the pool.
// It discards the instance if the minimum number of instances
// has been reached.
// The hasher is reset before being put back into the pool.
func (h *pool) Put(v *bmtlegacy.Hasher) {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	h.rented--

	// only put back if we're not exceeding the minimum capacity
	if h.size+1 > h.minimum {
		return
	}

	v.Reset()
	h.p.Put(v)
	h.size++
}

// Size of the pool.
func (h *pool) Size() int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.size
}

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}
