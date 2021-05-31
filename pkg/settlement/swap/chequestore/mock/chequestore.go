// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
)

// Service is the mock chequeStore service.
type Service struct {
	receiveCheque func(ctx context.Context, cheque *chequebook.SignedCheque, exchange *big.Int, deduction *big.Int) (*big.Int, error)
	lastCheque    func(chequebook common.Address) (*chequebook.SignedCheque, error)
	lastCheques   func() (map[common.Address]*chequebook.SignedCheque, error)
}

func WithReceiveChequeFunc(f func(ctx context.Context, cheque *chequebook.SignedCheque, exchange *big.Int, deduction *big.Int) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.receiveCheque = f
	})
}

func WithLastChequeFunc(f func(chequebook common.Address) (*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastCheque = f
	})
}

func WithLastChequesFunc(f func() (map[common.Address]*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastCheques = f
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

func (s *Service) ReceiveCheque(ctx context.Context, cheque *chequebook.SignedCheque, exchange *big.Int, deduction *big.Int) (*big.Int, error) {
	return s.receiveCheque(ctx, cheque, exchange, deduction)
}

func (s *Service) LastCheque(chequebook common.Address) (*chequebook.SignedCheque, error) {
	return s.lastCheque(chequebook)
}

func (s *Service) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return s.lastCheques()
}

// Option is the option passed to the mock ChequeStore service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
