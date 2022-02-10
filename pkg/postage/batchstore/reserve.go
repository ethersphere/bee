// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package batchstore implements the reserve
// the reserve serves to maintain chunks in the area of responsibility
// it has two components
// -  the batchstore reserve which maintains information about batches, their values, priorities and synchronises with the blockchain
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
// - if a batch is unreserved on po  p, then  it is unreserved also on any p'<p
// - batch size based on fully filled the reserve should not  exceed Capacity
// - batch reserve is maximally utilised, i.e, cannot be extended and have 1-3 remain true

/*

Notes:
	if batch depth < radius, batch is fully unreserved

*/

package batchstore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// DefaultDepth is the initial depth for the reserve
var DefaultDepth = uint8(12) // 12 is the testnet depth at the time of merging to master

// Capacity is the number of chunks in reserve. `2^22` (4194304) was chosen to remain
// relatively near the current 5M chunks ~25GB.
var Capacity = exp2(22)

// UnreserveRadius is a flag to indicate that a batch may be fully unreserved.
const UnreserveRadius = swarm.MaxPO + 1

var errLowDepth = errors.New("batch depth is lower than node radius")

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
func (s *store) allocateBatch(b *postage.Batch) error {

	err := s.cleanup()
	if err != nil {
		fmt.Println("cleanup error")
		return err
	}

	if b.Depth < s.rs.Radius {
		return fmt.Errorf("allocate batch %s: %w", hex.EncodeToString(b.ID), errLowDepth)
	}

	fmt.Println("new radius", s.rs.Radius)

	// add eviction item and set to radius
	err = s.putUnreserveItem(&UnreserveItem{BatchID: b.ID, Radius: s.rs.Radius})
	if err != nil {
		fmt.Println("putUnreserve error")
		return err
	}

	capacity := exp2(uint(b.Depth) - uint(s.rs.Radius))
	s.rs.Available -= capacity

	fmt.Printf("reserved capacity %d, available %d %s\n ", capacity, s.rs.Available, hex.EncodeToString(b.ID))

	return s.evictForCapacity(b.Value)
}

// deallocateBatch unreserves a batch fully to regain previously allocated capacity.
func (s *store) deallocateBatch(b *postage.Batch) error {

	err := s.unreserveFn(b, UnreserveRadius)
	if err != nil {
		return err
	}

	fmt.Printf("deallocate change available %d %s\n", s.rs.Available, hex.EncodeToString(b.ID))

	return nil
}

// cleanup removes negative value batches, computes a new radius, and lowers batch evictions if new radius is lower.
func (s *store) cleanup() error {

	var toDelete [][]byte

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {

		batchID := valueKeyToID(key)
		b, err := s.get(batchID)
		if err != nil {
			return true, fmt.Errorf("release get %x %v: %w", batchID, b, err)
		}

		// negative value batches
		if b.Value.Cmp(s.cs.TotalAmount) <= 0 {
			fmt.Println("clean up", hex.EncodeToString(batchID))
			err := s.deallocateBatch(b)
			if err != nil {
				return false, err
			}
			toDelete = append(toDelete, b.ID)
		}

		return false, nil
	})
	if err != nil {
		return err
	}

	err = s.delete(toDelete...)
	if err != nil {
		return err
	}

	oldRadius := s.rs.Radius

	err = s.computeRadius()
	if err != nil {
		return err
	}

	if s.rs.Radius < oldRadius {
		return s.lowerEvictionRadius(s.rs.Radius)
	}

	return nil
}

// evictForCapacity iterates on the list of batches in ascending order of value and unreserves batches with the new radius
// until a positive node capacity is reached.
func (s *store) evictForCapacity(upto *big.Int) error {

	if s.rs.Available >= 0 {
		return nil
	}

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {

		batchID := valueKeyToID(key)
		b, err := s.get(batchID)
		if err != nil {
			return true, fmt.Errorf("release get %x %v: %w", batchID, b, err)
		}

		// adjust evictions until positive available AND until the last added batch's value
		if s.rs.Available >= 0 && b.Value.Cmp(upto) >= 0 {
			return true, nil
		}

		err = s.unreserveFn(b, s.rs.Radius)
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

// unreserve adds extra capacity to the node by tweaking a batches eviction based on a new radius.
func (s *store) unreserve(b *postage.Batch, evictionRadius uint8) error {

	item, err := s.getUnreserveItem(b.ID)
	if err != nil {
		return err
	}

	fmt.Printf("unreserve %s item radius %d eviction %d\n", hex.EncodeToString(b.ID), item.Radius, evictionRadius)

	_, change := s.capacity(b, item, evictionRadius)

	// more POs to evict
	if evictionRadius > item.Radius {
		item.Evicted = false
	}

	// if eviction is tagged with full unreserve, do not override
	if item.Radius != UnreserveRadius {
		item.Radius = evictionRadius
	}

	s.rs.Available += change

	return s.putUnreserveItem(item)
}

// capacity returns the new capacity and old capacity dedicated to a batch given the new eviction radius.
func (s *store) capacity(b *postage.Batch, item *UnreserveItem, evictionRadius uint8) (int64, int64) {

	var (
		newCapacity int64
		oldCapacity int64
	)

	if b.Depth > evictionRadius {
		newCapacity = exp2(uint(b.Depth - evictionRadius))
	}

	// if eviction radius is greater than the depth of the batch, no capacity is reserved for the batch, old capacity should be zero
	if b.Depth > item.Radius {
		oldCapacity = exp2(uint(b.Depth - item.Radius))
	}

	return newCapacity, oldCapacity - newCapacity
}

// lowerEvictionRadius reduces the eviction radius of batches if the current radius is lower than the eviction radius
// if an eviction radius is lower, than the eviction is too aggresive, and is tweaked down.
func (s *store) lowerEvictionRadius(evictionRadius uint8) error {

	var toUpdate []*UnreserveItem

	defer func() {
		for _, v := range toUpdate {
			b, err := s.get(v.BatchID)
			if err != nil {
				fmt.Println(err)
				s.logger.Warning(err)
			} else {
				err := s.unreserveFn(b, evictionRadius)
				if err != nil {
					s.logger.Warning(err)
				}
			}
		}
	}()

	return s.store.Iterate(unreservePrefix, func(key, val []byte) (bool, error) {
		if !strings.HasPrefix(string(key), unreservePrefix) {
			return true, nil
		}
		v := &UnreserveItem{}
		err := v.UnmarshalBinary(val)
		if err != nil {
			return false, err
		}

		if v.Radius != UnreserveRadius && evictionRadius < v.Radius {
			fmt.Println("~ ~ ~ ~ ~to update ~ ~ ~ ~  ~")
			toUpdate = append(toUpdate, v)
		}

		return false, nil
	})
}

// computeRadius calculates the radius by using the sum up all the number of chunks from all batches
// and the node capacity using the formula
// total_needed_capacity/node_capacity = 2^R .
func (s *store) computeRadius() error {

	var totalCommitment int64

	err := s.store.Iterate(valueKeyPrefix, func(key, value []byte) (stop bool, err error) {
		batchID := valueKeyToID(key)
		b, err := s.get(valueKeyToID(key))
		if err != nil {
			return true, fmt.Errorf("compute radius %x %v: %w", batchID, b, err)
		}

		totalCommitment += exp2(uint(b.Depth))

		// 2^R + 2^R + 2^R + 2^R.... /
		// t/2, t/4, t/8, t/16,
		// 500, 250, 125, 62.5, 31, 15, 7, 3, 2, 1,0
		return false, nil
	})
	if err != nil {
		return err
	}

	fmt.Println("compute radius total capacity", totalCommitment)

	// total_needed_capacity/node_capacity = 2^R
	// log2(total_needed_capacity/node_capacity) = R
	s.rs.Radius = uint8(math.Ceil(math.Log2(float64(totalCommitment) / float64(Capacity))))

	return nil
}

// Unreserve is implementation of postage.Storer interface Unreserve method.
// The UnreserveIteratorFn is used to delete the chunks related to evicted batches.
func (s *store) Unreserve(cb postage.UnreserveIteratorFn) error {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	fmt.Println("localstore: Unreserve")

	var (
		delete  []string
		evicted []*UnreserveItem
	)

	err := s.store.Iterate(unreservePrefix, func(key, val []byte) (bool, error) {
		if !strings.HasPrefix(string(key), unreservePrefix) {
			return true, nil
		}
		v := &UnreserveItem{}
		err := v.UnmarshalBinary(val)
		if err != nil {
			return true, err
		}

		// skip if eviction has already been called previously
		if v.Evicted {
			return false, nil
		}

		stop, err := cb(v.BatchID, v.Radius)
		if err != nil {
			return true, err
		}

		// delete from eviction index since the entire batch has been marked for deletion
		if v.Radius == UnreserveRadius {
			delete = append(delete, string(key))
		} else {
			evicted = append(evicted, v)
		}

		return stop, nil
	})
	if err != nil {
		return err
	}

	for _, item := range evicted {
		item.Evicted = true
		if err := s.putUnreserveItem(item); err != nil {
			s.logger.Warning(err)
		}
	}

	for _, key := range delete {
		if err := s.store.Delete(key); err != nil {
			s.logger.Warning(err)
		}
	}

	s.rs.StorageRadius = s.rs.Radius
	s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
	if err = s.store.Put(reserveStateKey, s.rs); err != nil {
		return err
	}

	return err
}

func (s *store) putUnreserveItem(item *UnreserveItem) error {
	return s.store.Put(unreserveKey(item.BatchID), item)
}

func (s *store) getUnreserveItem(id []byte) (*UnreserveItem, error) {
	item := &UnreserveItem{}
	err := s.store.Get(unreserveKey(id), item)
	return item, err
}

type UnreserveItem struct {
	BatchID []byte
	Radius  uint8

	// is this necessary??? what happens if the owner uploads more and more chunks with the batch AFTER some POs were evicted
	Evicted bool
}

func (u *UnreserveItem) MarshalBinary() ([]byte, error) {
	out := make([]byte, 32+1+1) // 32 byte batch ID + 1 byte uint8 radius
	copy(out, u.BatchID)
	out[32] = u.Radius

	var b byte = 0
	if u.Evicted {
		b = 1
	}

	out[33] = b
	return out, nil
}

func (u *UnreserveItem) UnmarshalBinary(b []byte) error {

	if len(b) != 34 {
		return errors.New("invalid unreserve item length")
	}

	u.BatchID = make([]byte, 32)
	copy(u.BatchID, b[:32])

	u.Radius = b[32]

	if b[33] > 0 {
		u.Evicted = true
	}

	return nil
}

// exp2 returns the e-th power of 2
func exp2(e uint) int64 {
	return 1 << e
}

func unreserveKey(b []byte) string {
	return fmt.Sprintf("%s_%s", unreservePrefix, string(b))
}
