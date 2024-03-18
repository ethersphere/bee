// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package staking

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	MinimumStakeAmount = big.NewInt(100000000000000000)

	erc20ABI = abiutil.MustParseABI(sw3abi.ERC20ABIv0_6_5)

	ErrInsufficientStakeAmount = errors.New("insufficient stake amount")
	ErrInsufficientFunds       = errors.New("insufficient token balance")
	ErrInsufficientStake       = errors.New("insufficient stake")
	ErrNotImplemented          = errors.New("not implemented")
	ErrNotPaused               = errors.New("contract is not paused")

	approveDescription       = "Approve tokens for stake deposit operations"
	depositStakeDescription  = "Deposit Stake"
	withdrawStakeDescription = "Withdraw stake"
)

type Contract interface {
	DepositStake(ctx context.Context, stakedAmount *big.Int) (common.Hash, error)
	GetStake(ctx context.Context) (*big.Int, error)
	WithdrawAllStake(ctx context.Context) (common.Hash, error)
	RedistributionStatuser
}

type RedistributionStatuser interface {
	IsOverlayFrozen(ctx context.Context, block uint64) (bool, error)
}

type contract struct {
	overlay                swarm.Address
	owner                  common.Address
	stakingContractAddress common.Address
	stakingContractABI     abi.ABI
	bzzTokenAddress        common.Address
	transactionService     transaction.Service
	overlayNonce           common.Hash
}

func New(
	overlay swarm.Address,
	owner common.Address,
	stakingContractAddress common.Address,
	stakingContractABI abi.ABI,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
	nonce common.Hash,
) Contract {
	return &contract{
		overlay:                overlay,
		owner:                  owner,
		stakingContractAddress: stakingContractAddress,
		stakingContractABI:     stakingContractABI,
		bzzTokenAddress:        bzzTokenAddress,
		transactionService:     transactionService,
		overlayNonce:           nonce,
	}
}

func (c *contract) sendApproveTransaction(ctx context.Context, amount *big.Int) (receipt *types.Receipt, err error) {
	callData, err := erc20ABI.Pack("approve", c.stakingContractAddress, amount)
	if err != nil {
		return nil, err
	}

	request := &transaction.TxRequest{
		To:          &c.bzzTokenAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    65000,
		Value:       big.NewInt(0),
		Description: approveDescription,
	}

	defer func() {
		err = c.transactionService.UnwrapABIError(
			ctx,
			request,
			err,
			c.stakingContractABI.Errors,
		)
	}()

	txHash, err := c.transactionService.Send(ctx, request, 0)
	if err != nil {
		return nil, err
	}

	receipt, err = c.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}

	if receipt.Status == 0 {
		return nil, transaction.ErrTransactionReverted
	}

	return receipt, nil
}

func (c *contract) sendTransaction(ctx context.Context, callData []byte, desc string) (receipt *types.Receipt, err error) {
	request := &transaction.TxRequest{
		To:          &c.stakingContractAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    sctx.GetGasLimit(ctx),
		Value:       big.NewInt(0),
		Description: desc,
	}

	defer func() {
		err = c.transactionService.UnwrapABIError(
			ctx,
			request,
			err,
			c.stakingContractABI.Errors,
		)
	}()

	txHash, err := c.transactionService.Send(ctx, request, transaction.DefaultTipBoostPercent)
	if err != nil {
		return nil, err
	}

	receipt, err = c.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}

	if receipt.Status == 0 {
		return nil, transaction.ErrTransactionReverted
	}

	return receipt, nil
}

func (c *contract) sendDepositStakeTransaction(ctx context.Context, owner common.Address, stakedAmount *big.Int, nonce common.Hash) (*types.Receipt, error) {
	callData, err := c.stakingContractABI.Pack("depositStake", owner, nonce, stakedAmount)
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, depositStakeDescription)
	if err != nil {
		return nil, fmt.Errorf("deposit stake: stakedAmount %d: %w", stakedAmount, err)
	}

	return receipt, nil
}

func (c *contract) getStake(ctx context.Context, overlay swarm.Address) (*big.Int, error) {
	var overlayAddr [32]byte
	copy(overlayAddr[:], overlay.Bytes())
	callData, err := c.stakingContractABI.Pack("stakeOfOverlay", overlayAddr)
	if err != nil {
		return nil, err
	}
	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.stakingContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, fmt.Errorf("get stake: overlayAddress %d: %w", overlay, err)
	}

	results, err := c.stakingContractABI.Unpack("stakeOfOverlay", result)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, errors.New("unexpected empty results")
	}

	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func (c *contract) DepositStake(ctx context.Context, stakedAmount *big.Int) (common.Hash, error) {
	prevStakedAmount, err := c.GetStake(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	if len(prevStakedAmount.Bits()) == 0 {
		if stakedAmount.Cmp(MinimumStakeAmount) == -1 {
			return common.Hash{}, ErrInsufficientStakeAmount
		}
	}

	balance, err := c.getBalance(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	if balance.Cmp(stakedAmount) < 0 {
		return common.Hash{}, ErrInsufficientFunds
	}

	_, err = c.sendApproveTransaction(ctx, stakedAmount)
	if err != nil {
		return common.Hash{}, err
	}

	receipt, err := c.sendDepositStakeTransaction(ctx, c.owner, stakedAmount, c.overlayNonce)
	if err != nil {
		return common.Hash{}, err
	}

	return receipt.TxHash, nil
}

func (c *contract) GetStake(ctx context.Context) (*big.Int, error) {
	stakedAmount, err := c.getStake(ctx, c.overlay)
	if err != nil {
		return nil, fmt.Errorf("staking contract: failed to get stake: %w", err)
	}
	return stakedAmount, nil
}

func (c *contract) getBalance(ctx context.Context) (*big.Int, error) {
	callData, err := erc20ABI.Pack("balanceOf", c.owner)
	if err != nil {
		return nil, err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.bzzTokenAddress,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}

	results, err := erc20ABI.Unpack("balanceOf", result)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, errors.New("unexpected empty results")
	}

	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func (c *contract) WithdrawAllStake(ctx context.Context) (txHash common.Hash, err error) {
	isPaused, err := c.paused(ctx)
	if err != nil {
		return
	}
	if !isPaused {
		return common.Hash{}, ErrNotPaused
	}

	stakedAmount, err := c.getStake(ctx, c.overlay)
	if err != nil {
		return
	}

	if stakedAmount.Cmp(big.NewInt(0)) <= 0 {
		return common.Hash{}, ErrInsufficientStake
	}

	_, err = c.sendApproveTransaction(ctx, stakedAmount)
	if err != nil {
		return common.Hash{}, err
	}

	receipt, err := c.withdrawFromStake(ctx, stakedAmount)
	if err != nil {
		return common.Hash{}, err
	}
	if receipt != nil {
		txHash = receipt.TxHash
	}
	return txHash, nil
}

func (c *contract) withdrawFromStake(ctx context.Context, stakedAmount *big.Int) (*types.Receipt, error) {
	var overlayAddr [32]byte
	copy(overlayAddr[:], c.overlay.Bytes())

	callData, err := c.stakingContractABI.Pack("withdrawFromStake", overlayAddr, stakedAmount)
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, withdrawStakeDescription)
	if err != nil {
		return nil, fmt.Errorf("withdraw stake: stakedAmount %d: %w", stakedAmount, err)
	}

	return receipt, nil
}

func (c *contract) paused(ctx context.Context) (bool, error) {
	callData, err := c.stakingContractABI.Pack("paused")
	if err != nil {
		return false, err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.stakingContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := c.stakingContractABI.Unpack("paused", result)
	if err != nil {
		return false, err
	}

	if len(results) == 0 {
		return false, errors.New("unexpected empty results")
	}

	return results[0].(bool), nil
}

func (c *contract) IsOverlayFrozen(ctx context.Context, block uint64) (bool, error) {

	var overlayAddr [32]byte
	copy(overlayAddr[:], c.overlay.Bytes())
	callData, err := c.stakingContractABI.Pack("lastUpdatedBlockNumberOfOverlay", overlayAddr)
	if err != nil {
		return false, err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.stakingContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := c.stakingContractABI.Unpack("lastUpdatedBlockNumberOfOverlay", result)
	if err != nil {
		return false, err
	}

	if len(results) == 0 {
		return false, errors.New("unexpected empty results")
	}

	lastUpdate := abi.ConvertType(results[0], new(big.Int)).(*big.Int)

	return lastUpdate.Uint64() >= block, nil
}
