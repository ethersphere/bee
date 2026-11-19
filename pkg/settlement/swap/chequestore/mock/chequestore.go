// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
)

// Service is the mock chequeStore service.
type Service struct {
	receiveCheque       func(ctx context.Context, cheque *chequebook.SignedCheque, exchangeRate *big.Int, deduction *big.Int) (*big.Int, error)
	lastReceivedCheque  func(chequebook common.Address) (*chequebook.SignedCheque, error)
	lastReceivedCheques func() (map[common.Address]*chequebook.SignedCheque, error)
}

func WithReceiveChequeFunc(f func(ctx context.Context, cheque *chequebook.SignedCheque, exchangeRate *big.Int, deduction *big.Int) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.receiveCheque = f
	})
}

func WithLastReceivedChequeFunc(f func(chequebook common.Address) (*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastReceivedCheque = f
	})
}

func WithLastReceivedChequesFunc(f func() (map[common.Address]*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastReceivedCheques = f
	})
}

// NewChequeStore creates the mock chequeStore implementation
func NewChequeStore(opts ...Option) chequebook.ChequeStore {
	mock := new(Service)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func (s *Service) ReceiveCheque(ctx context.Context, cheque *chequebook.SignedCheque, exchangeRate, deduction *big.Int) (*big.Int, error) {
	return s.receiveCheque(ctx, cheque, exchangeRate, deduction)
}

func (s *Service) LastReceivedCheque(chequebook common.Address) (*chequebook.SignedCheque, error) {
	return s.lastReceivedCheque(chequebook)
}

func (s *Service) LastReceivedCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return s.lastReceivedCheques()
}

// Option is the option passed to the mock ChequeStore service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
