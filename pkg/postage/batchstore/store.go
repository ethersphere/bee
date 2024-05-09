// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "batchstore"

const (
	batchKeyPrefix   = "batchstore_batch_"
	valueKeyPrefix   = "batchstore_value_"
	chainStateKey    = "batchstore_chainstate"
	reserveRadiusKey = "batchstore_radius"
)

// ErrNotFound signals that the element was not found.
var ErrNotFound = errors.New("batchstore: not found")
var ErrStorageRadiusExceeds = errors.New("batchstore: storage radius must not exceed reserve radius")

type evictFn func(batchID []byte) error

// store implements postage.Storer
type store struct {
	capacity int
	store    storage.StateStorer // State store backend to persist batches.

	cs atomic.Pointer[postage.ChainState]

	radius  atomic.Uint32
	evictFn evictFn // evict function
	metrics metrics // metrics
	logger  log.Logger

	batchExpiry postage.BatchExpiryHandler

	mtx sync.RWMutex
}

// New constructs a new postage batch store.
// It initialises both chain state and reserve state from the persistent state store.
func New(st storage.StateStorer, ev evictFn, capacity int, logger log.Logger) (postage.Storer, error) {
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
	var radius uint8
	err = st.Get(reserveRadiusKey, &radius)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	}

	s := &store{
		capacity: capacity,
		store:    st,
		evictFn:  ev,
		metrics:  newMetrics(),
		logger:   logger.WithName(loggerName).Register(),
	}
	s.cs.Store(cs)

	s.radius.Store(uint32(radius))

	return s, nil
}

func (s *store) Radius() uint8 {
	return uint8(s.radius.Load())
}

func (s *store) GetChainState() *postage.ChainState {
	return s.cs.Load()
}

// Get returns a batch from the batchstore with the given ID.
func (s *store) Get(id []byte) (*postage.Batch, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.get(id)
}

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
	s.mtx.RLock()
	defer s.mtx.RUnlock()

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
		return cb(b)
	})
}

// Save is implementation of postage.Storer interface Save method.
// This method has side effects; it also updates the radius of the node if successful.
func (s *store) Save(batch *postage.Batch) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	switch err := s.store.Get(batchKey(batch.ID), new(postage.Batch)); {
	case errors.Is(err, storage.ErrNotFound):
		if err := s.store.Put(batchKey(batch.ID), batch); err != nil {
			return err
		}

		if err := s.saveBatch(batch); err != nil {
			return err
		}

		s.logger.Debug("batch saved", "batch_id", hex.EncodeToString(batch.ID), "batch_depth", batch.Depth, "batch_value", batch.Value.Int64(), "radius", s.radius.Load())

		return nil
	case err != nil:
		return fmt.Errorf("batchstore: save batch %s depth %d value %d failed: get batch: %w", hex.EncodeToString(batch.ID), batch.Depth, batch.Value.Int64(), err)
	}

	return fmt.Errorf("batchstore: save batch %s depth %d value %d failed: already exists", hex.EncodeToString(batch.ID), batch.Depth, batch.Value.Int64())
}

// Update is implementation of postage.Storer interface Update method.
// This method has side effects; it also updates the radius of the node if successful.
func (s *store) Update(batch *postage.Batch, value *big.Int, depth uint8) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	oldBatch := &postage.Batch{}

	s.logger.Debug("update batch", "batch_id", hex.EncodeToString(batch.ID), "new_batch_depth", depth, "new_batch_value", value.Int64())

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

	err := s.store.Put(batchKey(batch.ID), batch)
	if err != nil {
		return err
	}

	err = s.saveBatch(batch)
	if err != nil {
		return err
	}

	return nil
}

// PutChainState is implementation of postage.Storer interface PutChainState method.
// This method has side effects; it purges expired batches and unreserves underfunded
// ones before it stores the chain state in the store.
func (s *store) PutChainState(cs *postage.ChainState) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.cs.Store(cs)

	s.logger.Debug("put chain state", "block", cs.Block, "amount", cs.TotalAmount.Int64(), "price", cs.CurrentPrice.Int64())

	err := s.cleanup()
	if err != nil {
		return fmt.Errorf("batchstore: put chain state clean up: %w", err)
	}

	err = s.computeRadius()
	if err != nil {
		return fmt.Errorf("batchstore: put chain state adjust radius: %w", err)
	}

	return s.store.Put(chainStateKey, cs)
}

func (s *store) Commitment() (uint64, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var totalCommitment int
	err := s.store.Iterate(batchKeyPrefix, func(key, value []byte) (bool, error) {

		b := &postage.Batch{}
		if err := b.UnmarshalBinary(value); err != nil {
			return false, err
		}

		totalCommitment += exp2(uint(b.Depth))

		return false, nil
	})
	if err != nil {
		return 0, err
	}
	return uint64(totalCommitment), err
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

	s.cs.Store(&postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(0),
		CurrentPrice: big.NewInt(0),
	})

	s.radius = atomic.Uint32{}

	return nil
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
		if b.Value.Cmp(s.cs.Load().TotalAmount) <= 0 {
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
		s.logger.Debug("batch expired", "batch_id", hex.EncodeToString(b.ID))
		if s.batchExpiry != nil {
			err = s.batchExpiry.HandleStampExpiry(context.Background(), b.ID)
			if err != nil {
				return fmt.Errorf("handle stamp expiry, batch %x: %w", b.ID, err)
			}
		}
		err = s.evictFn(b.ID)
		if err != nil {
			return fmt.Errorf("evict batch %x: %w", b.ID, err)
		}
		err := s.store.Delete(valueKey(b.Value, b.ID))
		if err != nil {
			return fmt.Errorf("delete value key for batch %x: %w", b.ID, err)
		}
		err = s.store.Delete(batchKey(b.ID))
		if err != nil {
			return fmt.Errorf("delete batch %x: %w", b.ID, err)
		}
	}

	return nil
}

// computeRadius calculates the radius by using the sum of all batch depths
// and the node capacity using the formula totalCommitment/node_capacity = 2^R.
// Must be called under lock.
func (s *store) computeRadius() error {

	var totalCommitment int

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

	s.metrics.Commitment.Set(float64(totalCommitment))

	var radius uint8
	if totalCommitment > s.capacity {
		// totalCommitment/node_capacity = 2^R
		// log2(totalCommitment/node_capacity) = R
		radius = uint8(math.Ceil(math.Log2(float64(totalCommitment) / float64(s.capacity))))
	}

	s.metrics.Radius.Set(float64(radius))
	s.radius.Store(uint32(radius))

	return s.store.Put(reserveRadiusKey, &radius)
}

// exp2 returns the e-th power of 2
func exp2(e uint) int {
	return 1 << e
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

func (s *store) SetBatchExpiryHandler(be postage.BatchExpiryHandler) {
	s.batchExpiry = be
}
