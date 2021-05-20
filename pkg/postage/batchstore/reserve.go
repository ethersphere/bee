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
package batchstore

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// ErrBatchNotFound is returned when the postage batch is not found or expired
var ErrBatchNotFound = errors.New("postage batch not found or expired")

// DefaultDepth is the initial depth for the reserve
var DefaultDepth = uint8(12) // 12 is the testnet depth at the time of merging to master

// Capacity is the number of chunks in reserve. `2^22` (4194304) was chosen to remain
// relatively near the current 5M chunks ~25GB.
var Capacity = exp2(22)

var big1 = big.NewInt(1)

// reserveState records the state and is persisted in the state store
type reserveState struct {
	// Radius is the radius of responsibility,
	// it defines the proximity order of chunks which we
	// would like to guarantee that all chunks are stored
	Radius uint8 `json:"radius"`
	// Available capacity of the reserve which can still be used.
	Available int64    `json:"available"`
	Outer     *big.Int `json:"outer"` // lower value limit for outer layer = the further half of chunks
	Inner     *big.Int `json:"inner"` // lower value limit for inner layer = the closer half of chunks
}

// unreserve is called when the batchstore decides not to reserve a batch on a PO
// i.e. chunk of the batch in bins [0 upto PO] (closed  interval) are unreserved
func (s *store) unreserve(b *postage.Batch, radius uint8) error {
	return s.unreserveFunc(b.ID, radius)
}

// evictExpired is called when PutChainState is called (and there is 'settlement')
func (s *store) evictExpired() error {
	var toDelete [][]byte

	// set until to total or inner whichever is greater
	until := new(big.Int)

	// if inner > 0 && total >= inner
	if s.rs.Inner.Cmp(big.NewInt(0)) > 0 && s.cs.TotalAmount.Cmp(s.rs.Inner) >= 0 {
		// collect until total+1
		until.Add(s.cs.TotalAmount, big1)
	} else {
		// collect until inner (collect all outer ones)
		until.Set(s.rs.Inner)
	}
	var multiplier int64
	err := s.store.Iterate(valueKeyPrefix, func(key, _ []byte) (bool, error) {
		b, err := s.Get(valueKeyToID(key))
		if err != nil {
			return true, err
		}

		// if batch value >= until then continue to next.
		// terminate iteration if until is passed
		if b.Value.Cmp(until) >= 0 {
			return true, nil
		}

		// in the following if statements we check the batch value
		// against the inner and outer values and set the multiplier
		// to 1 or 2 depending on the value. if the batch value falls
		// outside of Outer it means we are evicting twice more chunks
		// than within Inner, therefore the multiplier is needed to
		// estimate better how much capacity gain is leveraged from
		// evicting this specific batch.

		// if multiplier == 0 && batch value >= inner
		if multiplier == 0 && b.Value.Cmp(s.rs.Inner) >= 0 {
			multiplier = 1
		}
		// if multiplier == 1 && batch value >= outer
		if multiplier == 1 && b.Value.Cmp(s.rs.Outer) >= 0 {
			multiplier = 2
		}

		// unreserve batch fully
		err = s.unreserve(b, swarm.MaxPO+1)
		if err != nil {
			return true, err
		}
		s.rs.Available += multiplier * exp2(b.Radius-s.rs.Radius-1)

		// if batch has no value then delete it
		if b.Value.Cmp(s.cs.TotalAmount) <= 0 {
			toDelete = append(toDelete, b.ID)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	// set inner to either until or Outer, whichever
	// is the smaller value.

	s.rs.Inner.Set(until)

	// if outer < until
	if s.rs.Outer.Cmp(until) < 0 {
		s.rs.Outer.Set(until)
	}
	if err = s.store.Put(reserveStateKey, s.rs); err != nil {
		return err
	}
	return s.delete(toDelete...)
}

// tier represents the sections of the reserve that can be  described as value intervals
// 0 - out of the reserve
// 1 - within reserve radius = depth (inner half)
// 2 - within reserve radius = depth-1 (both inner and outer halves)
type tier int

const (
	unreserved tier = iota // out of the reserve
	inner                  // the mid range where chunks are kept within depth
	outer                  // top range where chunks are kept within depth - 1
)

// change calculates info relevant to the value change from old to new value and old and new depth
// returns the change in capacity and the radius of reserve
func (rs *reserveState) change(oldv, newv *big.Int, oldDepth, newDepth uint8) (int64, uint8) {
	oldTier := rs.tier(oldv)
	newTier := rs.setLimits(newv, rs.tier(newv))

	oldSize := rs.size(oldDepth, oldTier)
	newSize := rs.size(newDepth, newTier)

	availableCapacityChange := oldSize - newSize
	reserveRadius := rs.radius(newTier)

	return availableCapacityChange, reserveRadius
}

// size returns the number of chunks the local node is responsible
// to store in its reserve.
func (rs *reserveState) size(depth uint8, t tier) int64 {
	size := exp2(depth - rs.Radius - 1)
	switch t {
	case inner:
		return size
	case outer:
		return 2 * size
	default:
		// case is unreserved
		return 0
	}
}

// tier returns which tier a value falls into
func (rs *reserveState) tier(x *big.Int) tier {

	// x < rs.Inner || x == 0
	if x.Cmp(rs.Inner) < 0 || rs.Inner.Cmp(big.NewInt(0)) == 0 {
		return unreserved
	}

	// x < rs.Outer
	if x.Cmp(rs.Outer) < 0 {
		return inner
	}

	// x >= rs.Outer
	return outer
}

// radius returns the reserve radius of a batch given the depth (radius of responsibility)
// based on the tier it falls in
func (rs *reserveState) radius(t tier) uint8 {
	switch t {
	case unreserved:
		return swarm.MaxPO
	case inner:
		return rs.Radius
	default:
		// outer
		return rs.Radius - 1
	}
}

// setLimits sets the tier 1 value limit, if new item is the minimum so far (or the very first batch)
// returns the adjusted new tier
func (rs *reserveState) setLimits(val *big.Int, newTier tier) tier {
	if newTier != unreserved {
		return newTier
	}

	// if we're here it means that the new tier
	// falls under the unreserved tier
	var adjustedTier tier

	// rs.Inner == 0 || rs.Inner > val
	if rs.Inner.Cmp(big.NewInt(0)) == 0 || rs.Inner.Cmp(val) > 0 {
		adjustedTier = inner
		// if the outer is the same as the inner
		if rs.Outer.Cmp(rs.Inner) == 0 {
			// the value falls below inner and outer
			rs.Outer.Set(val)
			adjustedTier = outer
		}
		// inner is decreased to val, this is done when the
		// batch is diluted, decreasing the value of it.
		rs.Inner.Set(val)
	}
	return adjustedTier
}

// update manages what chunks of which batch are allocated to the reserve
func (s *store) update(b *postage.Batch, oldDepth uint8, oldValue *big.Int) error {
	newValue := b.Value
	newDepth := b.Depth
	capacityChange, reserveRadius := s.rs.change(oldValue, newValue, oldDepth, newDepth)
	s.rs.Available += capacityChange

	if err := s.unreserve(b, reserveRadius); err != nil {
		return err
	}
	err := s.evictOuter(b)
	if err != nil {
		return err
	}

	s.metrics.AvailableCapacity.Set(float64(s.rs.Available))
	s.metrics.Radius.Set(float64(s.rs.Radius))
	s.metrics.Inner.Set(float64(s.rs.Inner.Int64()))
	s.metrics.Outer.Set(float64(s.rs.Outer.Int64()))
	return nil
}

// evictOuter is responsible for keeping capacity positive by unreserving lowest priority batches
func (s *store) evictOuter(last *postage.Batch) error {
	// if capacity is positive nothing to evict
	if s.rs.Available >= 0 {
		return nil
	}
	err := s.store.Iterate(valueKeyPrefix, func(key, _ []byte) (bool, error) {
		batchID := valueKeyToID(key)
		b := last
		if !bytes.Equal(b.ID, batchID) {
			var err error
			b, err = s.Get(batchID)
			if err != nil {
				return true, fmt.Errorf("release get %x %v: %w", batchID, b, err)
			}
		}
		//  FIXME: this is needed only because  the statestore iterator does not allow seek, only prefix
		//  so we need to page through all the batches until outer limit is reached
		if b.Value.Cmp(s.rs.Outer) < 0 {
			return false, nil
		}
		// stop iteration  only if we consumed all batches of the same value as the one that put capacity above zero
		if s.rs.Available >= 0 && s.rs.Outer.Cmp(b.Value) != 0 {
			return true, nil
		}
		// unreserve outer PO of the lowest priority batch  until capacity is back to positive
		s.rs.Available += exp2(b.Depth - s.rs.Radius - 1)
		s.rs.Outer.Set(b.Value)
		return false, s.unreserve(b, s.rs.Radius)
	})
	if err != nil {
		return err
	}
	// add 1 to outer limit value so we dont hit on the same batch next time we iterate
	s.rs.Outer.Add(s.rs.Outer, big1)
	// if we consumed all batches, ie. we unreserved all chunks on the outer = depth PO
	//  then its time to  increase depth
	if s.rs.Available < 0 {
		s.rs.Radius++
		s.rs.Outer.Set(s.rs.Inner) // reset outer limit to inner limit
		return s.evictOuter(last)
	}
	return s.store.Put(reserveStateKey, s.rs)
}

// exp2 returns the e-th power of 2
func exp2(e uint8) int64 {
	if e == 0 {
		return 1
	}
	b := int64(2)
	for i := uint8(1); i < e; i++ {
		b *= 2
	}
	return b
}
