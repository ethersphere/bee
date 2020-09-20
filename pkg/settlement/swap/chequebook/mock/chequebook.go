// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
)

// Service is the mock settlement service.
type Service struct {
	chequebookBalanceFunc func(context.Context) (*big.Int, error)
}

// WithsettlementFunc sets the mock settlement function
func WithChequebookBalanceFunc(f func(ctx context.Context) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.chequebookBalanceFunc = f
	})
}

// Newsettlement creates the mock settlement implementation
func NewChequebook(opts ...Option) chequebook.Service {
	mock := new(Service)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func (s *Service) Balance(ctx context.Context) (bal *big.Int, err error) {
	if s.chequebookBalanceFunc != nil {
		return s.chequebookBalanceFunc(ctx)
	}
	return big.NewInt(0), errors.New("Error")
}

func (s *Service) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	return common.Hash{}, errors.New("Error")
}
func (s *Service) WaitForDeposit(ctx context.Context, txHash common.Hash) error {
	return errors.New("Error")
}
func (s *Service) Address() common.Address {
	return common.Address{}
}

// Option is the option passed to the mock settlement service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
