// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/sw3-bindings/simpleswapfactory"
)

type Deposit struct {
	amount *big.Int
}

type Service interface {
	Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error)
	DepositStatus(hash common.Hash) (*Deposit, error)
	WaitForDeposit(ctx context.Context, txHash common.Hash) error
	Balance(ctx context.Context) (*big.Int, error)
	Address() common.Address
}

type service struct {
	backend            Backend
	transactionService TransactionService
	address            common.Address

	chequebookABI      abi.ABI
	chequebookInstance *simpleswapfactory.ERC20SimpleSwap

	erc20ABI abi.ABI

	depositsMu sync.Mutex
	deposits   map[common.Hash]Deposit
}

func New(backend Backend, transactionService TransactionService, address common.Address) (Service, error) {
	chequebookABI, err := abi.JSON(strings.NewReader(simpleswapfactory.ERC20SimpleSwapABI))
	if err != nil {
		return nil, err
	}

	erc20ABI, err := abi.JSON(strings.NewReader(simpleswapfactory.ERC20ABI))
	if err != nil {
		return nil, err
	}

	chequebookInstance, err := simpleswapfactory.NewERC20SimpleSwap(address, backend)
	if err != nil {
		return nil, err
	}

	return &service{
		backend:            backend,
		transactionService: transactionService,
		address:            address,
		chequebookABI:      chequebookABI,
		chequebookInstance: chequebookInstance,
		erc20ABI:           erc20ABI,
		deposits:           make(map[common.Hash]Deposit),
	}, nil
}

func (s *service) Address() common.Address {
	return s.address
}

func (s *service) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	callData, err := s.erc20ABI.Pack("transfer", s.address, amount)
	if err != nil {
		return common.Hash{}, err
	}

	erc20Address, err := s.chequebookInstance.Token(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return common.Hash{}, err
	}

	request := &TxRequest{
		To:       erc20Address,
		Data:     callData,
		GasPrice: nil,
		GasLimit: 0,
		Value:    big.NewInt(0),
	}

	txHash, err := s.transactionService.Send(ctx, request)
	if err != nil {
		return common.Hash{}, err
	}

	s.depositsMu.Lock()
	defer s.depositsMu.Unlock()

	s.deposits[txHash] = Deposit{
		amount: amount,
	}

	return txHash, nil
}

func (s *service) DepositStatus(txHash common.Hash) (*Deposit, error) {
	s.depositsMu.Lock()
	defer s.depositsMu.Unlock()

	deposit, ok := s.deposits[txHash]
	if !ok {
		return nil, errors.New("deposit not found")
	}

	return &deposit, nil
}

func (s *service) Balance(ctx context.Context) (*big.Int, error) {
	return s.chequebookInstance.Balance(&bind.CallOpts{
		Context: ctx,
	})
}

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
