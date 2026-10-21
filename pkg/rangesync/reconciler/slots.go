// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"container/list"
	"context"
	"sync"
)

// slotPool is a counting semaphore whose size can change while it is in use:
// the slots of the sessions this node opens, sized by the per-node cap
// (InitiatedSessionCap) at every round. acquire waits first come, first
// served. A smaller size takes no slot away: the holders keep theirs, and no
// waiter gets one until the slots in use are below the new size.
type slotPool struct {
	mu      sync.Mutex
	size    int
	used    int
	waiters list.List // of chan struct{}, closed when the slot is granted
}

func newSlotPool(size int) *slotPool {
	return &slotPool{size: size}
}

// acquire takes a slot, waiting for one until ctx ends. release frees it; it
// must be called once.
func (p *slotPool) acquire(ctx context.Context) (release func(), err error) {
	p.mu.Lock()
	if p.used < p.size && p.waiters.Len() == 0 {
		p.used++
		p.mu.Unlock()
		return p.release, nil
	}
	ready := make(chan struct{})
	elem := p.waiters.PushBack(ready)
	p.mu.Unlock()

	select {
	case <-ready:
		return p.release, nil
	case <-ctx.Done():
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-ready: // granted while ctx ended: give the slot to the next waiter
		p.used--
	default:
		p.waiters.Remove(elem)
	}
	p.grantLocked()
	return nil, context.Cause(ctx)
}

// release frees a slot taken by acquire.
func (p *slotPool) release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used--
	p.grantLocked()
}

// resize sets the number of slots to size, granting slots to the waiters it
// lets in.
func (p *slotPool) resize(size int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.size = size
	p.grantLocked()
}

// grantLocked grants free slots to the waiters, in the order they came.
// p.mu must be held.
func (p *slotPool) grantLocked() {
	for p.used < p.size && p.waiters.Len() > 0 {
		elem := p.waiters.Front()
		p.waiters.Remove(elem)
		p.used++
		close(elem.Value.(chan struct{}))
	}
}

// state returns the size and the slots in use.
func (p *slotPool) state() (size, used int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.size, p.used
}
