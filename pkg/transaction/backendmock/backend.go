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
	"github.com/ethersphere/bee/pkg/transaction"
)

type backendMock struct {
	codeAt             func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error)
	sendTransaction    func(ctx context.Context, tx *types.Transaction) error
	suggestGasPrice    func(ctx context.Context) (*big.Int, error)
	estimateGas        func(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error)
	transactionReceipt func(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	pendingNonceAt     func(ctx context.Context, account common.Address) (uint64, error)
	transactionByHash  func(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error)
	blockNumber        func(ctx context.Context) (uint64, error)
	blockByNumber      func(ctx context.Context, number *big.Int) (*types.Block, error)
	headerByNumber     func(ctx context.Context, number *big.Int) (*types.Header, error)
	balanceAt          func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)
	nonceAt            func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
}

func (m *backendMock) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	if m.codeAt != nil {
		return m.codeAt(ctx, contract, blockNumber)
	}
	return nil, errors.New("not implemented")
}

func (*backendMock) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (*backendMock) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (m *backendMock) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if m.pendingNonceAt != nil {
		return m.pendingNonceAt(ctx, account)
	}
	return 0, errors.New("not implemented")
}

func (m *backendMock) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	if m.suggestGasPrice != nil {
		return m.suggestGasPrice(ctx)
	}
	return nil, errors.New("not implemented")
}

func (m *backendMock) EstimateGas(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error) {
	if m.estimateGas != nil {
		return m.estimateGas(ctx, call)
	}
	return 0, errors.New("not implemented")
}

func (m *backendMock) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if m.sendTransaction != nil {
		return m.sendTransaction(ctx, tx)
	}
	return errors.New("not implemented")
}

func (*backendMock) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return nil, errors.New("not implemented")
}

func (*backendMock) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	return nil, errors.New("not implemented")
}

func (m *backendMock) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	if m.transactionReceipt != nil {
		return m.transactionReceipt(ctx, txHash)
	}
	return nil, errors.New("not implemented")
}

func (m *backendMock) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	if m.transactionByHash != nil {
		return m.transactionByHash(ctx, hash)
	}
	return nil, false, errors.New("not implemented")
}

func (m *backendMock) BlockNumber(ctx context.Context) (uint64, error) {
	if m.blockNumber != nil {
		return m.blockNumber(ctx)
	}
	return 0, errors.New("not implemented")
}

func (m *backendMock) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	if m.blockNumber != nil {
		return m.blockByNumber(ctx, number)
	}
	return nil, errors.New("not implemented")
}

func (m *backendMock) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if m.headerByNumber != nil {
		return m.headerByNumber(ctx, number)
	}
	return nil, errors.New("not implemented")
}

func (m *backendMock) BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
	if m.balanceAt != nil {
		return m.balanceAt(ctx, address, block)
	}
	return nil, errors.New("not implemented")
}
func (m *backendMock) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	if m.nonceAt != nil {
		return m.nonceAt(ctx, account, blockNumber)
	}
	return 0, errors.New("not implemented")
}

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

func WithCodeAtFunc(f func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.codeAt = f
	})
}

func WithPendingNonceAtFunc(f func(ctx context.Context, account common.Address) (uint64, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.pendingNonceAt = f
	})
}

func WithSuggestGasPriceFunc(f func(ctx context.Context) (*big.Int, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.suggestGasPrice = f
	})
}

func WithEstimateGasFunc(f func(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error)) Option {
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

func WithBlockByNumberFunc(f func(ctx context.Context, number *big.Int) (*types.Block, error)) Option {
	return optionFunc(func(s *backendMock) {
		s.blockByNumber = f
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
