// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package backendnoop

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/prometheus/client_golang/prometheus"
)

var _ transaction.Backend = (*Backend)(nil)

// Backend is a no-op implementation for transaction.Backend interface.
// It's used when the blockchain functionality is disabled.
type Backend struct {
	chainID int64
}

// New creates a new no-op backend with the specified chain ID.
func New(chainID int64) transaction.Backend {
	return &Backend{
		chainID: chainID,
	}
}

func (b *Backend) Metrics() []prometheus.Collector {
	return nil
}

func (b *Backend) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return nil, errors.New("disabled chain backend")
}

func (b *Backend) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	h := new(types.Header)
	h.Time = uint64(time.Now().Unix())
	return h, nil
}

func (b *Backend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	panic("chain no op: PendingNonceAt")
}

func (b *Backend) SuggestedFeeAndTip(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
	panic("chain no op: SuggestedFeeAndTip")
}

func (b *Backend) SuggestGasTipCap(context.Context) (*big.Int, error) {
	panic("chain no op: SuggestGasTipCap")
}

func (b *Backend) EstimateGasAtBlock(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) (uint64, error) {
	panic("chain no op: EstimateGas")
}

func (b *Backend) SendTransaction(context.Context, *types.Transaction) error {
	panic("chain no op: SendTransaction")
}

func (b *Backend) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	r := new(types.Receipt)
	r.BlockNumber = big.NewInt(1)
	return r, nil
}

func (b *Backend) TransactionByHash(context.Context, common.Hash) (tx *types.Transaction, isPending bool, err error) {
	panic("chain no op: TransactionByHash")
}

func (b *Backend) BlockNumber(context.Context) (uint64, error) {
	return 4, nil
}

func (b *Backend) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return nil, postagecontract.ErrChainDisabled
}

func (b *Backend) NonceAt(context.Context, common.Address, *big.Int) (uint64, error) {
	panic("chain no op: NonceAt")
}

func (b *Backend) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	panic("chain no op: FilterLogs")
}

func (b *Backend) ChainID(context.Context) (*big.Int, error) {
	return big.NewInt(b.chainID), nil
}

func (b *Backend) Close() {}
