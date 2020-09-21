// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/sw3-bindings/v2/simpleswapfactory"
)

// Service is the main interface for interacting with the nodes chequebook
type Service interface {
	// Deposit starts depositing erc20 token into the chequebook. This returns once the transactions has been broadcast.
	Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error)
	// WaitForDeposit waits for the deposit transaction to confirm and verifies the result
	WaitForDeposit(ctx context.Context, txHash common.Hash) error
	// Balance returns the token balance of the chequebook
	Balance(ctx context.Context) (*big.Int, error)
	// Address returns the address of the used chequebook contract
	Address() common.Address
}

type service struct {
	backend            Backend
	transactionService TransactionService

	address            common.Address
	chequebookABI      abi.ABI
	chequebookInstance SimpleSwapBinding
	ownerAddress       common.Address

	erc20Address  common.Address
	erc20ABI      abi.ABI
	erc20Instance ERC20Binding
}

// New creates a new chequebook service for the provided chequebook contract
func New(backend Backend, transactionService TransactionService, address, erc20Address, ownerAddress common.Address, simpleSwapBindingFunc SimpleSwapBindingFunc, erc20BindingFunc ERC20BindingFunc) (Service, error) {
	chequebookABI, err := abi.JSON(strings.NewReader(simpleswapfactory.ERC20SimpleSwapABI))
	if err != nil {
		return nil, err
	}

	erc20ABI, err := abi.JSON(strings.NewReader(simpleswapfactory.ERC20ABI))
	if err != nil {
		return nil, err
	}

	chequebookInstance, err := simpleSwapBindingFunc(address, backend)
	if err != nil {
		return nil, err
	}

	erc20Instance, err := erc20BindingFunc(erc20Address, backend)
	if err != nil {
		return nil, err
	}

	return &service{
		backend:            backend,
		transactionService: transactionService,
		address:            address,
		chequebookABI:      chequebookABI,
		chequebookInstance: chequebookInstance,
		ownerAddress:       ownerAddress,
		erc20Address:       erc20Address,
		erc20ABI:           erc20ABI,
		erc20Instance:      erc20Instance,
	}, nil
}

// Address returns the address of the used chequebook contract
func (s *service) Address() common.Address {
	return s.address
}

// Deposit starts depositing erc20 token into the chequebook. This returns once the transactions has been broadcast.
func (s *service) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	balance, err := s.erc20Instance.BalanceOf(&bind.CallOpts{
		Context: ctx,
	}, s.ownerAddress)
	if err != nil {
		return common.Hash{}, err
	}

	// check we can afford this so we don't waste gas
	if balance.Cmp(amount) < 0 {
		return common.Hash{}, errors.New("insufficient token balance")
	}

	callData, err := s.erc20ABI.Pack("transfer", s.address, amount)
	if err != nil {
		return common.Hash{}, err
	}

	request := &TxRequest{
		To:       s.erc20Address,
		Data:     callData,
		GasPrice: nil,
		GasLimit: 0,
		Value:    big.NewInt(0),
	}

	txHash, err := s.transactionService.Send(ctx, request)
	if err != nil {
		return common.Hash{}, err
	}

	return txHash, nil
}

// Balance returns the token balance of the chequebook
func (s *service) Balance(ctx context.Context) (*big.Int, error) {
	return s.chequebookInstance.Balance(&bind.CallOpts{
		Context: ctx,
	})
}

// WaitForDeposit waits for the deposit transaction to confirm and verifies the result
func (s *service) WaitForDeposit(ctx context.Context, txHash common.Hash) error {
	receipt, err := s.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return err
	}
	if receipt.Status != 1 {
		return ErrTransactionReverted
	}
	return nil
}
