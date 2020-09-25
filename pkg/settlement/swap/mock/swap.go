// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Service struct {
	lock            sync.Mutex
	settlementsSent map[string]uint64
	settlementsRecv map[string]uint64

	observer           settlement.PaymentObserver
	settlementSentFunc func(swarm.Address) (uint64, error)
	settlementRecvFunc func(swarm.Address) (uint64, error)

	settlementsSentFunc func() (map[string]uint64, error)
	settlementsRecvFunc func() (map[string]uint64, error)

	receiveChequeFunc      func(context.Context, swarm.Address, *chequebook.SignedCheque) error
	payFunc                func(context.Context, swarm.Address, uint64) error
	setPaymentObserverFunc func(observer settlement.PaymentObserver)
	handshakeFunc          func(swarm.Address, common.Address) error
	lastChequePeerFunc     func(swarm.Address) (*chequebook.SignedCheque, error)
	lastChequesFunc        func() (map[string]*chequebook.SignedCheque, error)
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

func WithReceiveChequeFunc(f func(context.Context, swarm.Address, *chequebook.SignedCheque) error) Option {
	return optionFunc(func(s *Service) {
		s.receiveChequeFunc = f
	})
}

func WithPayFunc(f func(context.Context, swarm.Address, uint64) error) Option {
	return optionFunc(func(s *Service) {
		s.payFunc = f
	})
}

// WithsettlementsFunc sets the mock settlements function
func WithSetPaymentObserverFunc(f func(observer settlement.PaymentObserver)) Option {
	return optionFunc(func(s *Service) {
		s.setPaymentObserverFunc = f
	})
}

func WithHandshakeFunc(f func(swarm.Address, common.Address) error) Option {
	return optionFunc(func(s *Service) {
		s.handshakeFunc = f
	})
}

func WithLastChequePeerFunc(f func(swarm.Address) (*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastChequePeerFunc = f
	})
}

func WithLastChequesFunc(f func() (map[string]*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastChequesFunc = f
	})
}

// New creates the mock swap implementation
func New(opts ...Option) settlement.Interface {
	mock := new(Service)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func NewApiInterface(opts ...Option) swap.ApiInterface {
	mock := new(Service)
	mock.settlementsSent = make(map[string]uint64)
	mock.settlementsRecv = make(map[string]uint64)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// ReceiveCheque is the mock ReceiveCheque function of swap.
func (s *Service) ReceiveCheque(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque) (err error) {
	if s.receiveChequeFunc != nil {
		return s.receiveChequeFunc(ctx, peer, cheque)
	}
	return nil
}

// Pay is the mock Pay function of swap.
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount uint64) error {
	if s.payFunc != nil {
		return s.payFunc(ctx, peer, amount)
	}
	return nil
}

// SetPaymentObserver is the mock SetPaymentObserver function of swap.
func (s *Service) SetPaymentObserver(observer settlement.PaymentObserver) {
	if s.setPaymentObserverFunc != nil {
		s.setPaymentObserverFunc(observer)
	}
}

// TotalSent is the mock TotalSent function of swap.
func (s *Service) TotalSent(peer swarm.Address) (totalSent uint64, err error) {
	if s.settlementSentFunc != nil {
		return s.settlementSentFunc(peer)
	}
	return 0, nil
}

// TotalReceived is the mock TotalReceived function of swap.
func (s *Service) TotalReceived(peer swarm.Address) (totalReceived uint64, err error) {
	if s.settlementRecvFunc != nil {
		return s.settlementRecvFunc(peer)
	}
	return 0, nil
}

// SettlementsSent is the mock SettlementsSent function of swap.
func (s *Service) SettlementsSent() (map[string]uint64, error) {
	if s.settlementsSentFunc != nil {
		return s.settlementsSentFunc()
	}
	result := make(map[string]uint64)
	return result, nil
}

// SettlementsReceived is the mock SettlementsReceived function of swap.
func (s *Service) SettlementsReceived() (map[string]uint64, error) {
	if s.settlementsRecvFunc != nil {
		return s.settlementsRecvFunc()
	}
	result := make(map[string]uint64)
	return result, nil
}

// Handshake is called by the swap protocol when a handshake is received.
func (s *Service) Handshake(peer swarm.Address, beneficiary common.Address) error {
	if s.handshakeFunc != nil {
		return s.handshakeFunc(peer, beneficiary)
	}
	return nil
}

func (s *Service) LastChequePeer(address swarm.Address) (*chequebook.SignedCheque, error) {
	if s.lastChequePeerFunc != nil {
		return s.lastChequePeerFunc(address)
	}
	return nil, nil
}

func (s *Service) LastCheques() (map[string]*chequebook.SignedCheque, error) {
	if s.lastChequesFunc != nil {
		return s.lastChequesFunc()
	}
	return nil, nil
}

// Option is the option passed to the mock settlement service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
