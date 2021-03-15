// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/sw3-bindings/v3/simpleswapfactory"
)

// SimpleSwapBinding is the interface for the generated go bindings for ERC20SimpleSwap
type SimpleSwapBinding interface {
	Balance(*bind.CallOpts) (*big.Int, error)
	Issuer(*bind.CallOpts) (common.Address, error)
	TotalPaidOut(*bind.CallOpts) (*big.Int, error)
	PaidOut(*bind.CallOpts, common.Address) (*big.Int, error)
}

type SimpleSwapBindingFunc = func(common.Address, bind.ContractBackend) (SimpleSwapBinding, error)

// NewSimpleSwapBindings generates the default go bindings
func NewSimpleSwapBindings(address common.Address, backend bind.ContractBackend) (SimpleSwapBinding, error) {
	return simpleswapfactory.NewERC20SimpleSwap(address, backend)
}
