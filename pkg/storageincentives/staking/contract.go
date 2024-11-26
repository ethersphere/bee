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
	ErrUnexpectedLength        = errors.New("unexpected results length")

	approveDescription       = "Approve tokens for stake deposit operations"
	depositStakeDescription  = "Deposit Stake"
	withdrawStakeDescription = "Withdraw stake"
	migrateStakeDescription  = "Migrate stake"
)

type Contract interface {
	DepositStake(ctx context.Context, stakedAmount *big.Int) (common.Hash, error)
	ChangeStakeOverlay(ctx context.Context, nonce common.Hash) (common.Hash, error)
	GetPotentialStake(ctx context.Context) (*big.Int, error)
	GetWithdrawableStake(ctx context.Context) (*big.Int, error)
	WithdrawStake(ctx context.Context) (common.Hash, error)
	MigrateStake(ctx context.Context) (common.Hash, error)
	UpdateHeight(ctx context.Context) (common.Hash, bool, error)
	RedistributionStatuser
}

type RedistributionStatuser interface {
	IsOverlayFrozen(ctx context.Context, block uint64) (bool, error)
}

type contract struct {
	owner                  common.Address
	stakingContractAddress common.Address
	stakingContractABI     abi.ABI
	bzzTokenAddress        common.Address
	transactionService     transaction.Service
	overlayNonce           common.Hash
	gasLimit               uint64
	height                 uint8
}

func New(
	owner common.Address,
	stakingContractAddress common.Address,
	stakingContractABI abi.ABI,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
	nonce common.Hash,
	setGasLimit bool,
	height uint8,
) Contract {

	var gasLimit uint64
	if setGasLimit {
		gasLimit = transaction.DefaultGasLimit
	}

	return &contract{
		owner:                  owner,
		stakingContractAddress: stakingContractAddress,
		stakingContractABI:     stakingContractABI,
		bzzTokenAddress:        bzzTokenAddress,
		transactionService:     transactionService,
		overlayNonce:           nonce,
		gasLimit:               gasLimit,
		height:                 height,
	}
}

func (c *contract) DepositStake(ctx context.Context, stakedAmount *big.Int) (common.Hash, error) {
	prevStakedAmount, err := c.GetPotentialStake(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	if len(prevStakedAmount.Bits()) == 0 {
		if stakedAmount.Cmp(MinimumStakeAmount) == -1 {
			return common.Hash{}, ErrInsufficientStakeAmount
		}
	}

	if big.NewInt(0).Add(prevStakedAmount, stakedAmount).Cmp(big.NewInt(0).Mul(big.NewInt(1<<c.height), MinimumStakeAmount)) < 0 {
		return common.Hash{}, fmt.Errorf("stake amount does not sufficiently cover the additional reserve capacity: %w", ErrInsufficientStakeAmount)
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

	receipt, err := c.sendManageStakeTransaction(ctx, stakedAmount)
	if err != nil {
		return common.Hash{}, err
	}

	return receipt.TxHash, nil
}

// ChangeStakeOverlay only changes the overlay address used in the redistribution game.
func (c *contract) ChangeStakeOverlay(ctx context.Context, nonce common.Hash) (common.Hash, error) {
	c.overlayNonce = nonce
	receipt, err := c.sendManageStakeTransaction(ctx, new(big.Int))
	if err != nil {
		return common.Hash{}, err
	}

	return receipt.TxHash, nil
}

// UpdateHeight submits the reserve doubling height to the contract only if the height is a new value.
func (c *contract) UpdateHeight(ctx context.Context) (common.Hash, bool, error) {

	h, err := c.getHeight(ctx)
	if err != nil {
		return common.Hash{}, false, fmt.Errorf("staking contract: failed to read previous height: %w", err)
	}

	if h == c.height {
		return common.Hash{}, false, nil
	}

	receipt, err := c.sendManageStakeTransaction(ctx, new(big.Int))
	if err != nil {
		return common.Hash{}, false, fmt.Errorf("staking contract: failed to write new height: %w", err)
	}

	return receipt.TxHash, true, nil
}

func (c *contract) GetPotentialStake(ctx context.Context) (*big.Int, error) {
	stakedAmount, err := c.getPotentialStake(ctx)
	if err != nil {
		return nil, fmt.Errorf("staking contract: failed to get stake: %w", err)
	}
	return stakedAmount, nil
}

func (c *contract) GetWithdrawableStake(ctx context.Context) (*big.Int, error) {
	withdrawableStake, err := c.getWithdrawableStake(ctx)
	if err != nil {
		return nil, fmt.Errorf("staking contract: failed to get stake: %w", err)
	}
	return withdrawableStake, nil
}

func (c *contract) WithdrawStake(ctx context.Context) (txHash common.Hash, err error) {
	withdrawableStake, err := c.getWithdrawableStake(ctx)
	if err != nil {
		return
	}

	if withdrawableStake.Cmp(big.NewInt(0)) <= 0 {
		return common.Hash{}, ErrInsufficientStake
	}

	receipt, err := c.withdrawFromStake(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	if receipt != nil {
		txHash = receipt.TxHash
	}
	return txHash, nil
}

func (c *contract) MigrateStake(ctx context.Context) (txHash common.Hash, err error) {
	isPaused, err := c.paused(ctx)
	if err != nil {
		return
	}
	if !isPaused {
		return common.Hash{}, ErrNotPaused
	}

	receipt, err := c.migrateStake(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	if receipt != nil {
		txHash = receipt.TxHash
	}
	return txHash, nil
}

func (c *contract) IsOverlayFrozen(ctx context.Context, block uint64) (bool, error) {
	callData, err := c.stakingContractABI.Pack("lastUpdatedBlockNumberOfAddress", c.owner)
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

	results, err := c.stakingContractABI.Unpack("lastUpdatedBlockNumberOfAddress", result)
	if err != nil {
		return false, err
	}

	if len(results) == 0 {
		return false, errors.New("unexpected empty results")
	}

	lastUpdate := abi.ConvertType(results[0], new(big.Int)).(*big.Int)

	return lastUpdate.Uint64() >= block, nil
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
		GasLimit:    max(sctx.GetGasLimit(ctx), c.gasLimit),
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
		GasLimit:    max(sctx.GetGasLimit(ctx), c.gasLimit),
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

func (c *contract) sendManageStakeTransaction(ctx context.Context, stakedAmount *big.Int) (*types.Receipt, error) {
	callData, err := c.stakingContractABI.Pack("manageStake", c.overlayNonce, stakedAmount, c.height)
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, depositStakeDescription)
	if err != nil {
		return nil, fmt.Errorf("deposit stake: stakedAmount %d: %w", stakedAmount, err)
	}

	return receipt, nil
}

func (c *contract) getPotentialStake(ctx context.Context) (*big.Int, error) {
	callData, err := c.stakingContractABI.Pack("stakes", c.owner)
	if err != nil {
		return nil, err
	}
	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.stakingContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, fmt.Errorf("get potential stake: %w", err)
	}

	// overlay bytes32,
	// committedStake uint256,
	// potentialStake uint256,
	// lastUpdatedBlockNumber uint256,
	results, err := c.stakingContractABI.Unpack("stakes", result)
	if err != nil {
		return nil, err
	}

	if len(results) < 4 {
		return nil, ErrUnexpectedLength
	}

	return abi.ConvertType(results[2], new(big.Int)).(*big.Int), nil
}

func (c *contract) getWithdrawableStake(ctx context.Context) (*big.Int, error) {
	callData, err := c.stakingContractABI.Pack("withdrawableStake")
	if err != nil {
		return nil, err
	}
	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.stakingContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, fmt.Errorf("get withdrawable stake: %w", err)
	}

	results, err := c.stakingContractABI.Unpack("withdrawableStake", result)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, errors.New("unexpected empty results")
	}

	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
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

func (c *contract) migrateStake(ctx context.Context) (*types.Receipt, error) {
	callData, err := c.stakingContractABI.Pack("migrateStake")
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, migrateStakeDescription)
	if err != nil {
		return nil, fmt.Errorf("migrate stake: %w", err)
	}

	return receipt, nil
}

func (c *contract) withdrawFromStake(ctx context.Context) (*types.Receipt, error) {
	callData, err := c.stakingContractABI.Pack("withdrawFromStake")
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, withdrawStakeDescription)
	if err != nil {
		return nil, fmt.Errorf("withdraw stake: %w", err)
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

func (c *contract) getHeight(ctx context.Context) (uint8, error) {
	callData, err := c.stakingContractABI.Pack("heightOfAddress", c.owner)
	if err != nil {
		return 0, err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.stakingContractAddress,
		Data: callData,
	})
	if err != nil {
		return 0, err
	}
	results, err := c.stakingContractABI.Unpack("heightOfAddress", result)
	if err != nil {
		return 0, err
	}

	if len(results) == 0 {
		return 0, errors.New("unexpected empty results")
	}

	return results[0].(uint8), nil
}
