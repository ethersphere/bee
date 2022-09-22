// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stakingcontract

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	erc20ABI = parseABI(sw3abi.ERC20ABIv0_3_1)
	//TODO: get ABI for staking contract
	stakingABI = parseABI(sw3abi.ERC20ABIv0_3_1)

	approveDescription      = "Approve tokens for staking operations"
	depositStakeDescription = "Deposit Stake"
)

type Interface interface {
	DepositStake(ctx context.Context, stakedAmount *big.Int) error
}

type stakingContract struct {
	owner                  common.Address
	stakingContractAddress common.Address
	bzzTokenAddress        common.Address
	transactionService     transaction.Service
	//stakingService         staking.Service
}

func New(
	owner common.Address,
	stakingContractAddress common.Address,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
//stakingService         staking.Service,
) Interface {
	return &stakingContract{
		owner:                  owner,
		stakingContractAddress: stakingContractAddress,
		bzzTokenAddress:        bzzTokenAddress,
		transactionService:     transactionService,
		//stakingService: stakingService,
	}
}

func (s stakingContract) DepositStake(ctx context.Context, stakedAmount *big.Int) error {
	//TODO implement me
	panic("implement me")
}

func (s *stakingContract) sendApproveTransaction(ctx context.Context, amount *big.Int) (*types.Receipt, error) {
	callData, err := erc20ABI.Pack("approve", s.stakingContractAddress, amount)
	if err != nil {
		return nil, err
	}

	txHash, err := s.transactionService.Send(ctx, &transaction.TxRequest{
		To:          &s.bzzTokenAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    65000,
		Value:       big.NewInt(0),
		Description: approveDescription,
	})
	if err != nil {
		return nil, err
	}

	receipt, err := s.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}

	if receipt.Status == 0 {
		return nil, transaction.ErrTransactionReverted
	}

	return receipt, nil
}

func (s *stakingContract) sendTransaction(ctx context.Context, callData []byte, desc string) (*types.Receipt, error) {
	request := &transaction.TxRequest{
		To:          &s.stakingContractAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    1600000,
		Value:       big.NewInt(0),
		Description: desc,
	}

	txHash, err := s.transactionService.Send(ctx, request)
	if err != nil {
		return nil, err
	}

	receipt, err := s.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}

	if receipt.Status == 0 {
		return nil, transaction.ErrTransactionReverted
	}

	return receipt, nil
}

func (s *stakingContract) sendDepositStakeTransaction(ctx context.Context, owner common.Address, stakedAmount *big.Int, nonce common.Hash) (*types.Receipt, error) {

	callData, err := stakingABI.Pack("depositStake", owner, stakedAmount, nonce)
	if err != nil {
		return nil, err
	}

	receipt, err := s.sendTransaction(ctx, callData, depositStakeDescription)
	if err != nil {
		return nil, fmt.Errorf("deposit stake: stakedAmount %d: %w", stakedAmount, err)
	}

	return receipt, nil
}

//TODO: getStakedAmount

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for staking contract: %v", err))
	}
	return cabi
}
