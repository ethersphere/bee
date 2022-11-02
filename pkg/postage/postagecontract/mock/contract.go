// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"math/big"

	"github.com/ethersphere/bee/pkg/postage/postagecontract"
)

type contractMock struct {
	createBatch          func(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) ([]byte, error)
	topupBatch           func(ctx context.Context, id []byte, amount *big.Int) error
	diluteBatch          func(ctx context.Context, id []byte, newDepth uint8) error
	expireBatches        func(ctx context.Context) error
	countExpiredBatches  func(ctx context.Context) (*big.Int, error)
	expireLimitedBatches func(ctx context.Context, count *big.Int) error
}

func (c *contractMock) ExpireBatches(ctx context.Context) error {
	return c.expireBatches(ctx)
}

func (c *contractMock) CountExpiredBatches(ctx context.Context) (*big.Int, error) {
	return c.countExpiredBatches(ctx)
}

func (c *contractMock) ExpireLimitedBatches(ctx context.Context, count *big.Int) error {
	return c.expireLimitedBatches(ctx, count)
}

func (c *contractMock) CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) ([]byte, error) {
	return c.createBatch(ctx, initialBalance, depth, immutable, label)
}

func (c *contractMock) TopUpBatch(ctx context.Context, batchID []byte, amount *big.Int) error {
	return c.topupBatch(ctx, batchID, amount)
}

func (c *contractMock) DiluteBatch(ctx context.Context, batchID []byte, newDepth uint8) error {
	return c.diluteBatch(ctx, batchID, newDepth)
}

// Option is a an option passed to New
type Option func(*contractMock)

// New creates a new mock BatchStore
func New(opts ...Option) postagecontract.Interface {
	bs := &contractMock{}

	for _, o := range opts {
		o(bs)
	}

	return bs
}

func WithCreateBatchFunc(f func(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) ([]byte, error)) Option {
	return func(m *contractMock) {
		m.createBatch = f
	}
}

func WithTopUpBatchFunc(f func(ctx context.Context, batchID []byte, amount *big.Int) error) Option {
	return func(m *contractMock) {
		m.topupBatch = f
	}
}

func WithDiluteBatchFunc(f func(ctx context.Context, batchID []byte, newDepth uint8) error) Option {
	return func(m *contractMock) {
		m.diluteBatch = f
	}
}
