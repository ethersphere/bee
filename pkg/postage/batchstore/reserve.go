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
	"math/big"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// DefaultDepth is the initial depth for the reserve
var DefaultDepth = uint8(12) // 12 is the testnet depth at the time of merging to master

// Capacity is the number of chunks in reserve. `2^22` (4194304) was chosen to remain
// relatively near the current 5M chunks ~25GB.
var Capacity = exp2(22)

// fullEvictionRadius is a PO value used to fully dealloacte a batch.
const fullEvictionRadius = swarm.MaxPO + 1

// reserveState records the state and is persisted in the state store
type reserveState struct {
	// Radius is the Radius of responsibility,
	// it defines the proximity order of chunks which we
	// would like to guarantee that all chunks are stored
	Radius uint8
	// StorageRadius is the de-facto storage radius tracked
	// by monitoring the events communicated to the localstore
	// reserve eviction worker.
	StorageRadius uint8
	// Available capacity of the reserve which can still be used.
	Available int64
}

// allocateBatch is the main point of entry for a new batch.
// After computing a new radius, the available capacity of the node is deducted
// using the new batch's depth.
// Must be called under the mutex lock.
func (s *store) allocateBatch(b *postage.Batch) error {

	err := s.cleanup()
	if err != nil {
		return fmt.Errorf("batchstore: allocate batch cleanup %x %w", b.ID, err)
	}

	err = s.adjustRadius(exp2(uint(b.Depth)))
	if err != nil {
		return fmt.Errorf("batchstore: allocate batch adjust radius %x %w", b.ID, err)
	}

	if err := s.putValueItem(b.ID, b.Value, s.rs.Radius, 0); err != nil {
		return fmt.Errorf("batchstore: allocate batch %x %w", b.ID, err)
	}

	capacity := exp2(uint(b.Depth) - uint(s.rs.Radius))
	s.rs.Available -= capacity

	s.metrics.AvailableCapacity.Set(float64(s.rs.Available))

	err = s.gainCapacity(b.Value)
	if err != nil {
		return fmt.Errorf("batchstore: allocate batch gain capacity %x %w", b.ID, err)
	}

	return nil
}

// deallocateBatch unreserves a batch fully to regain previously allocated capacity.
// Must be called under the mutex lock.
func (s *store) deallocateBatch(b *postage.Batch) error {

	v, err := s.getValueItem(b)
	if err != nil {
		return err
	}

	err = s.adjustCapacity(b, v, fullEvictionRadius)
	if err != nil {
		return fmt.Errorf("batchstore: deallocate batch adjust capacity %x %w", b.ID, err)
	}

	return nil
}

// cleanup evicts and removes negative value batches.
// Must be called under the mutex lock.
func (s *store) cleanup() error {

	var tofullyEvict [][]byte

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {

		batchID := valueKeyToID(key)
		b, err := s.get(batchID)
		if err != nil {
			return false, err
		}

		v := &valueItem{}
		err = v.UnmarshalBinary(value)
		if err != nil {
			return false, err
		}

		// negative value batches
		if b.Value.Cmp(s.cs.TotalAmount) <= 0 {
			err := s.adjustCapacity(b, v, fullEvictionRadius)
			if err != nil {
				return false, err
			}
			tofullyEvict = append(tofullyEvict, b.ID)
		} else {
			return true, nil // stop early as an optimization at first non-negative value
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	return s.evict(tofullyEvict)
}

// Must be called under the mutex lock.
func (s *store) adjustRadius(newBatch int64) error {

	oldRadius := s.rs.Radius

	err := s.computeRadius(newBatch)
	if err != nil {
		return err
	}

	err = s.store.Put(reserveStateKey, s.rs)
	if err != nil {
		return err
	}

	s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
	s.metrics.Radius.Set(float64(s.rs.Radius))

	// when batches expire, radius may decrease, so adjust value index accordingly
	if s.rs.Radius < oldRadius {
		// if radius is lower than storage radius, override
		if s.rs.StorageRadius > s.rs.Radius {
			s.rs.StorageRadius = s.rs.Radius
		}
		return s.lowerBatchRadius()
	}

	return nil
}

// gainCapacity iterates on the list of batches in ascending order of value and unreserves batches with the new radius
// until a positive node capacity is reached.
// Must be called under the mutex lock.
func (s *store) gainCapacity(upto *big.Int) error {

	if s.rs.Available >= 0 {
		return nil
	}

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {

		batchID := valueKeyToID(key)
		b, err := s.get(batchID)
		if err != nil {
			return false, err
		}

		v := &valueItem{}
		err = v.UnmarshalBinary(value)
		if err != nil {
			return false, err
		}

		// adjust capacity until positive available AND until the last added batch's value
		if s.rs.Available >= 0 && b.Value.Cmp(upto) >= 0 {
			return false, nil
		}

		err = s.adjustCapacity(b, v, s.rs.Radius)
		if err != nil {
			return false, err
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	return nil
}

// adjustCapacity adds extra capacity to the node by tweaking a batches radius based on a new radius.
// Must be called under the mutex lock.
func (s *store) adjustCapacity(b *postage.Batch, v *valueItem, radius uint8) error {

	_, change := s.capacity(b.Depth, v.Radius, radius)

	err := s.putValueItem(b.ID, b.Value, radius, v.StorageRadius)
	if err != nil {
		return err
	}

	s.rs.Available += change

	return nil
}

// capacity returns the new capacity and old capacity dedicated to a batch given the new radius.
// Must be called under the mutex lock.
func (s *store) capacity(depth, batchRadius, radius uint8) (int64, int64) {

	var (
		newCapacity int64
		oldCapacity int64
	)

	if depth > radius {
		newCapacity = exp2(uint(depth - radius))
	}

	// if eviction radius is greater than the depth of the batch, no capacity is reserved for the batch, old capacity should be zero
	if depth > batchRadius {
		oldCapacity = exp2(uint(depth - batchRadius))
	}

	return newCapacity, oldCapacity - newCapacity
}

// lowerBatchRadius reduces the radius of batches if the current radius is lower than the batch radius.
// If the radius is lower, then the allocated capacity is not sufficient, so the batch radius is lowered.
// A lower batch radius means more allocated capacity.
// Must be called under the mutex lock.
func (s *store) lowerBatchRadius() error {

	type updateValueItem struct {
		id   []byte
		item *valueItem
	}

	var toUpdate []updateValueItem

	defer func() {
		for _, v := range toUpdate {
			b, err := s.get(v.id)
			if err != nil {
				s.logger.Warningf("lowerEvictionRadius: %v", err)
			} else {
				err := s.adjustCapacity(b, v.item, s.rs.Radius)
				if err != nil {
					s.logger.Warningf("lowerEvictionRadius: %v", err)
				}
			}
		}
	}()

	return s.store.Iterate(valueKeyPrefix, func(key, val []byte) (bool, error) {

		id := valueKeyToID(key)

		v := &valueItem{}
		err := v.UnmarshalBinary(val)
		if err != nil {
			return false, err
		}

		if s.rs.StorageRadius < v.StorageRadius {
			v.StorageRadius = s.rs.StorageRadius
			toUpdate = append(toUpdate, updateValueItem{item: v, id: id})
		} else if s.rs.Radius < v.Radius {
			toUpdate = append(toUpdate, updateValueItem{item: v, id: id})
		}

		return false, nil
	})
}

// computeRadius calculates the radius by using the sum up all the number of chunks from all batches
// and the node capacity using the formula
// total_needed_capacity/node_capacity = 2^R.
// Must be called under the mutex lock.
func (s *store) computeRadius(newBatch int64) error {

	var totalCommitment int64 = newBatch

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {
		batchID := valueKeyToID(key)
		b, err := s.get(valueKeyToID(key))
		if err != nil {
			return false, fmt.Errorf("compute radius %x %v: %w", batchID, b, err)
		}

		totalCommitment += exp2(uint(b.Depth))

		return false, nil
	})
	if err != nil {
		return err
	}

	// total_needed_capacity/node_capacity = 2^R
	// log2(total_needed_capacity/node_capacity) = R
	s.rs.Radius = uint8(math.Ceil(math.Log2(float64(totalCommitment) / float64(Capacity))))

	return nil
}

// delete calls the evict callback and removes the batches with ids given as arguments.
func (s *store) evict(ids [][]byte) error {
	for _, id := range ids {

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

// Unreserve is implementation of postage.Storer interface Unreserve method.
func (s *store) Unreserve(cb postage.UnreserveIteratorFn) error {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	type updateItem struct {
		item *valueItem
		key  string
	}

	var (
		stopped  = false
		toUpdate []updateItem
	)

	defer func() {
		for _, u := range toUpdate {
			err := s.store.Put(u.key, u.item)
			if err != nil {
				s.logger.Warningf("Unreserve: %v", err)
			}
		}
	}()

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

	// a full iteration has occurred, so more evictions from localstore may be necessary, increase storage radius
	if !stopped && s.rs.StorageRadius < s.rs.Radius {
		s.rs.StorageRadius++
		s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
		return s.store.Put(reserveStateKey, s.rs)
	}

	return nil
}

// exp2 returns the e-th power of 2
func exp2(e uint) int64 {
	return 1 << e
}
