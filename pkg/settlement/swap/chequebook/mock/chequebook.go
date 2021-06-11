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
	chequebookBalanceFunc          func(context.Context) (*big.Int, error)
	chequebookAvailableBalanceFunc func(context.Context) (*big.Int, error)
	chequebookAddressFunc          func() common.Address
	chequebookIssueFunc            func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error)
	chequebookWithdrawFunc         func(ctx context.Context, amount *big.Int) (hash common.Hash, err error)
	chequebookDepositFunc          func(ctx context.Context, amount *big.Int) (hash common.Hash, err error)
	lastChequeFunc                 func(common.Address) (*chequebook.SignedCheque, error)
	lastChequesFunc                func() (map[common.Address]*chequebook.SignedCheque, error)
}

// WithChequebook*Functions set the mock chequebook functions
func WithChequebookBalanceFunc(f func(ctx context.Context) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.chequebookBalanceFunc = f
	})
}

func WithChequebookAvailableBalanceFunc(f func(ctx context.Context) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.chequebookAvailableBalanceFunc = f
	})
}

func WithChequebookAddressFunc(f func() common.Address) Option {
	return optionFunc(func(s *Service) {
		s.chequebookAddressFunc = f
	})
}

func WithChequebookDepositFunc(f func(ctx context.Context, amount *big.Int) (hash common.Hash, err error)) Option {
	return optionFunc(func(s *Service) {
		s.chequebookDepositFunc = f
	})
}

func WithChequebookIssueFunc(f func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.chequebookIssueFunc = f
	})
}

func WithChequebookWithdrawFunc(f func(ctx context.Context, amount *big.Int) (hash common.Hash, err error)) Option {
	return optionFunc(func(s *Service) {
		s.chequebookWithdrawFunc = f
	})
}

func WithLastChequeFunc(f func(beneficiary common.Address) (*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastChequeFunc = f
	})
}

func WithLastChequesFunc(f func() (map[common.Address]*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastChequesFunc = f
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

func (s *Service) AvailableBalance(ctx context.Context) (bal *big.Int, err error) {
	if s.chequebookAvailableBalanceFunc != nil {
		return s.chequebookAvailableBalanceFunc(ctx)
	}
	return big.NewInt(0), errors.New("Error")
}

// Deposit mocks the chequebook .Deposit function
func (s *Service) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	if s.chequebookDepositFunc != nil {
		return s.chequebookDepositFunc(ctx, amount)
	}
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

func (s *Service) Issue(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error) {
	if s.chequebookIssueFunc != nil {
		return s.chequebookIssueFunc(ctx, beneficiary, amount, sendChequeFunc)
	}
	return big.NewInt(0), nil
}

func (s *Service) LastCheque(beneficiary common.Address) (*chequebook.SignedCheque, error) {
	if s.lastChequeFunc != nil {
		return s.lastChequeFunc(beneficiary)
	}
	return nil, errors.New("Error")
}

func (s *Service) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	if s.lastChequesFunc != nil {
		return s.lastChequesFunc()
	}
	return nil, errors.New("Error")
}

func (s *Service) Withdraw(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	return s.chequebookWithdrawFunc(ctx, amount)
}

// Option is the option passed to the mock Chequebook service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
