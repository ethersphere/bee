// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package backendsimulation

import (
	"context"
	"errors"
	"maps"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

var ErrNotImplemented = errors.New("not implemented")

type AccountAtKey struct {
	BlockNumber uint64
	Account     common.Address
}

type simulatedBackend struct {
	blockNumber uint64

	receipts map[common.Hash]*types.Receipt
	noncesAt map[AccountAtKey]uint64

	blocks []Block
	step   uint64
}

type Block struct {
	Number   uint64
	Receipts map[common.Hash]*types.Receipt
	NoncesAt map[AccountAtKey]uint64
}

type Option interface {
	apply(*simulatedBackend)
}

type optionFunc func(*simulatedBackend)

func (f optionFunc) apply(r *simulatedBackend) { f(r) }

func WithBlocks(blocks ...Block) Option {
	return optionFunc(func(sb *simulatedBackend) {
		sb.blocks = blocks
	})
}

func New(options ...Option) transaction.Backend {
	m := &simulatedBackend{
		receipts: make(map[common.Hash]*types.Receipt),
		noncesAt: make(map[AccountAtKey]uint64),

		blockNumber: 0,
	}
	for _, opt := range options {
		opt.apply(m)
	}

	return m
}

func (m *simulatedBackend) advanceBlock() {
	if m.step >= uint64(len(m.blocks)) {
		return
	}
	block := m.blocks[m.step]
	m.step++

	m.blockNumber = block.Number

	if block.Receipts != nil {
		maps.Copy(m.receipts, block.Receipts)
	}

	if block.NoncesAt != nil {
		maps.Copy(m.noncesAt, block.NoncesAt)
	}
}

func (*simulatedBackend) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (m *simulatedBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 0, ErrNotImplemented
}

func (m *simulatedBackend) SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	return nil, nil, ErrNotImplemented
}

func (m *simulatedBackend) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	return 0, ErrNotImplemented
}

func (m *simulatedBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return ErrNotImplemented
}

func (*simulatedBackend) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return nil, ErrNotImplemented
}

func (m *simulatedBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	receipt, ok := m.receipts[txHash]
	if ok {
		return receipt, nil
	} else {
		return nil, ethereum.NotFound
	}
}

func (m *simulatedBackend) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	return nil, false, ErrNotImplemented
}

func (m *simulatedBackend) BlockNumber(ctx context.Context) (uint64, error) {
	m.advanceBlock()
	return m.blockNumber, nil
}

func (m *simulatedBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return nil, ErrNotImplemented
}

func (m *simulatedBackend) BalanceAt(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
	return nil, ErrNotImplemented
}

func (m *simulatedBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	nonce, ok := m.noncesAt[AccountAtKey{Account: account, BlockNumber: blockNumber.Uint64()}]
	if ok {
		return nonce, nil
	} else {
		return 0, nil
	}
}

func (m *simulatedBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return nil, ErrNotImplemented
}

func (m *simulatedBackend) ChainID(ctx context.Context) (*big.Int, error) {
	return nil, ErrNotImplemented
}

func (m *simulatedBackend) Close() {}
