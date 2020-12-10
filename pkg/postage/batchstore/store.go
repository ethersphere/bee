// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	batchKeyPrefix = "batchKeyPrefix"
	valueKeyPrefix = "valueKeyPrefix"
	stateKey       = "stateKey"
)

var _ postage.EventUpdater = (*Store)(nil)

// Store is a local store  for postage batches
type Store struct {
	store  storage.StateStorer // state store backend to persist batches
	mu     sync.Mutex          // mutex to lock statestore during atomic changes
	state  *state              // the current state
	logger logging.Logger
}

// New constructs a new postage batch store
func New(store storage.StateStorer, logger logging.Logger) (*Store, error) {
	// initialise state from statestore or start with 0-s
	st := &state{}
	err := store.Get(stateKey, st)
	if err == storage.ErrNotFound {
		st.total = big.NewInt(0)
		st.price = big.NewInt(0)
	}

	s := &Store{
		store:  store,
		logger: logger,
	}
	return s, nil
}

// settle retrieves the current state
// - sets the cumulative outpayment normalised, cno+=price*period
// - sets the new block number
func (s *Store) settle(block uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updatePeriod := int64(block - s.state.block)
	s.state.block = block
	s.state.total.Add(s.state.total, new(big.Int).Mul(s.state.price, big.NewInt(updatePeriod)))

	_ = s.store.Put(stateKey, s.state)
}

func (s *Store) get(id []byte) (*Batch, error) {
	b := &Batch{}
	err := s.store.Get(batchKey(id), b)
	return b, err
}

func (s *Store) put(b *Batch) error {
	return s.store.Put(batchKey(b.ID), b)
}

func (s *Store) Create(id []byte, owner []byte, value *big.Int, depth uint8) error {
	b := &Batch{
		ID:    id,
		Start: s.state.block,
		Owner: owner,
		Depth: depth,
		Value: value,
	}

	return s.put(b)
}

func (s *Store) TopUp(id []byte, value *big.Int) error {
	b, err := s.get(id)
	if err != nil {
		return err
	}
	b.Value = value
	return s.put(b)
}

func (s *Store) UpdateDepth(id []byte, depth uint8) error {
	b, err := s.get(id)
	if err != nil {
		return err
	}
	b.Depth = depth
	return s.put(b)
}

func (s *Store) UpdatePrice(price *big.Int) error {
	s.state.price = price
	return nil
}

// batchKey returns the index key for the batch ID used in the by-ID batch index
func batchKey(id []byte) string {
	return batchKeyPrefix + string(id)
}

// valueKey returns the index key for the batch value used in the by-value (priority) batch index
func valueKey(v *big.Int) string {
	key := make([]byte, 32)
	value := v.Bytes()
	copy(key[32-len(value):], value)
	return valueKeyPrefix + string(key)
}

// state implements BinaryMarshaler interface
var _ encoding.BinaryMarshaler = (*state)(nil)

// state represents the current state of the reserve
type state struct {
	block uint64   // the block number of the last postage event
	total *big.Int // cumulative amount paid per stamp
	price *big.Int // bzz/chunk/block normalised price, comes from the oracle
}

// MarshalBinary serialises the state to be used by the state store
func (st *state) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 9)
	binary.BigEndian.PutUint64(buf, st.block)
	totalBytes := st.total.Bytes()
	if len(totalBytes) > math.MaxUint8 {
		return nil, fmt.Errorf("cumulative payout too large")
	}

	buf[8] = uint8(len(totalBytes))
	buf = append(buf, totalBytes...)
	return append(buf, st.price.Bytes()...), nil
}

// UnmarshalBinary deserialises the state to be used by the state store
func (st *state) UnmarshalBinary(buf []byte) error {
	st.block = binary.BigEndian.Uint64(buf[:8])
	totalLen := int(buf[8])
	if totalLen > math.MaxUint8 {
		return fmt.Errorf("cumulative payout too large")
	}

	st.total = new(big.Int).SetBytes(buf[9 : 9+totalLen])
	st.price = new(big.Int).SetBytes(buf[9+totalLen:])
	return nil
}
