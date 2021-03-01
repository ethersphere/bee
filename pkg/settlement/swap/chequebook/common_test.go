// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/sw3-bindings/v3/simpleswapfactory"
)

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
	balance            func(*bind.CallOpts) (*big.Int, error)
	issuer             func(*bind.CallOpts) (common.Address, error)
	totalPaidOut       func(o *bind.CallOpts) (*big.Int, error)
	paidOut            func(*bind.CallOpts, common.Address) (*big.Int, error)
	parseChequeCashed  func(types.Log) (*simpleswapfactory.ERC20SimpleSwapChequeCashed, error)
	parseChequeBounced func(types.Log) (*simpleswapfactory.ERC20SimpleSwapChequeBounced, error)
}

func (m *simpleSwapBindingMock) Balance(o *bind.CallOpts) (*big.Int, error) {
	return m.balance(o)
}

func (m *simpleSwapBindingMock) Issuer(o *bind.CallOpts) (common.Address, error) {
	return m.issuer(o)
}

func (m *simpleSwapBindingMock) TotalPaidOut(o *bind.CallOpts) (*big.Int, error) {
	return m.totalPaidOut(o)
}

func (m *simpleSwapBindingMock) ParseChequeCashed(l types.Log) (*simpleswapfactory.ERC20SimpleSwapChequeCashed, error) {
	return m.parseChequeCashed(l)
}

func (m *simpleSwapBindingMock) ParseChequeBounced(l types.Log) (*simpleswapfactory.ERC20SimpleSwapChequeBounced, error) {
	return m.parseChequeBounced(l)
}

func (m *simpleSwapBindingMock) PaidOut(o *bind.CallOpts, c common.Address) (*big.Int, error) {
	return m.paidOut(o, c)
}

type erc20BindingMock struct {
	balanceOf func(*bind.CallOpts, common.Address) (*big.Int, error)
}

func (m *erc20BindingMock) BalanceOf(o *bind.CallOpts, a common.Address) (*big.Int, error) {
	return m.balanceOf(o, a)
}

type chequeSignerMock struct {
	sign func(cheque *chequebook.Cheque) ([]byte, error)
}

func (m *chequeSignerMock) Sign(cheque *chequebook.Cheque) ([]byte, error) {
	return m.sign(cheque)
}

type factoryMock struct {
	erc20Address     func(ctx context.Context) (common.Address, error)
	deploy           func(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int) (common.Hash, error)
	waitDeployed     func(ctx context.Context, txHash common.Hash) (common.Address, error)
	verifyBytecode   func(ctx context.Context) error
	verifyChequebook func(ctx context.Context, chequebook common.Address) error
}

// ERC20Address returns the token for which this factory deploys chequebooks.
func (m *factoryMock) ERC20Address(ctx context.Context) (common.Address, error) {
	return m.erc20Address(ctx)
}

func (m *factoryMock) Deploy(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int) (common.Hash, error) {
	return m.deploy(ctx, issuer, defaultHardDepositTimeoutDuration)
}

func (m *factoryMock) WaitDeployed(ctx context.Context, txHash common.Hash) (common.Address, error) {
	return m.waitDeployed(ctx, txHash)
}

// VerifyBytecode checks that the factory is valid.
func (m *factoryMock) VerifyBytecode(ctx context.Context) error {
	return m.verifyBytecode(ctx)
}

// VerifyChequebook checks that the supplied chequebook has been deployed by this factory.
func (m *factoryMock) VerifyChequebook(ctx context.Context, chequebook common.Address) error {
	return m.verifyChequebook(ctx, chequebook)
}
