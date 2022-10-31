// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"bytes"
	"errors"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ postage.Storer = (*BatchStore)(nil)

// BatchStore is a mock BatchStorer
type BatchStore struct {
	rs                    *postage.ReserveState
	cs                    *postage.ChainState
	isWithinStorageRadius bool
	id                    []byte
	batch                 *postage.Batch
	getErr                error
	getErrDelayCnt        int
	updateErr             error
	updateErrDelayCnt     int
	resetCallCount        int

	radiusSetter postage.StorageRadiusSetter

	existsFn func([]byte) (bool, error)

	mtx sync.Mutex
}

func (bs *BatchStore) SetBatchExpiryHandler(eh postage.BatchExpiryHandler) {}

// Option is an option passed to New.
type Option func(*BatchStore)

// New creates a new mock BatchStore
func New(opts ...Option) *BatchStore {
	bs := &BatchStore{}
	bs.cs = &postage.ChainState{}
	bs.rs = &postage.ReserveState{}
	bs.isWithinStorageRadius = true
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

func WithIsWithinStorageRadius(b bool) Option {
	return func(bs *BatchStore) {
		bs.isWithinStorageRadius = b
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

func WithExistsFunc(f func([]byte) (bool, error)) Option {
	return func(bs *BatchStore) {
		bs.existsFn = f
	}
}

func WithAcceptAllExistsFunc() Option {
	return func(bs *BatchStore) {
		bs.existsFn = func(_ []byte) (bool, error) {
			return true, nil
		}
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
		return nil, storage.ErrNotFound
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
	bs.mtx.Lock()
	defer bs.mtx.Unlock()

	rs := new(postage.ReserveState)
	if bs.rs != nil {
		rs.Radius = bs.rs.Radius
		rs.StorageRadius = bs.rs.StorageRadius
	}
	return rs
}

func (bs *BatchStore) StorageRadius() uint8 {
	bs.mtx.Lock()
	defer bs.mtx.Unlock()

	if bs.rs != nil {
		return bs.rs.StorageRadius
	}
	return 0
}

func (bs *BatchStore) IsWithinStorageRadius(swarm.Address) bool {
	return bs.isWithinStorageRadius
}

func (bs *BatchStore) SetStorageRadiusSetter(r postage.StorageRadiusSetter) {
	bs.radiusSetter = r
}

func (bs *BatchStore) SetStorageRadius(f func(uint8) uint8) error {
	bs.mtx.Lock()
	defer bs.mtx.Unlock()

	bs.rs.StorageRadius = f(bs.rs.StorageRadius)
	if bs.radiusSetter != nil {
		bs.radiusSetter.SetStorageRadius(bs.rs.StorageRadius)
	}
	return nil
}

func (bs *BatchStore) Unreserve(_ postage.UnreserveIteratorFn) error {
	panic("not implemented")
}

// Exists reports whether batch referenced by the give id exists.
func (bs *BatchStore) Exists(id []byte) (bool, error) {
	if bs.existsFn != nil {
		return bs.existsFn(id)
	}
	return bytes.Equal(bs.id, id), nil
}

func (bs *BatchStore) Reset() error {
	bs.resetCallCount++
	return nil
}

func (bs *BatchStore) ResetCalls() int {
	return bs.resetCallCount
}

func (bs *BatchStore) GetBatchIDsExpiringUntil(*big.Int) (map[string]bool, error) {
	emptyMap := make(map[string]bool)
	return emptyMap, nil
}

type MockEventUpdater struct {
	inProgress bool
	err        error
}

func NewNotReady() *MockEventUpdater           { return &MockEventUpdater{inProgress: true} }
func NewWithError(err error) *MockEventUpdater { return &MockEventUpdater{inProgress: false, err: err} }

func (s *MockEventUpdater) GetSyncStatus() (isDone bool, err error) {
	return !s.inProgress, s.err
}
