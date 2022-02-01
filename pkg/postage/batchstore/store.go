// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	batchKeyPrefix              = "batchstore_batch_"
	valueKeyPrefix              = "batchstore_value_"
	chainStateKey               = "batchstore_chainstate"
	reserveStateKey             = "batchstore_reservestate"
	unreserveQueueKey           = "batchstore_unreserve_queue_"
	ureserveQueueCardinalityKey = "batchstore_queue_cardinality"
)

// ErrNotFound signals that the element was not found.
var ErrNotFound = errors.New("batchstore: not found")

type unreserveFn func(batchID []byte, radius uint8) error
type evictFn func(batchID []byte) error

// store implements postage.Storer
type store struct {
	store storage.StateStorer // State store backend to persist batches.
	cs    *postage.ChainState // the chain state

	rsMtx       sync.Mutex
	rs          *reserveState // the reserve state
	unreserveFn unreserveFn   // unreserve function
	evictFn     evictFn       // evict function
	queueIdx    uint64        // unreserve queue cardinality
	metrics     metrics       // metrics
	logger      logging.Logger

	radiusSetter postage.RadiusSetter // setter for radius notifications
}

// create stores given batch and updates radius.
func (s *store) create(batch *postage.Batch, value *big.Int, depth uint8) error {
	//oldVal := new(big.Int).Set(batch.Value)
	//oldDepth := batch.Depth

	//batch.Value.Set(value)
	//batch.Depth = depth

	if err := s.store.Put(valueKey(batch.Value, batch.ID), nil); err != nil {
		return err
	}

	if err := s.update(batch, 0, big.NewInt(0)); err != nil {
		return err
	}

	if s.radiusSetter != nil {
		s.rsMtx.Lock()
		s.radiusSetter.SetRadius(s.rs.Radius)
		s.rsMtx.Unlock()
	}
	return s.store.Put(batchKey(batch.ID), batch)
}

// delete removes the batches with ids given as arguments.
func (s *store) delete(ids ...[]byte) error {
	for _, id := range ids {
		b, err := s.Get(id)
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

// New constructs a new postage batch store.
// It initialises both chain state and reserve state from the persistent state store.
func New(st storage.StateStorer, ev evictFn, logger logging.Logger) (postage.Storer, error) {
	cs := &postage.ChainState{}
	err := st.Get(chainStateKey, cs)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		cs = &postage.ChainState{
			Block:        0,
			TotalAmount:  big.NewInt(0),
			CurrentPrice: big.NewInt(0),
		}
	}
	rs := &reserveState{}
	err = st.Get(reserveStateKey, rs)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		rs = &reserveState{
			Radius:    DefaultDepth,
			Inner:     big.NewInt(0),
			Outer:     big.NewInt(0),
			Available: Capacity,
		}
	}

	s := &store{
		store:   st,
		cs:      cs,
		rs:      rs,
		evictFn: ev,
		metrics: newMetrics(),
		logger:  logger,
	}

	s.unreserveFn = s.unreserve
	if s.queueIdx, err = s.getQueueCardinality(); err != nil {
		return nil, err
	}

	return s, nil
}

// Get is implementation of postage.Storer interface Get method.
func (s *store) Get(id []byte) (*postage.Batch, error) {
	b := &postage.Batch{}
	err := s.store.Get(batchKey(id), b)
	if err != nil {
		return nil, fmt.Errorf("get batch %s: %w", hex.EncodeToString(id), err)
	}

	s.rsMtx.Lock()
	defer s.rsMtx.Unlock()

	if s.rs.StorageRadius < s.rs.Radius {
		b.Radius = s.rs.StorageRadius
	} else {
		b.Radius = s.rs.radius(s.rs.tier(b.Value))
	}
	return b, nil
}

// Exists is implementation of postage.Storer interface Exists method.
func (s *store) Exists(id []byte) (bool, error) {
	switch err := s.store.Get(batchKey(id), new(postage.Batch)); {
	case err == nil:
		return true, nil
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	default:
		return false, err
	}
}

// Iterate is implementation of postage.Storer interface Iterate method.
func (s *store) Iterate(cb func(*postage.Batch) (bool, error)) error {
	return s.store.Iterate(batchKeyPrefix, func(key, value []byte) (bool, error) {
		b := &postage.Batch{}
		if err := b.UnmarshalBinary(value); err != nil {
			return false, err
		}
		stop, err := cb(b)
		if stop {
			return true, nil
		}

		return false, err
	})
}

// Save is implementation of postage.Storer interface Save method.
// This method has side effects; it also updates the radius of the node if successful.
func (s *store) Save(batch *postage.Batch) error {
	switch err := s.store.Get(batchKey(batch.ID), new(postage.Batch)); {
	case errors.Is(err, storage.ErrNotFound):
		if err := s.store.Put(valueKey(batch.Value, batch.ID), nil); err != nil {
			return err
		}
		if err := s.update(batch, 0, big.NewInt(0)); err != nil {
			return err
		}
		if s.radiusSetter != nil {
			s.rsMtx.Lock()
			s.radiusSetter.SetRadius(s.rs.Radius)
			s.rsMtx.Unlock()
		}
		return s.store.Put(batchKey(batch.ID), batch)
	case err != nil:
		return fmt.Errorf("get batch %s: %w", hex.EncodeToString(batch.ID), err)
	}
	return nil
}

// Update is implementation of postage.Storer interface Update method.
// This method has side effects; it also updates the radius of the node if successful.
func (s *store) Update(batch *postage.Batch, value *big.Int, depth uint8) error {
	switch err := s.store.Get(batchKey(batch.ID), new(postage.Batch)); {
	case errors.Is(err, storage.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("get batch %s: %w", hex.EncodeToString(batch.ID), err)
	}

	if err := s.store.Delete(valueKey(batch.Value, batch.ID)); err != nil {
		return err
	}
	return s.create(batch, value, depth)
}

// GetChainState is implementation of postage.Storer interface GetChainState method.
func (s *store) GetChainState() *postage.ChainState {
	return s.cs
}

// PutChainState is implementation of postage.Storer interface PutChainState method.
// This method has side effects; it purges expired batches and unreserves underfunded
// ones before it stores the chain state in the store.
func (s *store) PutChainState(cs *postage.ChainState) error {
	s.cs = cs
	err := s.evictExpired()
	if err != nil {
		return err
	}
	// this needs to be improved, since we can miss some calls on
	// startup. the same goes for the other call to radiusSetter
	if s.radiusSetter != nil {
		s.rsMtx.Lock()
		s.radiusSetter.SetRadius(s.rs.Radius)
		s.rsMtx.Unlock()
	}

	return s.store.Put(chainStateKey, cs)
}

// GetReserveState is implementation of postage.Storer interface GetReserveState method.
func (s *store) GetReserveState() *postage.ReserveState {
	s.rsMtx.Lock()
	defer s.rsMtx.Unlock()
	return &postage.ReserveState{
		Radius:        s.rs.Radius,
		StorageRadius: s.rs.StorageRadius,
		Available:     s.rs.Available,
		Outer:         new(big.Int).Set(s.rs.Outer),
		Inner:         new(big.Int).Set(s.rs.Inner),
	}
}

// SetRadiusSetter is implementation of postage.Storer interface SetRadiusSetter method.
func (s *store) SetRadiusSetter(r postage.RadiusSetter) {
	s.radiusSetter = r
}

// Reset is implementation of postage.Storer interface Reset method.
func (s *store) Reset() error {
	const prefix = "batchstore_"
	if err := s.store.Iterate(prefix, func(k, _ []byte) (bool, error) {
		if strings.HasPrefix(string(k), prefix) {
			if err := s.store.Delete(string(k)); err != nil {
				return false, err
			}
		}
		return false, nil
	}); err != nil {
		return err
	}
	s.cs = &postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(0),
		CurrentPrice: big.NewInt(0),
	}
	s.rs = &reserveState{
		Radius:    DefaultDepth,
		Inner:     big.NewInt(0),
		Outer:     big.NewInt(0),
		Available: Capacity,
	}
	return nil
}

// batchKey returns the index key for the batch ID used in the by-ID batch index.
func batchKey(id []byte) string {
	return batchKeyPrefix + string(id)
}

// valueKey returns the index key for the batch ID used in the by-ID batch index.
func valueKey(val *big.Int, id []byte) string {
	value := make([]byte, 32)
	val.FillBytes(value) // zero-extended big-endian byte slice
	return valueKeyPrefix + string(value) + string(id)
}

// valueKeyToID extracts the batch ID from a value key - used in value-based iteration.
func valueKeyToID(key []byte) []byte {
	l := len(key)
	return key[l-32 : l]
}
