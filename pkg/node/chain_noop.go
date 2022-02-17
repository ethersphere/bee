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

// noOpChainTransaction is a noOp implementation for transaction.Backend interface.
type noOpChainTransaction struct {
	log logging.Logger
}

func (m noOpChainTransaction) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return common.FromHex(sw3abi.SimpleSwapFactoryDeployedBinv0_4_0), nil
}
func (m noOpChainTransaction) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	panic("chain no op: CallContract")
}
func (m noOpChainTransaction) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	h := new(types.Header)
	h.Time = uint64(time.Now().Unix())
	return h, nil
}
func (m noOpChainTransaction) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	panic("chain no op: PendingNonceAt")
}
func (m noOpChainTransaction) SuggestGasPrice(context.Context) (*big.Int, error) {
	panic("chain no op: SuggestGasPrice")
}
func (m noOpChainTransaction) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	panic("chain no op: EstimateGas")
}
func (m noOpChainTransaction) SendTransaction(context.Context, *types.Transaction) error {
	panic("chain no op: SendTransaction")
}
func (m noOpChainTransaction) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	r := new(types.Receipt)
	r.BlockNumber = big.NewInt(1)
	return r, nil
}
func (m noOpChainTransaction) TransactionByHash(context.Context, common.Hash) (tx *types.Transaction, isPending bool, err error) {
	return nil, false, nil
}
func (m noOpChainTransaction) BlockNumber(context.Context) (uint64, error) {
	return 4, nil
}
func (m noOpChainTransaction) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	panic("chain no op: BalanceAt")
}
func (m noOpChainTransaction) NonceAt(context.Context, common.Address, *big.Int) (uint64, error) {
	panic("chain no op: NonceAt")
}
func (m noOpChainTransaction) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	panic("chain no op: FilterLogs")
}
func (m noOpChainTransaction) ChainID(context.Context) (*big.Int, error) {
	panic("chain no op: ChainID")
}
func (m noOpChainTransaction) Close() {}

type noOpPostageContract struct{}

func (m *noOpPostageContract) CreateBatch(context.Context, *big.Int, uint8, bool, string) ([]byte, error) {
	return nil, postagecontract.ErrChainDisabled
}
func (m *noOpPostageContract) TopUpBatch(context.Context, []byte, *big.Int) error {
	return postagecontract.ErrChainDisabled
}
func (m *noOpPostageContract) DiluteBatch(context.Context, []byte, uint8) error {
	return postagecontract.ErrChainDisabled
}

// noOpChequebookService is a noOp implementation for chequebook.Service interface.
type noOpChequebookService struct{}

func (m *noOpChequebookService) Deposit(context.Context, *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("chain disabled")
}
func (m *noOpChequebookService) Withdraw(context.Context, *big.Int) (hash common.Hash, err error) {
	return hash, errors.New("chain disabled")
}
func (m *noOpChequebookService) WaitForDeposit(context.Context, common.Hash) error {
	return errors.New("chain disabled")
}
func (m *noOpChequebookService) Balance(context.Context) (*big.Int, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) AvailableBalance(context.Context) (*big.Int, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) Address() common.Address {
	return common.Address{}
}
func (m *noOpChequebookService) Issue(context.Context, common.Address, *big.Int, chequebook.SendChequeFunc) (*big.Int, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) LastCheque(common.Address) (*chequebook.SignedCheque, error) {
	return nil, errors.New("chain disabled")
}
func (m *noOpChequebookService) LastCheques() (map[common.Address]*chequebook.SignedCheque, error) {
	return nil, errors.New("chain disabled")
}
