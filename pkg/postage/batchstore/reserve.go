// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package batchstore implements the reserve
// the reserve serves to maintain chunks in the area of responsibility
// it has two components
// - the batchstore reserve which maintains information about batches, their values, priorities and synchronises with the blockchain
// - the localstore which stores chunks and manages garbage collection
//
// when a new chunk arrives in the localstore, the batchstore reserve is asked to check
// the batch used in the postage stamp attached to the chunk.
// Depending on the value of the batch (reserve depth of the batch), the localstore
// either pins the chunk (thereby protecting it from garbage collection) or not.
// the chunk stays pinned until it is 'unreserved' based on changes in relative priority of the batch it belongs to
//
// the atomic db operation is unreserving a batch down to a depth
// the intended semantics of unreserve is to unpin the chunks
// in the relevant POs, belonging to the batch and (unless they are otherwise pinned)
// allow them to be gargage collected.
//
// the batchstore keeps track of a radius such that the total number of chunks
// with POs above the radius covers the entire capacity.
//
// the rules of the reserve
// - if batch a is unreserved and val(b) <  val(a) then b is unreserved on any po
// - if a batch is unreserved on po p, then it is unreserved also on any p'<p
// - batch size based on fully filled the reserve should not exceed Capacity
// - batch reserve is maximally utilised, i.e, cannot be extended and have 1-3 remain true
// - radius is based on maximum batch utilization, as such, storage radius cannot exceed radius

package batchstore

import (
	"fmt"
	"math"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
)

// Capacity is the number of chunks in reserve. `2^22` (4194304) was chosen to remain
// relatively near the current 5M chunks ~25GB.
var Capacity = exp2(22)

// reserveState records the state and is persisted in the state store
type reserveState struct {
	// Radius is the Radius of responsibility,
	// it defines the proximity order of chunks which we
	// would like to guarantee that all chunks are stored.
	Radius uint8
	// StorageRadius is the de-facto storage radius tracked
	// by monitoring the events communicated to the localstore
	// reserve eviction worker.
	StorageRadius uint8
}

// saveBatch adds a new batch to the batchstore by creating a new value item, cleaning up
// expired batches, and computing a new radius.
// Must be called under lock.
func (s *store) saveBatch(b *postage.Batch) error {

	if err := s.store.Put(valueKey(b.Value, b.ID), nil); err != nil {
		return fmt.Errorf("batchstore: allocate batch %x: %w", b.ID, err)
	}

	err := s.cleanup()
	if err != nil {
		return fmt.Errorf("batchstore: allocate batch cleanup %x: %w", b.ID, err)
	}

	err = s.computeRadius()
	if err != nil {
		return fmt.Errorf("batchstore: allocate batch adjust radius %x: %w", b.ID, err)
	}

	return nil
}

// cleanup evicts and removes expired batch.
// Must be called under lock.
func (s *store) cleanup() error {

	var evictions []*postage.Batch

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {

		b, err := s.get(valueKeyToID(key))
		if err != nil {
			return false, err
		}

		// batches whose balance is below the total cumulative payout
		if b.Value.Cmp(s.cs.TotalAmount) <= 0 {
			evictions = append(evictions, b)
		} else {
			return true, nil // stop early as an optimization at first value above the total cumulative payout
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	for _, b := range evictions {

		s.logger.Debugf("batchstore: cleaning up batch %x", b.ID)

		err := s.evictFn(b.ID)
		if err != nil {
			return err
		}
		err = s.store.Delete(valueKey(b.Value, b.ID))
		if err != nil {
			return err
		}
		err = s.store.Delete(batchKey(b.ID))
		if err != nil {
			return err
		}
	}

	return nil
}

// computeRadius calculates the radius by using the sum of all batch depths
// and the node capacity using the formula totalCommitment/node_capacity = 2^R.
// In the case that the new radius is lower than the current storage radius,
// batch storage radiuses are adjusted to the new radius.
// Must be called under lock.
func (s *store) computeRadius() error {

	var totalCommitment int64

	err := s.store.Iterate(batchKeyPrefix, func(key, value []byte) (bool, error) {

		b := &postage.Batch{}
		if err := b.UnmarshalBinary(value); err != nil {
			return false, err
		}

		totalCommitment += exp2(uint(b.Depth))

		return false, nil
	})
	if err != nil {
		return err
	}

	// edge case where the sum of all batches is below the node capacity.
	if totalCommitment <= Capacity {
		s.rs.Radius = 0
	} else {
		// totalCommitment/node_capacity = 2^R
		// log2(totalCommitment/node_capacity) = R
		s.rs.Radius = uint8(math.Ceil(math.Log2(float64(totalCommitment) / float64(Capacity))))
	}

	s.metrics.Radius.Set(float64(s.rs.Radius))

	// if the new radius is lower than the storage radius, adjust the storage radius.
	// also, lower the storage radius of each batch to the new radius to prevent higher than radius POs
	// from being evicted when localstore calls the Unreserve.
	if s.rs.StorageRadius > s.rs.Radius {
		s.rs.StorageRadius = s.rs.Radius
		s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
		if err = s.lowerStorageRadius(); err != nil {
			s.logger.Warningf("batchstore: lower storage radius: %v", err)
		}
	}

	s.logger.Debugf("batchstore: computed radius %d", s.rs.Radius)

	return s.store.Put(reserveStateKey, s.rs)
}

// Unreserve is implementation of postage.Storer interface Unreserve method.
func (s *store) Unreserve(cb postage.UnreserveIteratorFn) error {

	defer func(t time.Time) {
		s.metrics.UnreserveDuration.WithLabelValues("true").Observe(time.Since(t).Seconds())
	}(time.Now())

	s.mtx.Lock()
	defer s.mtx.Unlock()

	defer func(t time.Time) {
		s.metrics.UnreserveDuration.WithLabelValues("false").Observe(time.Since(t).Seconds())
	}(time.Now())

	var (
		stopped = false
		updates []*postage.Batch
	)

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (bool, error) {

		id := valueKeyToID(key)

		b, err := s.get(id)
		if err != nil {
			return false, err
		}

		// skip eviction and try the next batch if the batch storage radius is higher than
		// the global storage radius.
		if b.StorageRadius > s.rs.StorageRadius {
			return false, nil
		}

		s.logger.Debugf("batchstore: Unreserve callback batch %x storage radius %d", id, b.StorageRadius)

		stopped, err = cb(id, b.StorageRadius)
		if err != nil {
			return false, err
		}

		// each call of Unreseve means that the eviction of chunks is required to recover storage space, as such,
		// the storage radius of the batch is inreased so that future Unreserve calls will
		// evict higher PO chunks of the batch. When the storage radius of a batch becomes higher than
		// the global storage radius, other batches are attempted for eviction.
		b.StorageRadius++

		updates = append(updates, b)

		return stopped, nil
	})
	if err != nil {
		return err
	}

	for _, u := range updates {
		err := s.store.Put(batchKey(u.ID), u)
		if err != nil {
			s.logger.Warningf("batchstore: Unreserve: %v", err)
		}
	}

	// a full iteration has occurred, so more evictions from localstore may be necessary, increase storage radius
	if !stopped && s.rs.StorageRadius < s.rs.Radius {
		s.rs.StorageRadius++
		s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
		s.logger.Debugf("batchstore: new storage radius %d ", s.rs.StorageRadius)
		return s.store.Put(reserveStateKey, s.rs)
	}

	return nil
}

// lowerStorageRadius lowers the storage radius of batches to the radius.
// radius is based on maximum batch utilization, as such, batch storage radius cannot exceed the radius.
// Must be called under lock.
func (s *store) lowerStorageRadius() error {

	var updates []*postage.Batch

	err := s.store.Iterate(batchKeyPrefix, func(key, value []byte) (bool, error) {

		b := &postage.Batch{}
		err := b.UnmarshalBinary(value)
		if err != nil {
			return false, err
		}

		if b.StorageRadius > s.rs.StorageRadius {
			b.StorageRadius = s.rs.StorageRadius
			s.logger.Debugf("batchstore: lowering storage radius for batch %x from %d to %d", b.ID, b.StorageRadius, s.rs.StorageRadius)
			updates = append(updates, b)
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	for _, u := range updates {
		err := s.store.Put(batchKey(u.ID), u)
		if err != nil {
			s.logger.Warningf("batchstore: lower eviction radius: %v", err)
		}
	}

	return nil
}

// exp2 returns the e-th power of 2
func exp2(e uint) int64 {
	return 1 << e
}
