// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	batchKeyPrefix = "batchKeyPrefix"
	valueKeyPrefix = "valueKeyPrefix"
	stateKey       = "stateKey"
)

// Store is a local store for postage batches
type Store struct {
	store storage.StateStorer // State store backend to persist batches
}

// New constructs a new postage batch store
func New(store storage.StateStorer) postage.BatchStorer {
	return &Store{store}
}

// Get returns a batch from the batchstore with the given ID
func (s *Store) Get(id []byte) (*postage.Batch, error) {
	b := &postage.Batch{}
	err := s.store.Get(batchKey(id), b)
	return b, err
}

// Put stores a given batch in the batchstore with the given ID
func (s *Store) Put(b *postage.Batch) error {
	return s.store.Put(batchKey(b.ID), b)
}

// func (s *Store) Block() uint64 {
// 	return s.block
// }

// func (s *Store) SetBlock(b uint64) {

// }

// func (s *Store) Total() *big.Int {
// 	return new(big.Int).SetBytes(s.total.Bytes())
// }

// func (s *Store) Price() *big.Int {
// 	return new(big.Int).SetBytes(s.price.Bytes())
// }

// func (s *Store) Create(id []byte, owner []byte, value *big.Int, depth uint8) error {
// 	b := &Batch{
// 		ID:    id,
// 		Start: s.block,
// 		Owner: owner,
// 		Depth: depth,
// 		Value: value,
// 	}

// 	return s.put(b)
// }

// func (s *Store) TopUp(id []byte, value *big.Int) error {
// 	b, err := s.Get(id)
// 	if err != nil {
// 		return err
// 	}

// 	b.Value = value
// 	return s.put(b)
// }

// func (s *Store) UpdateDepth(id []byte, depth uint8) error {
// 	b, err := s.Get(id)
// 	if err != nil {
// 		return err
// 	}
// 	b.Depth = depth
// 	return s.put(b)
// }

// func (s *Store) UpdatePrice(price *big.Int) error {
// 	s.price = price
// 	return s.store.Put(stateKey, s)
// }

// batchKey returns the index key for the batch ID used in the by-ID batch index
func batchKey(id []byte) string {
	return batchKeyPrefix + string(id)
}

// // valueKey returns the index key for the batch value used in the by-value (priority) batch index
// func valueKey(v *big.Int) string {
// 	key := make([]byte, 32)
// 	value := v.Bytes()
// 	copy(key[32-len(value):], value)
// 	return valueKeyPrefix + string(key)
// }

// // MarshalBinary serialises the state to be used by the state store
// func (s *Store) MarshalBinary() ([]byte, error) {
// 	buf := make([]byte, 9)
// 	binary.BigEndian.PutUint64(buf, s.block)
// 	totalBytes := s.total.Bytes()
// 	if len(totalBytes) > math.MaxUint8 {
// 		return nil, fmt.Errorf("cumulative payout too large")
// 	}

// 	buf[8] = uint8(len(totalBytes))
// 	buf = append(buf, totalBytes...)
// 	return append(buf, s.price.Bytes()...), nil
// }

// // UnmarshalBinary deserialises the state to be used by the state store
// func (s *Store) UnmarshalBinary(buf []byte) error {
// 	s.block = binary.BigEndian.Uint64(buf[:8])
// 	totalLen := int(buf[8])
// 	if totalLen > math.MaxUint8 {
// 		return fmt.Errorf("cumulative payout too large")
// 	}

// 	s.total = new(big.Int).SetBytes(buf[9 : 9+totalLen])
// 	s.price = new(big.Int).SetBytes(buf[9+totalLen:])
// 	return nil
// }
