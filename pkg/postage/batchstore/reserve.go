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
// allow  them  to be gargage collected.
//
// the rules of the reserve
// - if batch a is unreserved and val(b) <  val(a) then b is unreserved on any po
// - if a batch is unreserved on po p, then  it is unreserved also on any p'<p
// - batch size based on fully filled the reserve should not exceed Capacity
// - batch reserve is maximally utilised, i.e, cannot be extended and have 1-3 remain true

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
	// Available capacity of the reserve which can still be used.
	Available int64
}
type updateItem struct {
	item *valueItem
	key  string
}

// allocateBatch allocates a newly created batch.
// After computing a new radius, the available capacity of the node is deducted
// using the new batch's depth.
// Must be called under lock.
func (s *store) allocateBatch(b *postage.Batch) error {

	if err := s.store.Put(valueKey(b.Value, b.ID), &valueItem{StorageRadius: s.rs.StorageRadius}); err != nil {
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

	var tofullyEvict [][]byte

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {

		batchID := valueKeyToID(key)
		b, err := s.get(batchID)
		if err != nil {
			return false, err
		}

		// negative value batches
		if b.Value.Cmp(s.cs.TotalAmount) <= 0 {
			tofullyEvict = append(tofullyEvict, b.ID)
		} else {
			return true, nil // stop early as an optimization at first non-negative value
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	for _, id := range tofullyEvict {

		err := s.evictFn(id)
		if err != nil {
			return err
		}
		b, err := s.get(id)
		if err != nil {
			return err
		}
		err = s.store.Delete(valueKey(b.Value, id))
		if err != nil {
			return err
		}
		err = s.store.Delete(batchKey(id))
		if err != nil {
			return err
		}
	}

	return nil
}

// computeRadius calculates the radius by using the sum up all the number of chunks from all batches
// and the node capacity using the formula
// total_needed_capacity/node_capacity = 2^R.
// Must be called under lock.
func (s *store) computeRadius() error {

	var totalCommitment int64

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {
		batchID := valueKeyToID(key)
		b, err := s.get(valueKeyToID(key))
		if err != nil {
			return false, fmt.Errorf("compute radius %x: %w", batchID, err)
		}

		totalCommitment += exp2(uint(b.Depth))

		return false, nil
	})
	if err != nil {
		return err
	}

	if totalCommitment <= Capacity {
		s.rs.Radius = 0
		s.rs.Available = Capacity - totalCommitment
		return s.store.Put(reserveStateKey, s.rs)
	}

	// total_needed_capacity/node_capacity = 2^R
	// log2(total_needed_capacity/node_capacity) = R
	s.rs.Radius = uint8(math.Ceil(math.Log2(float64(totalCommitment) / float64(Capacity))))

	// Available = Capacity - total_needed_capacity/2^R
	s.rs.Available = int64(float64(Capacity) - (float64(totalCommitment) / math.Pow(2, float64(s.rs.Radius))))

	// fmt.Printf("calculated available %d %d %d\n", s.rs.Available, totalCommitment, s.rs.Radius)

	s.metrics.Radius.Set(float64(s.rs.Radius))

	if s.rs.StorageRadius > s.rs.Radius {
		s.rs.StorageRadius = s.rs.Radius
		s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
		err = s.lowerStorageRadius()
		if err != nil {
			s.logger.Warningf("batchstore: lower storage radius: %v", err)
		}
	}

	return s.store.Put(reserveStateKey, s.rs)
}

// Unreserve is implementation of postage.Storer interface Unreserve method.
func (s *store) Unreserve(cb postage.UnreserveIteratorFn) error {

	now := time.Now()
	defer func() {
		s.metrics.UnreserveDuration.Observe(time.Since(now).Seconds())
	}()

	s.mtx.Lock()
	defer s.mtx.Unlock()

	var (
		stopped  = false
		toUpdate []updateItem
	)

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (bool, error) {

		v := &valueItem{}
		err := v.UnmarshalBinary(value)
		if err != nil {
			return false, err
		}

		// skip eviction if previous eviction has higher radius
		if v.StorageRadius > s.rs.StorageRadius {
			return false, nil
		}

		id := valueKeyToID(key)

		stopped, err = cb(id, v.StorageRadius)
		if err != nil {
			return false, err
		}

		v.StorageRadius++

		toUpdate = append(toUpdate, updateItem{item: v, key: string(key)})

		return stopped, nil
	})
	if err != nil {
		return err
	}

	for _, u := range toUpdate {
		err := s.store.Put(u.key, u.item)
		if err != nil {
			s.logger.Warningf("batchstore: Unreserve: %v", err)
		}
	}

	// a full iteration has occurred, so more evictions from localstore may be necessary, increase storage radius
	if !stopped && s.rs.StorageRadius < s.rs.Radius {
		s.rs.StorageRadius++
		s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
		return s.store.Put(reserveStateKey, s.rs)
	}

	return nil
}

// lowerBatchRadius reduces the radius of batches if the current radius is lower than the batch radius.
// If the radius is lower, then the allocated capacity is not sufficient, so the batch radius is lowered.
// A lower batch radius means more allocated capacity.
// Must be called under lock.
func (s *store) lowerStorageRadius() error {

	var toUpdate []updateItem

	err := s.store.Iterate(valueKeyPrefix, func(key, val []byte) (bool, error) {

		v := &valueItem{}
		err := v.UnmarshalBinary(val)
		if err != nil {
			return false, err
		}

		if s.rs.StorageRadius < v.StorageRadius {
			v.StorageRadius = s.rs.StorageRadius
			toUpdate = append(toUpdate, updateItem{key: string(key), item: v})
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	for _, u := range toUpdate {
		err := s.store.Put(u.key, u.item)
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
