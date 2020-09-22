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

// Service is the mock chequebook service.
type Service struct {
	chequebookBalanceFunc func(context.Context) (*big.Int, error)
	chequebookAddressFunc func() common.Address
}

// WithChequebook*Functions set the mock chequebook functions
func WithChequebookBalanceFunc(f func(ctx context.Context) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.chequebookBalanceFunc = f
	})
}

func WithChequebookAddressFunc(f func() common.Address) Option {
	return optionFunc(func(s *Service) {
		s.chequebookAddressFunc = f
	})
}

// NewChequebook creates the mock chequebook implementation
func NewChequebook(opts ...Option) chequebook.Service {
	mock := new(Service)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// Balance mocks the chequebook .Balance function
func (s *Service) Balance(ctx context.Context) (bal *big.Int, err error) {
	if s.chequebookBalanceFunc != nil {
		return s.chequebookBalanceFunc(ctx)
	}
	return big.NewInt(0), errors.New("Error")
}

// Deposit mocks the chequebook .Deposit function
func (s *Service) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	return common.Hash{}, errors.New("Error")
}

// WaitForDeposit mocks the chequebook .WaitForDeposit function
func (s *Service) WaitForDeposit(ctx context.Context, txHash common.Hash) error {
	return errors.New("Error")
}

// Address mocks the chequebook .Address function
func (s *Service) Address() common.Address {
	if s.chequebookAddressFunc != nil {
		return s.chequebookAddressFunc()
	}
	return common.Address{}
}

func (s *Service) Issue(beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) error {
	return errors.New("Error")
}

func (s *Service) LastCheque(beneficiary common.Address) (*chequebook.SignedCheque, error) {
	return nil, errors.New("Error")
}

func (s *Service) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return nil, errors.New("Error")
}

// Option is the option passed to the mock Chequebook service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
