// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
)

const loggerName = "redistributionContract"

type Contract interface {
	ReserveSalt(context.Context) ([]byte, error)
	IsPlaying(context.Context, uint8) (bool, error)
	IsWinner(context.Context) (bool, error)
	Claim(context.Context) (*big.Int, error)
	Commit(context.Context, []byte, *big.Int) (*big.Int, error)
	Reveal(context.Context, uint8, []byte, []byte) (*big.Int, error)
}

type contract struct {
	overlay                   swarm.Address
	logger                    log.Logger
	txService                 transaction.Service
	incentivesContractAddress common.Address
	incentivesContractABI     abi.ABI
}

func New(
	overlay swarm.Address,
	logger log.Logger,
	txService transaction.Service,
	incentivesContractAddress common.Address,
	incentivesContractABI abi.ABI,
) Contract {
	return &contract{
		overlay:                   overlay,
		logger:                    logger.WithName(loggerName).Register(),
		txService:                 txService,
		incentivesContractAddress: incentivesContractAddress,
		incentivesContractABI:     incentivesContractABI,
	}
}

// IsPlaying checks if the overlay is participating in the upcoming round.
func (c *contract) IsPlaying(ctx context.Context, depth uint8) (bool, error) {
	callData, err := c.incentivesContractABI.Pack("isParticipatingInUpcomingRound", common.BytesToHash(c.overlay.Bytes()), depth)
	if err != nil {
		return false, err
	}

	result, err := c.callTx(ctx, callData)
	if err != nil {
		return false, fmt.Errorf("IsPlaying: overlay %v depth %d: %w", common.BytesToHash(c.overlay.Bytes()), depth, err)
	}

	results, err := c.incentivesContractABI.Unpack("isParticipatingInUpcomingRound", result)
	if err != nil {
		return false, fmt.Errorf("IsPlaying: results %v: %w", results, err)
	}

	return results[0].(bool), nil
}

// IsWinner checks if the overlay is winner by sending a transaction to blockchain.
func (c *contract) IsWinner(ctx context.Context) (isWinner bool, err error) {
	callData, err := c.incentivesContractABI.Pack("isWinner", common.BytesToHash(c.overlay.Bytes()))
	if err != nil {
		return false, err
	}

	result, err := c.callTx(ctx, callData)
	if err != nil {
		return false, fmt.Errorf("IsWinner: overlay %v : %w", common.BytesToHash(c.overlay.Bytes()), err)
	}

	results, err := c.incentivesContractABI.Unpack("isWinner", result)
	if err != nil {
		return false, fmt.Errorf("IsWinner: results %v : %w", results, err)
	}
	return results[0].(bool), nil
}

// Claim sends a transaction to blockchain if a win is claimed.
func (c *contract) Claim(ctx context.Context) (*big.Int, error) {
	callData, err := c.incentivesContractABI.Pack("claim")
	if err != nil {
		return nil, err
	}
	request := &transaction.TxRequest{
		To:                   &c.incentivesContractAddress,
		Data:                 callData,
		GasPrice:             sctx.GetGasPrice(ctx),
		GasLimit:             sctx.GetGasLimit(ctx),
		MinEstimatedGasLimit: 500_000,
		Value:                big.NewInt(0),
		Description:          "claim win transaction",
	}
	fee, err := c.sendAndWait(ctx, request, 50)
	if err != nil {
		return fee, fmt.Errorf("claim: %w", err)
	}

	return fee, nil
}

// Commit submits the obfusHash hash by sending a transaction to the blockchain.
func (c *contract) Commit(ctx context.Context, obfusHash []byte, round *big.Int) (*big.Int, error) {
	callData, err := c.incentivesContractABI.Pack("commit", common.BytesToHash(obfusHash), common.BytesToHash(c.overlay.Bytes()), round)
	if err != nil {
		return nil, err
	}
	request := &transaction.TxRequest{
		To:                   &c.incentivesContractAddress,
		Data:                 callData,
		GasPrice:             sctx.GetGasPrice(ctx),
		GasLimit:             sctx.GetGasLimit(ctx),
		MinEstimatedGasLimit: 500_000,
		Value:                big.NewInt(0),
		Description:          "commit transaction",
	}
	fee, err := c.sendAndWait(ctx, request, 50)
	if err != nil {
		return fee, fmt.Errorf("commit: obfusHash %v overlay %v: %w", common.BytesToHash(obfusHash), common.BytesToHash(c.overlay.Bytes()), err)
	}

	return fee, nil
}

// Reveal submits the storageDepth, reserveCommitmentHash and RandomNonce in a transaction to blockchain.
func (c *contract) Reveal(ctx context.Context, storageDepth uint8, reserveCommitmentHash, RandomNonce []byte) (*big.Int, error) {
	callData, err := c.incentivesContractABI.Pack("reveal", common.BytesToHash(c.overlay.Bytes()), storageDepth, common.BytesToHash(reserveCommitmentHash), common.BytesToHash(RandomNonce))
	if err != nil {
		return nil, err
	}
	request := &transaction.TxRequest{
		To:                   &c.incentivesContractAddress,
		Data:                 callData,
		GasPrice:             sctx.GetGasPrice(ctx),
		GasLimit:             sctx.GetGasLimit(ctx),
		MinEstimatedGasLimit: 500_000,
		Value:                big.NewInt(0),
		Description:          "reveal transaction",
	}
	fee, err := c.sendAndWait(ctx, request, 50)
	if err != nil {
		return fee, fmt.Errorf("reveal: storageDepth %d reserveCommitmentHash %v RandomNonce %v: %w", storageDepth, common.BytesToHash(reserveCommitmentHash), common.BytesToHash(RandomNonce), err)
	}

	return fee, nil
}

// ReserveSalt provides the current round anchor by transacting on the blockchain.
func (c *contract) ReserveSalt(ctx context.Context) ([]byte, error) {
	callData, err := c.incentivesContractABI.Pack("currentRoundAnchor")
	if err != nil {
		return nil, err
	}

	result, err := c.callTx(ctx, callData)
	if err != nil {
		return nil, err
	}

	results, err := c.incentivesContractABI.Unpack("currentRoundAnchor", result)
	if err != nil {
		return nil, err
	}
	salt := results[0].([32]byte)
	return salt[:], nil
}

func (c *contract) sendAndWait(ctx context.Context, request *transaction.TxRequest, boostPercent int) (*big.Int, error) {
	txHash, err := c.txService.Send(ctx, request, boostPercent)
	if err != nil {
		return nil, err
	}
	receipt, err := c.txService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return c.fee(ctx, txHash), err
	}

	if receipt.Status == 0 {
		return c.fee(ctx, txHash), transaction.ErrTransactionReverted
	}
	return c.fee(ctx, txHash), nil
}

// callTx simulates a transaction based on tx request.
func (c *contract) callTx(ctx context.Context, callData []byte) ([]byte, error) {
	result, err := c.txService.Call(ctx, &transaction.TxRequest{
		To:   &c.incentivesContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *contract) fee(ctx context.Context, txHash common.Hash) *big.Int {
	fee, err := c.txService.TransactionFee(ctx, txHash)
	if err != nil {
		c.logger.Info("transaction fee error:", err)
		return big.NewInt(0)
	}
	return fee
}
