// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"bytes"
	"errors"

	"github.com/ethersphere/bee/pkg/postage"
)

var _ postage.Storer = (*BatchStore)(nil)

// BatchStore is a mock BatchStorer
type BatchStore struct {
	cs             *postage.ChainState
	id             []byte
	batch          *postage.Batch
	getErr         error
	getErrDelayCnt int
	putErr         error
	putErrDelayCnt int
}

// Option is a an option passed to New
type Option func(*BatchStore)

// New creates a new mock BatchStore
func New(opts ...Option) *BatchStore {
	bs := &BatchStore{}

	for _, o := range opts {
		o(bs)
	}

	return bs
}

// WithChainState the initial ChainStore
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

// WithPutErr will set the put error returned by the ChainStore mock. The error
// will be returned on each subsequent call after delayCnt calls to Put have
// been made.
func WithPutErr(err error, delayCnt int) Option {
	return func(bs *BatchStore) {
		bs.putErr = err
		bs.putErrDelayCnt = delayCnt
	}
}

// Get mocks the Get method from the BatchStore
func (bs *BatchStore) Get(id []byte) (*postage.Batch, error) {
	if bs.getErr != nil {
		if bs.getErrDelayCnt == 0 {
			return nil, bs.getErr
		}
		bs.getErrDelayCnt--
	}
	if !bytes.Equal(bs.id, id) {
		return nil, errors.New("no such id")
	}
	return bs.batch, nil
}

// Put mocks the Put method from the BatchStore
func (bs *BatchStore) Put(batch *postage.Batch) error {
	if bs.putErr != nil {
		if bs.putErrDelayCnt == 0 {
			return bs.putErr
		}
		bs.putErrDelayCnt--
	}
	bs.batch = batch
	bs.id = batch.ID
	return nil
}

// GetChainState mocks the GetChainState method from the BatchStore
func (bs *BatchStore) GetChainState() (*postage.ChainState, error) {
	if bs.getErr != nil {
		if bs.getErrDelayCnt == 0 {
			return nil, bs.getErr
		}
		bs.getErrDelayCnt--
	}
	return bs.cs, nil
}

// PutChainState mocks the PutChainState method from the BatchStore
func (bs *BatchStore) PutChainState(cs *postage.ChainState) error {
	if bs.putErr != nil {
		if bs.putErrDelayCnt == 0 {
			return bs.putErr
		}
		bs.putErrDelayCnt--
	}
	bs.cs = cs
	return nil
}
