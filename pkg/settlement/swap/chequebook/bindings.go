// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/sw3-bindings/v2/simpleswapfactory"
)

type SimpleSwapBinding interface {
	Balance(*bind.CallOpts) (*big.Int, error)
	Token(*bind.CallOpts) (common.Address, error)
}

type SimpleSwapBindingFunc = func(common.Address, bind.ContractBackend) (SimpleSwapBinding, error)

func NewSimpleSwapBindings(address common.Address, backend bind.ContractBackend) (SimpleSwapBinding, error) {
	return simpleswapfactory.NewERC20SimpleSwap(address, backend)
}

type ERC20Binding interface {
	BalanceOf(*bind.CallOpts, common.Address) (*big.Int, error)
}

type ERC20BindingFunc = func(common.Address, bind.ContractBackend) (ERC20Binding, error)

func NewERC20Bindings(address common.Address, backend bind.ContractBackend) (ERC20Binding, error) {
	return simpleswapfactory.NewERC20(address, backend)
}

type SimpleSwapFactoryBinding interface {
	ParseSimpleSwapDeployed(types.Log) (*simpleswapfactory.SimpleSwapFactorySimpleSwapDeployed, error)
	DeployedContracts(*bind.CallOpts, common.Address) (bool, error)
	ERC20Address(*bind.CallOpts) (common.Address, error)
}

type SimpleSwapFactoryBindingFunc = func(common.Address, bind.ContractBackend) (SimpleSwapFactoryBinding, error)

func NewSimpleSwapFactoryBindingFunc(address common.Address, backend bind.ContractBackend) (SimpleSwapFactoryBinding, error) {
	return simpleswapfactory.NewSimpleSwapFactory(address, backend)
}
