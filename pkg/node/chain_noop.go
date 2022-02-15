// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

// noOpsenderMatcher is a noOp implementation for p2p.SenderMatcher interface.
type noOpsenderMatcher struct{}

func (m *noOpsenderMatcher) Matches(context.Context, []byte, uint64, swarm.Address, bool) ([]byte, error) {
	return nil, nil
}

// noOpswapChainTransaction is a noOp implementation for transaction.Backend interface.
type noOpswapChainTransaction struct {
	log logging.Logger
}

func (m noOpswapChainTransaction) CodeAt(_ context.Context, _ common.Address, _ *big.Int) ([]byte, error) {
	m.log.Debug("swap chain no op: CodeAt")
	return common.FromHex(sw3abi.SimpleSwapFactoryDeployedBinv0_4_0), nil
}
func (m noOpswapChainTransaction) CallContract(_ context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	panic("swap chain no op: CallContract")
}
func (m noOpswapChainTransaction) HeaderByNumber(_ context.Context, _ *big.Int) (*types.Header, error) {
	m.log.Debug("swap chain no op: HeaderByNumber")
	h := new(types.Header)
	h.Time = uint64(time.Now().Unix())
	return h, nil
}
func (m noOpswapChainTransaction) PendingNonceAt(_ context.Context, _ common.Address) (uint64, error) {
	panic("swap chain no op: PendingNonceAt")
}
func (m noOpswapChainTransaction) SuggestGasPrice(_ context.Context) (*big.Int, error) {
	panic("swap chain no op: SuggestGasPrice")
}
func (m noOpswapChainTransaction) EstimateGas(_ context.Context, _ ethereum.CallMsg) (uint64, error) {
	panic("swap chain no op: EstimateGas")
}
func (m noOpswapChainTransaction) SendTransaction(_ context.Context, _ *types.Transaction) error {
	panic("swap chain no op: SendTransaction")
}
func (m noOpswapChainTransaction) TransactionReceipt(_ context.Context, _ common.Hash) (*types.Receipt, error) {
	m.log.Debug("swap chain no op: TransactionReceipt")
	r := new(types.Receipt)
	r.BlockNumber = big.NewInt(1)
	return r, nil
}
func (m noOpswapChainTransaction) TransactionByHash(_ context.Context, _ common.Hash) (tx *types.Transaction, isPending bool, err error) {
	m.log.Debug("swap chain no op: TransactionByHash")
	return nil, false, nil
}
func (m noOpswapChainTransaction) BlockNumber(_ context.Context) (uint64, error) {
	m.log.Debug("swap chain no op: BlockNumber")
	return 4, nil
}
func (m noOpswapChainTransaction) BalanceAt(_ context.Context, _ common.Address, _ *big.Int) (*big.Int, error) {
	panic("swap chain no op: BalanceAt")
}
func (m noOpswapChainTransaction) NonceAt(_ context.Context, _ common.Address, _ *big.Int) (uint64, error) {
	panic("swap chain no op: NonceAt")
}
func (m noOpswapChainTransaction) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	panic("swap chain no op: FilterLogs")
}
func (m noOpswapChainTransaction) ChainID(_ context.Context) (*big.Int, error) {
	panic("swap chain no op: ChainID")
}
func (m noOpswapChainTransaction) Close() {}

type noOpPostageContract struct{}

func (m *noOpPostageContract) CreateBatch(_ context.Context, _ *big.Int, _ uint8, _ bool, _ string) ([]byte, error) {
	return nil, postagecontract.ErrSwapChainDisabled
}
func (m *noOpPostageContract) TopUpBatch(_ context.Context, _ []byte, _ *big.Int) error {
	return postagecontract.ErrSwapChainDisabled
}
func (m *noOpPostageContract) DiluteBatch(_ context.Context, _ []byte, _ uint8) error {
	return postagecontract.ErrSwapChainDisabled
}

// noOpChequebookService is a noOp implementation for chequebook.Service interface.
type noOpChequebookService struct{}

func (m *noOpChequebookService) Deposit(_ context.Context, _ *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("swap chain disabled")
}
func (m *noOpChequebookService) Withdraw(_ context.Context, _ *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("swap chain disabled")
}
func (m *noOpChequebookService) WaitForDeposit(_ context.Context, _ common.Hash) error {
	return errors.New("swap chain disabled")
}
func (m *noOpChequebookService) Balance(_ context.Context) (*big.Int, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *noOpChequebookService) AvailableBalance(_ context.Context) (*big.Int, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *noOpChequebookService) Address() common.Address {
	return common.Address{}
}
func (m *noOpChequebookService) Issue(_ context.Context, _ common.Address, _ *big.Int, _ chequebook.SendChequeFunc) (*big.Int, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *noOpChequebookService) LastCheque(_ common.Address) (*chequebook.SignedCheque, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *noOpChequebookService) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return nil, errors.New("swap chain disabled")
}
