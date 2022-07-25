// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	batchKeyPrefix  = "batchstore_batch_"
	valueKeyPrefix  = "batchstore_value_"
	chainStateKey   = "batchstore_chainstate"
	reserveStateKey = "batchstore_reservestate"
)

// ErrNotFound signals that the element was not found.
var ErrNotFound = errors.New("batchstore: not found")

type evictFn func(batchID []byte) error

// store implements postage.Storer
type store struct {
	mtx sync.Mutex

	store storage.StateStorer // State store backend to persist batches.
	cs    *postage.ChainState // the chain state

	rs      *reserveState // the reserve state
	evictFn evictFn       // evict function
	metrics metrics       // metrics
	logger  logging.Logger

	storageRadiusSetter postage.StorageRadiusSetter // setter for radius notifications
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
			Radius:        0,
			StorageRadius: 0,
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

	return s, nil
}

func (s *store) GetReserveState() *postage.ReserveState {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	return &postage.ReserveState{
		Radius:        s.rs.Radius,
		StorageRadius: s.rs.StorageRadius,
	}
}

func (s *store) GetChainState() *postage.ChainState {
	return s.cs
}

// Get returns a batch from the batchstore with the given ID.
func (s *store) Get(id []byte) (*postage.Batch, error) {

	defer func(t time.Time) {
		s.metrics.GetDuration.WithLabelValues("true").Observe(time.Since(t).Seconds())
	}(time.Now())

	s.mtx.Lock()
	defer s.mtx.Unlock()

	defer func(t time.Time) {
		s.metrics.GetDuration.WithLabelValues("false").Observe(time.Since(t).Seconds())
	}(time.Now())

	return s.get(id)
}

// get returns the postage batch from the statestore.
// Must be called under lock.
func (s *store) get(id []byte) (*postage.Batch, error) {
	b := &postage.Batch{}
	err := s.store.Get(batchKey(id), b)
	if err != nil {
		return nil, fmt.Errorf("get batch %s: %w", hex.EncodeToString(id), err)
	}
	return b, nil
}

// Exists is implementation of postage.Storer interface Exists method.
func (s *store) Exists(id []byte) (bool, error) {

	defer func(t time.Time) {
		s.metrics.ExistsDuration.WithLabelValues("true").Observe(time.Since(t).Seconds())
	}(time.Now())

	s.mtx.Lock()
	defer s.mtx.Unlock()

	defer func(t time.Time) {
		s.metrics.ExistsDuration.WithLabelValues("false").Observe(time.Since(t).Seconds())
	}(time.Now())

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

	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.store.Iterate(batchKeyPrefix, func(key, value []byte) (bool, error) {
		b := &postage.Batch{}
		if err := b.UnmarshalBinary(value); err != nil {
			return false, err
		}
		return cb(b)
	})
}

// Save is implementation of postage.Storer interface Save method.
// This method has side effects; it also updates the radius of the node if successful.
func (s *store) Save(batch *postage.Batch) error {
	defer func(t time.Time) {
		s.metrics.SaveDuration.WithLabelValues("true").Observe(time.Since(t).Seconds())
	}(time.Now())

	s.mtx.Lock()
	defer s.mtx.Unlock()

	defer func(t time.Time) {
		s.metrics.SaveDuration.WithLabelValues("false").Observe(time.Since(t).Seconds())
	}(time.Now())

	switch err := s.store.Get(batchKey(batch.ID), new(postage.Batch)); {
	case errors.Is(err, storage.ErrNotFound):
		batch.StorageRadius = s.rs.StorageRadius
		if err := s.store.Put(batchKey(batch.ID), batch); err != nil {
			return err
		}

		if err := s.saveBatch(batch); err != nil {
			return err
		}

		if s.storageRadiusSetter != nil {
			s.storageRadiusSetter.SetStorageRadius(s.rs.StorageRadius)
		}
		return nil
	case err == nil:
		return fmt.Errorf("batchstore: save batch %s depth %d value %d failed: already exists", hex.EncodeToString(batch.ID), batch.Depth, batch.Value.Int64())
	case err != nil:
		return fmt.Errorf("batchstore: save batch %s depth %d value %d failed: get batch: %w", hex.EncodeToString(batch.ID), batch.Depth, batch.Value.Int64(), err)
	}

	s.logger.Debugf("batchstore: saved batch %x depth %d value %d, radius %d, storage radius %d", batch.ID, batch.Depth, batch.Value.Int64(), s.rs.Radius, s.rs.StorageRadius)

	return nil
}

// Update is implementation of postage.Storer interface Update method.
// This method has side effects; it also updates the radius of the node if successful.
func (s *store) Update(batch *postage.Batch, value *big.Int, depth uint8) error {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	oldBatch := &postage.Batch{}

	s.logger.Debugf("batchstore: update batch %x depth %d value %d", batch.ID, depth, value.Int64())

	switch err := s.store.Get(batchKey(batch.ID), oldBatch); {
	case errors.Is(err, storage.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("get batch %s: %w", hex.EncodeToString(batch.ID), err)
	}

	if err := s.store.Delete(valueKey(batch.Value, batch.ID)); err != nil {
		return err
	}

	batch.Value.Set(value)
	batch.Depth = depth
	batch.StorageRadius = oldBatch.StorageRadius

	err := s.store.Put(batchKey(batch.ID), batch)
	if err != nil {
		return err
	}

	err = s.saveBatch(batch)
	if err != nil {
		return err
	}

	if s.storageRadiusSetter != nil {
		s.storageRadiusSetter.SetStorageRadius(s.rs.StorageRadius)
	}

	return nil
}

// PutChainState is implementation of postage.Storer interface PutChainState method.
// This method has side effects; it purges expired batches and unreserves underfunded
// ones before it stores the chain state in the store.
func (s *store) PutChainState(cs *postage.ChainState) error {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.cs = cs

	s.logger.Debugf("batchstore: put chain state block %d amount %d price %d", cs.Block, cs.TotalAmount.Int64(), cs.CurrentPrice.Int64())

	err := s.cleanup()
	if err != nil {
		return fmt.Errorf("batchstore: put chain state clean up: %w", err)
	}

	err = s.computeRadius()
	if err != nil {
		return fmt.Errorf("batchstore: put chain state adjust radius: %w", err)
	}

	// this needs to be improved, since we can miss some calls on
	// startup. the same goes for the other call to radiusSetter
	if s.storageRadiusSetter != nil {
		s.storageRadiusSetter.SetStorageRadius(s.rs.StorageRadius)
	}

	s.metrics.StorageRadius.Set(float64(s.rs.StorageRadius))
	s.metrics.Radius.Set(float64(s.rs.Radius))

	return s.store.Put(chainStateKey, cs)
}

// SetRadiusSetter is implementation of postage.Storer interface SetRadiusSetter method.
func (s *store) SetStorageRadiusSetter(r postage.StorageRadiusSetter) {
	s.storageRadiusSetter = r
}

// Reset is implementation of postage.Storer interface Reset method.
func (s *store) Reset() error {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	const prefix = "batchstore_"
	if err := s.store.Iterate(prefix, func(k, _ []byte) (bool, error) {
		return false, s.store.Delete(string(k))
	}); err != nil {
		return err
	}

	s.cs = &postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(0),
		CurrentPrice: big.NewInt(0),
	}

	s.rs = &reserveState{
		Radius: 0,
	}

	return nil
}

// batchKey returns the index key for the batch ID used in the by-ID batch index.
func batchKey(batchID []byte) string {
	return batchKeyPrefix + string(batchID)
}

// valueKey returns the index key for the batch ID used in the by-ID batch index.
func valueKey(val *big.Int, batchID []byte) string {
	value := make([]byte, 32)
	val.FillBytes(value) // zero-extended big-endian byte slice
	return valueKeyPrefix + string(value) + string(batchID)
}

// valueKeyToID extracts the batch ID from a value key - used in value-based iteration.
func valueKeyToID(key []byte) []byte {
	l := len(key)
	return key[l-32 : l]
}
