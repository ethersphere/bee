// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
)

var _ postage.Storer = (*BatchStore)(nil)

// BatchStore is a mock BatchStorer
type BatchStore struct {
	rs                *postage.ReserveState
	cs                *postage.ChainState
	id                []byte
	batch             *postage.Batch
	getErr            error
	getErrDelayCnt    int
	updateErr         error
	updateErrDelayCnt int
	resetCallCount    int
}

// Option is an option passed to New.
type Option func(*BatchStore)

// New creates a new mock BatchStore
func New(opts ...Option) *BatchStore {
	bs := &BatchStore{}
	bs.cs = &postage.ChainState{}
	for _, o := range opts {
		o(bs)
	}
	return bs
}

// WithReserveState will set the initial reservestate in the ChainStore mock.
func WithReserveState(rs *postage.ReserveState) Option {
	return func(bs *BatchStore) {
		bs.rs = rs
	}
}

// WithChainState will set the initial chainstate in the ChainStore mock.
func WithChainState(cs *postage.ChainState) Option {
	return func(bs *BatchStore) {
		bs.cs = cs
	}
}

// WithGetErr will set the get error returned by the ChainStore mock. The error
// will be returned on each subsequent call after delayCnt calls to Get have
// been made.
func WithGetErr(err error, delayCnt int) Option {
	return func(bs *BatchStore) {
		bs.getErr = err
		bs.getErrDelayCnt = delayCnt
	}
}

// WithUpdateErr will set the put error returned by the ChainStore mock.
// The error will be returned on each subsequent call after delayCnt
// calls to Update have been made.
func WithUpdateErr(err error, delayCnt int) Option {
	return func(bs *BatchStore) {
		bs.updateErr = err
		bs.updateErrDelayCnt = delayCnt
	}
}

// WithBatch will set batch to the one provided by user.
// This will be returned in the next Get.
func WithBatch(b *postage.Batch) Option {
	return func(bs *BatchStore) {
		bs.batch = b
		bs.id = b.ID
	}
}

// Get mocks the Get method from the BatchStore.
func (bs *BatchStore) Get(id []byte) (*postage.Batch, error) {
	if bs.getErr != nil {
		if bs.getErrDelayCnt == 0 {
			return nil, bs.getErr
		}
		bs.getErrDelayCnt--
	}
	exists, err := bs.Exists(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("no such id")
	}
	return bs.batch, nil
}

// Iterate mocks the Iterate method from the BatchStore
func (bs *BatchStore) Iterate(f func(*postage.Batch) (bool, error)) error {
	if bs.batch == nil {
		return nil
	}
	_, err := f(bs.batch)
	return err
}

// Save mocks the Save method from the BatchStore.
func (bs *BatchStore) Save(batch *postage.Batch) error {
	if bs.batch != nil {
		return errors.New("batch already taken")
	}
	bs.batch = batch
	bs.id = batch.ID
	return nil
}

// Update mocks the Update method from the BatchStore.
func (bs *BatchStore) Update(batch *postage.Batch, newValue *big.Int, newDepth uint8) error {
	if bs.batch == nil || !bytes.Equal(batch.ID, bs.id) {
		return batchstore.ErrNotFound
	}
	if bs.updateErr != nil {
		if bs.updateErrDelayCnt == 0 {
			return bs.updateErr
		}
		bs.updateErrDelayCnt--
	}
	bs.batch = batch
	batch.Depth = newDepth
	batch.Value.Set(newValue)
	bs.id = batch.ID
	return nil
}

// GetChainState mocks the GetChainState method from the BatchStore
func (bs *BatchStore) GetChainState() *postage.ChainState {
	return bs.cs
}

// PutChainState mocks the PutChainState method from the BatchStore
func (bs *BatchStore) PutChainState(cs *postage.ChainState) error {
	if bs.updateErr != nil {
		if bs.updateErrDelayCnt == 0 {
			return bs.updateErr
		}
		bs.updateErrDelayCnt--
	}
	bs.cs = cs
	return nil
}

func (bs *BatchStore) GetReserveState() *postage.ReserveState {
	rs := new(postage.ReserveState)
	if bs.rs != nil {
		rs.Radius = bs.rs.Radius
		rs.Available = bs.rs.Available
		rs.Outer = bs.rs.Outer
		rs.Inner = bs.rs.Inner
	}
	return rs
}
func (bs *BatchStore) Unreserve(_ postage.UnreserveIteratorFn) error {
	panic("not implemented")
}
func (bs *BatchStore) SetRadiusSetter(r postage.RadiusSetter) {
	panic("not implemented")
}

// Exists reports whether batch referenced by the give id exists.
func (bs *BatchStore) Exists(id []byte) (bool, error) {
	return bytes.Equal(bs.id, id), nil
}

func (bs *BatchStore) Reset() error {
	bs.resetCallCount++
	return nil
}

func (bs *BatchStore) ResetCalls() int {
	return bs.resetCallCount
}
