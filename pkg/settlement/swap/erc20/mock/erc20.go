// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
)

type Service struct {
	balanceOfFunc func(ctx context.Context, address common.Address) (*big.Int, error)
	transferFunc  func(ctx context.Context, address common.Address, value *big.Int) (common.Hash, error)
}

func WithBalanceOfFunc(f func(ctx context.Context, address common.Address) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.balanceOfFunc = f
	})
}

func WithTransferFunc(f func(ctx context.Context, address common.Address, value *big.Int) (common.Hash, error)) Option {
	return optionFunc(func(s *Service) {
		s.transferFunc = f
	})
}

func New(opts ...Option) erc20.Service {
	mock := new(Service)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func (s *Service) BalanceOf(ctx context.Context, address common.Address) (*big.Int, error) {
	if s.balanceOfFunc != nil {
		return s.balanceOfFunc(ctx, address)
	}
	return big.NewInt(0), errors.New("Error")
}

func (s *Service) Transfer(ctx context.Context, address common.Address, value *big.Int) (common.Hash, error) {
	if s.transferFunc != nil {
		return s.transferFunc(ctx, address, value)
	}
	return common.Hash{}, errors.New("Error")
}

// Option is the option passed to the mock Chequebook service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
