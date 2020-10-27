// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Service is the mock Accounting service.
type Service struct {
	lock               sync.Mutex
	balances           map[string]int64
	reserveFunc        func(ctx context.Context, peer swarm.Address, price uint64) error
	releaseFunc        func(peer swarm.Address, price uint64)
	creditFunc         func(peer swarm.Address, price uint64) error
	debitFunc          func(peer swarm.Address, price uint64) error
	balanceFunc        func(swarm.Address) (int64, error)
	balancesFunc       func() (map[string]int64, error)
	balanceSurplusFunc func(swarm.Address) (int64, error)
}

// WithReserveFunc sets the mock Reserve function
func WithReserveFunc(f func(ctx context.Context, peer swarm.Address, price uint64) error) Option {
	return optionFunc(func(s *Service) {
		s.reserveFunc = f
	})
}

// WithReleaseFunc sets the mock Release function
func WithReleaseFunc(f func(peer swarm.Address, price uint64)) Option {
	return optionFunc(func(s *Service) {
		s.releaseFunc = f
	})
}

// WithCreditFunc sets the mock Credit function
func WithCreditFunc(f func(peer swarm.Address, price uint64) error) Option {
	return optionFunc(func(s *Service) {
		s.creditFunc = f
	})
}

// WithDebitFunc sets the mock Debit function
func WithDebitFunc(f func(peer swarm.Address, price uint64) error) Option {
	return optionFunc(func(s *Service) {
		s.debitFunc = f
	})
}

// WithBalanceFunc sets the mock Balance function
func WithBalanceFunc(f func(swarm.Address) (int64, error)) Option {
	return optionFunc(func(s *Service) {
		s.balanceFunc = f
	})
}

// WithBalancesFunc sets the mock Balances function
func WithBalancesFunc(f func() (map[string]int64, error)) Option {
	return optionFunc(func(s *Service) {
		s.balancesFunc = f
	})
}

// WithBalanceSurplusFunc sets the mock SurplusBalance function
func WithBalanceSurplusFunc(f func(swarm.Address) (int64, error)) Option {
	return optionFunc(func(s *Service) {
		s.balanceSurplusFunc = f
	})
}

// NewAccounting creates the mock accounting implementation
func NewAccounting(opts ...Option) accounting.Interface {
	mock := new(Service)
	mock.balances = make(map[string]int64)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// Reserve is the mock function wrapper that calls the set implementation
func (s *Service) Reserve(ctx context.Context, peer swarm.Address, price uint64) error {
	if s.reserveFunc != nil {
		return s.reserveFunc(ctx, peer, price)
	}
	return nil
}

// Release is the mock function wrapper that calls the set implementation
func (s *Service) Release(peer swarm.Address, price uint64) {
	if s.releaseFunc != nil {
		s.releaseFunc(peer, price)
	}
}

// Credit is the mock function wrapper that calls the set implementation
func (s *Service) Credit(peer swarm.Address, price uint64) error {
	if s.creditFunc != nil {
		return s.creditFunc(peer, price)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.balances[peer.String()] -= int64(price)
	return nil
}

// Debit is the mock function wrapper that calls the set implementation
func (s *Service) Debit(peer swarm.Address, price uint64) error {
	if s.debitFunc != nil {
		return s.debitFunc(peer, price)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.balances[peer.String()] += int64(price)
	return nil
}

// Balance is the mock function wrapper that calls the set implementation
func (s *Service) Balance(peer swarm.Address) (int64, error) {
	if s.balanceFunc != nil {
		return s.balanceFunc(peer)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.balances[peer.String()], nil
}

// Balances is the mock function wrapper that calls the set implementation
func (s *Service) Balances() (map[string]int64, error) {
	if s.balancesFunc != nil {
		return s.balancesFunc()
	}
	return s.balances, nil
}

//
func (s *Service) SurplusBalance(peer swarm.Address) (int64, error) {
	if s.balanceFunc != nil {
		return s.balanceSurplusFunc(peer)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	return 0, nil
}

// Option is the option passed to the mock accounting service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
