// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
)

const minBatchSize = 500

// Reseerve should move under postage pkg
type Reserve interface {
	SubscribeToDepth() (sub <-chan uint8, cancel func())
	SubscribeToExpiredBatches() (sub <-chan []byte, cancel func())
}

// reserve is the localstore component that manages the fate of chunks
// dictated by the postage batchstore reserve based on a priority ordering of batches
type reserve struct {
	db     *DB
	depth  uint8
	mu     sync.RWMutex
	cancel func()
}

// NewReserve
func newReserve(db *DB, res Reserve) *reserve {
	stateSub, stateCancel := res.SubscribeToDepth()
	batchSub, batchCancel := res.SubscribeToExpiredBatches()
	cancel := func() {
		stateCancel()
		batchCancel()
	}
	r := &reserve{
		db:     db,
		cancel: cancel,
	}
	go r.unreserveBins(stateSub)
	go r.unreserveBatches(batchSub)
	return r
}

// Close terminates the async forever loops
func (r *reserve) Close() error {
	r.cancel()
	return nil
}

// WithinRadius returns true iff the given address is within the nodes area of responsibility
func (r *reserve) WithinRadius(addr []byte) bool {
	return swarm.Proximity(r.db.baseKey, addr) < r.getDepth()
}

// unreserveBins is called as an async forever loop
// whenever there is a depth change, the reserve unpins all chunks with oldDepth<=po<newDepth
// if only the reserve pinned the chunk, it will automatically appear in the GC index
func (r *reserve) unreserveBins(sub <-chan uint8) {
	for newDepth := range sub {
		oldDepth := r.setDepth(newDepth)
		for po := newDepth; po < oldDepth; po++ {
			r.unreserveBin(po)
		}
	}
}

// unreserveBatches is called as an async forever loop
// whenever a batch expires, its chunks are unpinned
// if only the reserve pinned the chunk, it will automatically appear in the GC index
func (r *reserve) unreserveBatches(sub <-chan []byte) {
	// sub is clossed when the subscription is cancelled
	// then the go routine terminates
	for b := range sub {
		// TODO: handle error
		_ = r.db.postage.unreserveBatch(b, swarm.MaxPO)
	}
}

// unreserveBin subscribes to the pull index for the bin and
// unpins all chunks in the bin
// in order to avoid race on pinning, chunks of batches belonging to po-s are atomically unpinned
func (r *reserve) unreserveBin(bin uint8) {
	free := make(chan struct{}, 1)
	lock := struct{}{}
	var lockc chan struct{}
	var batches map[string]bool
	ctx := context.Background()
	// until must be given so that the channel is closed
	// by the time this is called depth is already  set so new chunks in these bins will not be reserved
	until, err := r.db.LastPullSubscriptionBinID(bin)
	if err != nil {
		panic(err)
	}
	sub, _, stop := r.db.SubscribePull(ctx, bin, 0, until)
	// the goroutine terminates when sub channel is closed by cancelling the subscription
	// closes the pull subscription too
	defer stop()
	for {
		select {
		case item, more := <-sub:
			if !more {
				sub = nil
			}
			batches[string(item.BatchID)] = true
			// when collected enough batches or we run out of chunks, allow unreserve
			// note: since sub is not blocking, this does not delay unreserving bins
			if len(batches) == minBatchSize || sub == nil {
				lockc = free
			}
		case <-lockc:
			go func() {
				for batchID := range batches {
					// handle error
					_ = r.db.postage.unreserveBatch([]byte(batchID), bin)
				}
				free <- lock
			}()
			// terminate if sub finished
			if sub == nil {
				return
			}
			// reset the map
			batches = make(map[string]bool)
			// disable unreserve until enough batches and previous call returns
			lockc = nil
		}
	}
}

// getDepth is read accessor to depth
func (r *reserve) getDepth() uint8 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.depth
}

// setDepth is write accessor to depth
// returns old depth
func (r *reserve) setDepth(d uint8) uint8 {
	r.mu.Lock()
	defer r.mu.Unlock()
	oldDepth := r.depth
	r.depth = d
	return oldDepth
}
