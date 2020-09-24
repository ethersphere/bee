// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Service is the mock settlement service.
type Service struct {
	lock            sync.Mutex
	settlementsSent map[string]uint64
	settlementsRecv map[string]uint64

	settlementSentFunc func(swarm.Address) (uint64, error)
	settlementRecvFunc func(swarm.Address) (uint64, error)

	settlementsSentFunc func() (map[string]uint64, error)
	settlementsRecvFunc func() (map[string]uint64, error)
}

// WithsettlementFunc sets the mock settlement function
func WithSettlementSentFunc(f func(swarm.Address) (uint64, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementSentFunc = f
	})
}

func WithSettlementRecvFunc(f func(swarm.Address) (uint64, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementRecvFunc = f
	})
}

// WithsettlementsFunc sets the mock settlements function
func WithSettlementsSentFunc(f func() (map[string]uint64, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementsSentFunc = f
	})
}

func WithSettlementsRecvFunc(f func() (map[string]uint64, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementsRecvFunc = f
	})
}

// Newsettlement creates the mock settlement implementation
func NewSettlement(opts ...Option) settlement.Interface {
	mock := new(Service)
	mock.settlementsSent = make(map[string]uint64)
	mock.settlementsRecv = make(map[string]uint64)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func (s *Service) Pay(_ context.Context, peer swarm.Address, amount uint64) error {
	s.settlementsSent[peer.String()] += amount
	return nil
}

func (s *Service) TotalSent(peer swarm.Address) (totalSent uint64, err error) {
	if s.settlementSentFunc != nil {
		return s.settlementSentFunc(peer)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.settlementsSent[peer.String()], nil
}

func (s *Service) TotalReceived(peer swarm.Address) (totalSent uint64, err error) {
	if s.settlementRecvFunc != nil {
		return s.settlementRecvFunc(peer)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.settlementsRecv[peer.String()], nil
}

// settlements is the mock function wrapper that calls the set implementation
func (s *Service) SettlementsSent() (map[string]uint64, error) {
	if s.settlementsSentFunc != nil {
		return s.settlementsSentFunc()
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.settlementsSent, nil
}

func (s *Service) SettlementsReceived() (map[string]uint64, error) {
	if s.settlementsRecvFunc != nil {
		return s.settlementsRecvFunc()
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.settlementsRecv, nil
}

func (s *Service) SetPaymentObserver(settlement.PaymentObserver) {
}

// Option is the option passed to the mock settlement service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
