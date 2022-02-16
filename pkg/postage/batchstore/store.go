// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"encoding/binary"
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

	radiusSetter postage.RadiusSetter // setter for radius notifications
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

	return s, nil
}

func (s *store) GetReserveState() *postage.ReserveState {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	return &postage.ReserveState{
		Radius:        s.rs.Radius,
		StorageRadius: s.rs.StorageRadius,
		Available:     s.rs.Available,
	}
}

func (s *store) GetChainState() *postage.ChainState {
	return s.cs
}

// Get returns a batch from the batchstore with the given ID.
func (s *store) Get(id []byte) (*postage.Batch, error) {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.get(id)
}

func (s *store) get(id []byte) (*postage.Batch, error) {
	b := &postage.Batch{}
	err := s.store.Get(batchKey(id), b)
	if err != nil {
		return nil, fmt.Errorf("get batch %s: %w", hex.EncodeToString(id), err)
	}

	v, err := s.getValueItem(b)
	if err != nil {
		return nil, err
	}

	if v.StorageRadius < v.Radius {
		b.Radius = v.StorageRadius
	} else {
		b.Radius = v.Radius
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

	s.mtx.Lock()
	defer s.mtx.Unlock()

	switch err := s.store.Get(batchKey(batch.ID), new(postage.Batch)); {
	case errors.Is(err, storage.ErrNotFound):
		if err := s.store.Put(batchKey(batch.ID), batch); err != nil {
			return err
		}
		if err := s.allocateBatch(batch); err != nil {
			return err
		}
		if s.radiusSetter != nil {
			s.radiusSetter.SetRadius(s.rs.Radius)
		}
		return nil
	case err != nil:
		return fmt.Errorf("get batch %s: %w", hex.EncodeToString(batch.ID), err)
	}
	return nil
}

// Update is implementation of postage.Storer interface Update method.
// This method has side effects; it also updates the radius of the node if successful.
func (s *store) Update(batch *postage.Batch, value *big.Int, depth uint8) error {

	s.mtx.Lock()
	defer s.mtx.Unlock()

	switch err := s.store.Get(batchKey(batch.ID), new(postage.Batch)); {
	case errors.Is(err, storage.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("get batch %s: %w", hex.EncodeToString(batch.ID), err)
	}

	err := s.deallocateBatch(batch)
	if err != nil {
		return err
	}

	if err := s.store.Delete(valueKey(batch.Value, batch.ID)); err != nil {
		return err
	}

	batch.Value.Set(value)
	batch.Depth = depth

	err = s.store.Put(batchKey(batch.ID), batch)
	if err != nil {
		return err
	}

	err = s.allocateBatch(batch)
	if err != nil {
		return err
	}

	if s.radiusSetter != nil {
		s.radiusSetter.SetRadius(s.rs.Radius)
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

	err := s.cleanup()
	if err != nil {
		return err
	}

	err = s.adjustRadius(0)
	if err != nil {
		return err
	}

	// this needs to be improved, since we can miss some calls on
	// startup. the same goes for the other call to radiusSetter
	if s.radiusSetter != nil {
		s.radiusSetter.SetRadius(s.rs.Radius)
	}

	return s.store.Put(chainStateKey, cs)
}

// SetRadiusSetter is implementation of postage.Storer interface SetRadiusSetter method.
func (s *store) SetRadiusSetter(r postage.RadiusSetter) {
	s.radiusSetter = r
}

// Reset is implementation of postage.Storer interface Reset method.
func (s *store) Reset() error {

	s.mtx.Lock()
	defer s.mtx.Unlock()

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
		Available: Capacity,
	}

	return nil
}

func (s *store) putValueItem(id []byte, value *big.Int, radius, storageRadius uint8) error {
	return s.store.Put(valueKey(value, id), &valueItem{Radius: radius, StorageRadius: storageRadius})
}

func (s *store) getValueItem(b *postage.Batch) (*valueItem, error) {
	item := &valueItem{}

	err := s.store.Get(valueKey(b.Value, b.ID), item)
	return item, err
}

type valueItem struct {
	Radius        uint8
	StorageRadius uint8
}

func (u *valueItem) MarshalBinary() ([]byte, error) {

	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b, uint16(u.Radius))
	binary.BigEndian.PutUint16(b[2:], uint16(u.StorageRadius))

	return b, nil
}

func (u *valueItem) UnmarshalBinary(b []byte) error {

	u.Radius = uint8(binary.BigEndian.Uint16(b))
	u.StorageRadius = uint8(binary.BigEndian.Uint16(b[2:]))

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
