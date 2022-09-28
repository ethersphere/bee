// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stakingcontract

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	MinimumStakeAmount = big.NewInt(1)

	erc20ABI = parseABI(sw3abi.ERC20ABIv0_3_1)
	//TODO: get ABI for staking contract and replace it below
	stakingABI = parseABI(sw3abi.ERC20ABIv0_3_1)
	//TODO: enable below mentioned topic for receiving receipts
	//stakeUpdatedTopic = stakingABI.Events["StakeUpdated"].ID

	ErrInsufficentStakeAmount = errors.New("insufficient stake amount")
	ErrInsufficientFunds      = errors.New("insufficient token balance")
	ErrNotImplemented         = errors.New("not implemented")
	ErrGetStakeFailed         = errors.New("get stake failed")

	depositStakeDescription = "Deposit Stake"
)

type Contract interface {
	DepositStake(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error
	GetStake(ctx context.Context, overlay swarm.Address) (big.Int, error)
}

type contract struct {
	owner                  common.Address
	stakingContractAddress common.Address
	bzzTokenAddress        common.Address
	transactionService     transaction.Service
}

func New(
	owner common.Address,
	stakingContractAddress common.Address,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
) Contract {
	return &contract{
		owner:                  owner,
		stakingContractAddress: stakingContractAddress,
		bzzTokenAddress:        bzzTokenAddress,
		transactionService:     transactionService,
	}
}

func (s *contract) sendTransaction(ctx context.Context, callData []byte, desc string) (*types.Receipt, error) {
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

func (s *contract) sendDepositStakeTransaction(ctx context.Context, owner common.Address, stakedAmount big.Int, nonce common.Hash) (*types.Receipt, error) {

	callData, err := stakingABI.Pack("depositStake", owner, &stakedAmount, nonce)
	if err != nil {
		return nil, err
	}

	receipt, err := s.sendTransaction(ctx, callData, depositStakeDescription)
	if err != nil {
		return nil, fmt.Errorf("deposit stake: stakedAmount %d: %w", &stakedAmount, err)
	}

	return receipt, nil
}

func (s *contract) sendGetStakeTransaction(ctx context.Context, overlay []byte) (*big.Int, error) {

	callData, err := stakingABI.Pack("stakeOfOverlay", overlay)
	if err != nil {
		return nil, err
	}

	result, err := s.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &s.stakingContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, fmt.Errorf("get stake: overlayAddress %d: %w", overlay, err)
	}

	results, err := erc20ABI.Unpack("stakeOfOverlay", result)
	if err != nil {
		return nil, err
	}
	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func (s *contract) DepositStake(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error {
	prevStakedAmount, err := s.GetStake(ctx, overlay)
	if err != nil {
		return err
	}

	if len(prevStakedAmount.Bits()) == 0 {
		if stakedAmount.Cmp(MinimumStakeAmount) == -1 {
			return ErrInsufficentStakeAmount
		}
	}

	balance, err := s.getBalance(ctx)
	if err != nil {
		return err
	}

	if balance.Cmp(&stakedAmount) <= 0 {
		return ErrInsufficientFunds
	}

	nonce := make([]byte, 32)
	_, err = rand.Read(nonce)
	if err != nil {
		return err
	}

	_, err = s.sendDepositStakeTransaction(ctx, s.owner, stakedAmount, common.BytesToHash(nonce))
	if err != nil {
		return err
	}
	//TODO: verify if we need receipt as well as service for staking
	//TODO: logic for receipt would be added here
	return nil
}

func (s *contract) GetStake(ctx context.Context, overlay swarm.Address) (big.Int, error) {
	stakedAmount, err := s.sendGetStakeTransaction(ctx, overlay.Bytes())
	if err != nil {
		return *big.NewInt(0), fmt.Errorf("%w:%v", ErrGetStakeFailed, err)
	}
	return *stakedAmount, nil
}

func (s *contract) getBalance(ctx context.Context) (*big.Int, error) {
	callData, err := erc20ABI.Pack("balanceOf", s.owner)
	if err != nil {
		return nil, err
	}

	result, err := s.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &s.bzzTokenAddress,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}

	results, err := erc20ABI.Unpack("balanceOf", result)
	if err != nil {
		return nil, err
	}
	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for staking contract: %v", err))
	}
	return cabi
}
