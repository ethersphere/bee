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
	createBatch   func(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (string, []byte, error)
	topupBatch    func(ctx context.Context, id []byte, amount *big.Int) (string, error)
	diluteBatch   func(ctx context.Context, id []byte, newDepth uint8) (string, error)
	expireBatches func(ctx context.Context) error
}

func (c *contractMock) CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (string, []byte, error) {
	return c.createBatch(ctx, initialBalance, depth, immutable, label)
}

func (c *contractMock) TopUpBatch(ctx context.Context, batchID []byte, topUpBalance *big.Int) (string, error) {
	return c.topupBatch(ctx, batchID, topUpBalance)
}

func (c *contractMock) DiluteBatch(ctx context.Context, batchID []byte, newDepth uint8) (string, error) {
	return c.diluteBatch(ctx, batchID, newDepth)
}

func (c *contractMock) ExpireBatches(ctx context.Context) error {
	return c.expireBatches(ctx)
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

func WithCreateBatchFunc(f func(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (string, []byte, error)) Option {
	return func(m *contractMock) {
		m.createBatch = f
	}
}

func WithTopUpBatchFunc(f func(ctx context.Context, batchID []byte, amount *big.Int) (string, error)) Option {
	return func(m *contractMock) {
		m.topupBatch = f
	}
}

func WithDiluteBatchFunc(f func(ctx context.Context, batchID []byte, newDepth uint8) (string, error)) Option {
	return func(m *contractMock) {
		m.diluteBatch = f
	}
}

func WithExpiresBatchesFunc(f func(ctx context.Context) error) Option {
	return func(m *contractMock) {
		m.expireBatches = f
	}
}
