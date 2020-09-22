// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto/eip712"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/sw3-bindings/v2/simpleswapfactory"
)

type backendMock struct {
	codeAt             func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error)
	sendTransaction    func(ctx context.Context, tx *types.Transaction) error
	suggestGasPrice    func(ctx context.Context) (*big.Int, error)
	estimateGas        func(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error)
	transactionReceipt func(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	pendingNonceAt     func(ctx context.Context, account common.Address) (uint64, error)
}

func (m *backendMock) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	return m.codeAt(ctx, contract, blockNumber)
}

func (*backendMock) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	panic("not implemented")
}

func (*backendMock) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	panic("not implemented")
}

func (m *backendMock) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return m.pendingNonceAt(ctx, account)
}

func (m *backendMock) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return m.suggestGasPrice(ctx)
}

func (m *backendMock) EstimateGas(ctx context.Context, call ethereum.CallMsg) (gas uint64, err error) {
	return m.estimateGas(ctx, call)
}

func (m *backendMock) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return m.sendTransaction(ctx, tx)
}

func (*backendMock) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	panic("not implemented")
}

func (*backendMock) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	panic("not implemented")
}

func (m *backendMock) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return m.transactionReceipt(ctx, txHash)
}

type transactionServiceMock struct {
	send           func(ctx context.Context, request *chequebook.TxRequest) (txHash common.Hash, err error)
	waitForReceipt func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error)
}

func (m *transactionServiceMock) Send(ctx context.Context, request *chequebook.TxRequest) (txHash common.Hash, err error) {
	return m.send(ctx, request)
}

func (m *transactionServiceMock) WaitForReceipt(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
	return m.waitForReceipt(ctx, txHash)
}

type simpleSwapFactoryBindingMock struct {
	erc20Address            func(*bind.CallOpts) (common.Address, error)
	deployedContracts       func(*bind.CallOpts, common.Address) (bool, error)
	parseSimpleSwapDeployed func(types.Log) (*simpleswapfactory.SimpleSwapFactorySimpleSwapDeployed, error)
}

func (m *simpleSwapFactoryBindingMock) ParseSimpleSwapDeployed(l types.Log) (*simpleswapfactory.SimpleSwapFactorySimpleSwapDeployed, error) {
	return m.parseSimpleSwapDeployed(l)
}

func (m *simpleSwapFactoryBindingMock) DeployedContracts(o *bind.CallOpts, a common.Address) (bool, error) {
	return m.deployedContracts(o, a)
}
func (m *simpleSwapFactoryBindingMock) ERC20Address(o *bind.CallOpts) (common.Address, error) {
	return m.erc20Address(o)
}

type simpleSwapBindingMock struct {
	balance func(*bind.CallOpts) (*big.Int, error)
}

func (m *simpleSwapBindingMock) Balance(o *bind.CallOpts) (*big.Int, error) {
	return m.balance(o)
}

func (m *simpleSwapBindingMock) Issuer(*bind.CallOpts) (common.Address, error) {
	return common.Address{}, nil
}

type erc20BindingMock struct {
	balanceOf func(*bind.CallOpts, common.Address) (*big.Int, error)
}

func (m *erc20BindingMock) BalanceOf(o *bind.CallOpts, a common.Address) (*big.Int, error) {
	return m.balanceOf(o, a)
}

type signerMock struct {
	signTx        func(transaction *types.Transaction) (*types.Transaction, error)
	signTypedData func(*eip712.TypedData) ([]byte, error)
}

func (*signerMock) EthereumAddress() (common.Address, error) {
	return common.Address{}, nil
}

func (*signerMock) Sign(data []byte) ([]byte, error) {
	return nil, nil
}

func (m *signerMock) SignTx(transaction *types.Transaction) (*types.Transaction, error) {
	return m.signTx(transaction)
}

func (*signerMock) PublicKey() (*ecdsa.PublicKey, error) {
	return nil, nil
}

func (m *signerMock) SignTypedData(d *eip712.TypedData) ([]byte, error) {
	return m.signTypedData(d)
}
