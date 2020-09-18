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
	"github.com/ethersphere/sw3-bindings/v2/simpleswapfactory"
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
	chequebookInstance SimpleSwapBinding
	ownerAddress       common.Address

	erc20Address  common.Address
	erc20ABI      abi.ABI
	erc20Instance ERC20Binding

	depositsMu sync.Mutex
	deposits   map[common.Hash]Deposit
}

func New(backend Backend, transactionService TransactionService, address common.Address, erc20Address common.Address, ownerAddress common.Address, simpleSwapBindingFunc SimpleSwapBindingFunc, erc20BindingFunc ERC20BindingFunc) (Service, error) {
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
		deposits:           make(map[common.Hash]Deposit),
	}, nil
}

func (s *service) Address() common.Address {
	return s.address
}

func (s *service) Deposit(ctx context.Context, amount *big.Int) (hash common.Hash, err error) {
	balance, err := s.erc20Instance.BalanceOf(&bind.CallOpts{
		Context: ctx,
	}, s.ownerAddress)
	if err != nil {
		return common.Hash{}, err
	}

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
