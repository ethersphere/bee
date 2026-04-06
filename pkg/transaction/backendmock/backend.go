// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package backendmock

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

var ErrNotImplemented = errors.New("not implemented")

type backendMock struct {
	callContract       func(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	sendTransaction    func(ctx context.Context, tx *types.Transaction) error
	suggestedFeeAndTip func(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error)
	suggestGasTipCap   func(ctx context.Context) (*big.Int, error)
	estimateGas        func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error)
	transactionReceipt func(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	pendingNonceAt     func(ctx context.Context, account common.Address) (uint64, error)
	transactionByHash  func(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error)
	blockNumber        func(ctx context.Context) (uint64, error)
	headerByNumber     func(ctx context.Context, number *big.Int) (*types.Header, error)
	balanceAt          func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)
	nonceAt            func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
}

func (m *backendMock) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if m.callContract != nil {
		return m.callContract(ctx, call, blockNumber)
	}
	return nil, ErrNotImplemented
}

func (m *backendMock) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if m.pendingNonceAt != nil {
		return m.pendingNonceAt(ctx, account)
	}
	return 0, ErrNotImplemented
}

func (m *backendMock) SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	if m.suggestedFeeAndTip != nil {
		return m.suggestedFeeAndTip(ctx, gasPrice, boostPercent)
	}
	return nil, nil, ErrNotImplemented
}

func (m *backendMock) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	if m.estimateGas != nil {
		return m.estimateGas(ctx, msg)
	}
	return 0, ErrNotImplemented
}

func (m *backendMock) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if m.sendTransaction != nil {
		return m.sendTransaction(ctx, tx)
	}
	return ErrNotImplemented
}

func (*backendMock) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return nil, ErrNotImplemented
}

func (m *backendMock) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	if m.transactionReceipt != nil {
		return m.transactionReceipt(ctx, txHash)
	}
	return nil, ErrNotImplemented
}

func (m *backendMock) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	if m.transactionByHash != nil {
		return m.transactionByHash(ctx, hash)
	}
	return nil, false, ErrNotImplemented
}

func (m *backendMock) BlockNumber(ctx context.Context) (uint64, error) {
	if m.blockNumber != nil {
		return m.blockNumber(ctx)
	}
	return 0, ErrNotImplemented
}

func (m *backendMock) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if m.headerByNumber != nil {
		return m.headerByNumber(ctx, number)
	}
	return nil, ErrNotImplemented
}

func (m *backendMock) BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
	if m.balanceAt != nil {
		return m.balanceAt(ctx, address, block)
	}
	return nil, ErrNotImplemented
}

func (m *backendMock) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	if m.nonceAt != nil {
		return m.nonceAt(ctx, account, blockNumber)
	}
	return 0, ErrNotImplemented
}

func (m *backendMock) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	if m.suggestGasTipCap != nil {
		return m.suggestGasTipCap(ctx)
	}
	return nil, ErrNotImplemented
}

func (m *backendMock) ChainID(ctx context.Context) (*big.Int, error) {
	return nil, ErrNotImplemented
}

func (m *backendMock) Close() {}

func New(opts ...Option) transaction.Backend {
	mock := new(backendMock)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// Option is the option passed to the mock Chequebook service
type Option interface {
	apply(*backendMock)
}

type optionFunc func(*backendMock)

func (f optionFunc) apply(r *backendMock) { f(r) }

func WithCallContractFunc(f func(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.callContract = f
	})
}

func WithBalanceAt(f func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.balanceAt = f
	})
}

func WithPendingNonceAtFunc(f func(ctx context.Context, account common.Address) (uint64, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.pendingNonceAt = f
	})
}

func WithSuggestedFeeAndTipFunc(f func(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.suggestedFeeAndTip = f
	})
}

func WithSuggestGasTipCapFunc(f func(ctx context.Context) (*big.Int, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.suggestGasTipCap = f
	})
}

func WithEstimateGasFunc(f func(ctx context.Context, msg ethereum.CallMsg) (gas uint64, err error)) Option {
	return optionFunc(func(s *backendMock) {
		s.estimateGas = f
	})
}

func WithTransactionReceiptFunc(f func(ctx context.Context, txHash common.Hash) (*types.Receipt, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.transactionReceipt = f
	})
}

func WithTransactionByHashFunc(f func(ctx context.Context, txHash common.Hash) (*types.Transaction, bool, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.transactionByHash = f
	})
}

func WithSendTransactionFunc(f func(ctx context.Context, tx *types.Transaction) error) Option {
	return optionFunc(func(s *backendMock) {
		s.sendTransaction = f
	})
}

func WithBlockNumberFunc(f func(context.Context) (uint64, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.blockNumber = f
	})
}

func WithHeaderbyNumberFunc(f func(ctx context.Context, number *big.Int) (*types.Header, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.headerByNumber = f
	})
}

func WithNonceAtFunc(f func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.nonceAt = f
	})
}
