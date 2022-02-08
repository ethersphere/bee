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

// senderMatcherStub is a stub for p2p.SenderMatcher interface.
type senderMatcherStub struct{}

func (m *senderMatcherStub) Matches(context.Context, []byte, uint64, swarm.Address, bool) ([]byte, error) {
	return nil, nil
}

// swapChainTransactionStub is a stub for transaction.Backend interface.
type swapChainTransactionStub struct {
	log logging.Logger
}

func (m swapChainTransactionStub) CodeAt(_ context.Context, _ common.Address, _ *big.Int) ([]byte, error) {
	m.log.Debug("swap chain stub: CodeAt")
	return common.FromHex(sw3abi.SimpleSwapFactoryDeployedBinv0_4_0), nil
}
func (m swapChainTransactionStub) CallContract(_ context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	panic("swap chain stub: CallContract")
}
func (m swapChainTransactionStub) HeaderByNumber(_ context.Context, _ *big.Int) (*types.Header, error) {
	m.log.Debug("swap chain stub: HeaderByNumber")
	h := new(types.Header)
	h.Time = uint64(time.Now().Unix())
	return h, nil
}
func (m swapChainTransactionStub) PendingNonceAt(_ context.Context, _ common.Address) (uint64, error) {
	panic("swap chain stub: PendingNonceAt")
}
func (m swapChainTransactionStub) SuggestGasPrice(_ context.Context) (*big.Int, error) {
	panic("swap chain stub: SuggestGasPrice")
}
func (m swapChainTransactionStub) EstimateGas(_ context.Context, _ ethereum.CallMsg) (uint64, error) {
	panic("swap chain stub: EstimateGas")
}
func (m swapChainTransactionStub) SendTransaction(_ context.Context, _ *types.Transaction) error {
	panic("swap chain stub: SendTransaction")
}
func (m swapChainTransactionStub) TransactionReceipt(_ context.Context, _ common.Hash) (*types.Receipt, error) {
	m.log.Debug("swap chain stub: TransactionReceipt")
	r := new(types.Receipt)
	r.BlockNumber = big.NewInt(1)
	return r, nil
}
func (m swapChainTransactionStub) TransactionByHash(_ context.Context, _ common.Hash) (tx *types.Transaction, isPending bool, err error) {
	m.log.Debug("swap chain stub: TransactionByHash")
	return nil, false, nil
}
func (m swapChainTransactionStub) BlockNumber(_ context.Context) (uint64, error) {
	m.log.Debug("swap chain stub: BlockNumber")
	return 4, nil
}
func (m swapChainTransactionStub) BalanceAt(_ context.Context, _ common.Address, _ *big.Int) (*big.Int, error) {
	panic("swap chain stub: BalanceAt")
}
func (m swapChainTransactionStub) NonceAt(_ context.Context, _ common.Address, _ *big.Int) (uint64, error) {
	panic("swap chain stub: NonceAt")
}
func (m swapChainTransactionStub) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	panic("swap chain stub: FilterLogs")
}
func (m swapChainTransactionStub) ChainID(_ context.Context) (*big.Int, error) {
	panic("swap chain stub: ChainID")
}
func (m swapChainTransactionStub) Close() {}

type stubPostageContract struct{}

func (m *stubPostageContract) CreateBatch(_ context.Context, _ *big.Int, _ uint8, _ bool, _ string) ([]byte, error) {
	return nil, postagecontract.ErrSwapChainDisabled
}
func (m *stubPostageContract) TopUpBatch(_ context.Context, _ []byte, _ *big.Int) error {
	return postagecontract.ErrSwapChainDisabled
}
func (m *stubPostageContract) DiluteBatch(_ context.Context, _ []byte, _ uint8) error {
	return postagecontract.ErrSwapChainDisabled
}

// chequebookServiceStub is a stub for chequebook.Service interface.
type chequebookServiceStub struct{}

func (m *chequebookServiceStub) Deposit(_ context.Context, _ *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("swap chain disabled")
}
func (m *chequebookServiceStub) Withdraw(_ context.Context, _ *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("swap chain disabled")
}
func (m *chequebookServiceStub) WaitForDeposit(_ context.Context, _ common.Hash) error {
	return errors.New("swap chain disabled")
}
func (m *chequebookServiceStub) Balance(_ context.Context) (*big.Int, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *chequebookServiceStub) AvailableBalance(_ context.Context) (*big.Int, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *chequebookServiceStub) Address() common.Address {
	return common.Address{}
}
func (m *chequebookServiceStub) Issue(_ context.Context, _ common.Address, _ *big.Int, _ chequebook.SendChequeFunc) (*big.Int, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *chequebookServiceStub) LastCheque(_ common.Address) (*chequebook.SignedCheque, error) {
	return nil, errors.New("swap chain disabled")
}
func (m *chequebookServiceStub) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return nil, errors.New("swap chain disabled")
}
