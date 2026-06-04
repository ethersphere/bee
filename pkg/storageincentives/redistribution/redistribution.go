// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

const (
	loggerName      = "redistributionContract"
	BoostTipPercent = 50

	minEstimatedGasLimit = 250_000

	// redistributionGameTransactionsRetryDelay caps the retry delay for redistribution game txs.
	redistributionGameTransactionsRetryDelay = 35 * time.Second
)

// ClaimOpts configures optional claim behaviour: after OverrideAfterBlock (absolute
// chain block number), if ExpectedReward covers upper-bound claim cost plus
// RoundFees, the max-tx-cost limit is bypassed for that send attempt.
type ClaimOpts struct {
	OverrideAfterBlock uint64
	CurrentBlockFn     func() uint64
	ExpectedReward     *big.Int
	RoundFees          *big.Int
}

type Contract interface {
	ReserveSalt(context.Context) ([]byte, error)
	IsPlaying(context.Context, uint8) (bool, error)
	IsWinner(context.Context) (bool, error)
	Claim(context.Context, ChunkInclusionProofs, *ClaimOpts) (common.Hash, error)
	Commit(context.Context, []byte, uint64) (common.Hash, error)
	Reveal(context.Context, uint8, []byte, []byte) (common.Hash, error)
}

type contract struct {
	overlay                   swarm.Address
	owner                     common.Address
	logger                    log.Logger
	txService                 transaction.Service
	incentivesContractAddress common.Address
	incentivesContractABI     abi.ABI
	gasLimit                  uint64
	retryDelayRewrite         func(time.Duration) time.Duration
}

type Option func(*contract)

// WithRetryDelayRewrite sets a function that rewrites the configured SendWithRetry delay.
func WithRetryDelayRewrite(fn func(time.Duration) time.Duration) Option {
	return func(c *contract) {
		c.retryDelayRewrite = fn
	}
}

func New(
	overlay swarm.Address,
	owner common.Address,
	logger log.Logger,
	txService transaction.Service,
	incentivesContractAddress common.Address,
	incentivesContractABI abi.ABI,
	gasLimit uint64,
	opts ...Option,
) Contract {
	c := &contract{
		overlay:                   overlay,
		owner:                     owner,
		logger:                    logger.WithName(loggerName).Register(),
		txService:                 txService,
		incentivesContractAddress: incentivesContractAddress,
		incentivesContractABI:     incentivesContractABI,
		gasLimit:                  gasLimit,
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.retryDelayRewrite == nil {
		c.retryDelayRewrite = capRetryDelay
	}
	return c
}

// IsPlaying checks if the overlay is participating in the upcoming round.
func (c *contract) IsPlaying(ctx context.Context, depth uint8) (bool, error) {
	callData, err := c.incentivesContractABI.Pack("isParticipatingInUpcomingRound", c.owner, depth)
	if err != nil {
		return false, err
	}

	result, err := c.callTx(ctx, callData)
	if err != nil {
		return false, fmt.Errorf("IsPlaying: owner %v depth %d: %w", c.owner, depth, err)
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

// Claim sends a transaction to blockchain if a win is claimed. When opts is
// non-nil and the configured max-tx-price would block the broadcast,
// canOverrideClaim is consulted: if the override block threshold has passed
// and ExpectedReward covers the estimated cost plus previous round fees,
// the price cap is bypassed.
func (c *contract) Claim(ctx context.Context, proofs ChunkInclusionProofs, opts *ClaimOpts) (txHash common.Hash, err error) {
	callData, err := c.incentivesContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
	if err != nil {
		return common.Hash{}, err
	}
	request := &transaction.TxRequest{
		To:                   &c.incentivesContractAddress,
		Data:                 callData,
		GasLimit:             c.gasLimit,
		MinEstimatedGasLimit: minEstimatedGasLimit,
		Value:                big.NewInt(0),
		Description:          "claim win transaction",
	}

	retryOpts := []transaction.RetryOption{
		transaction.WithIgnoreMaxPrice(func(gasFeeCap *big.Int) bool {
			return c.canOverrideClaim(opts, gasFeeCap)
		}),
	}

	txHash, err = c.sendAndWait(ctx, request, retryOpts...)
	if err != nil {
		return txHash, fmt.Errorf("claim: %w", err)
	}
	return txHash, nil
}

// canOverrideClaim decides whether the claim transaction should bypass the
// max-tx-price cap. gasFeeCap is the actual max fee per gas (wei) that the
// retry loop wants to use — it is provided by suggestGasFeeForTier
// so there is no redundant estimation.
func (c *contract) canOverrideClaim(opts *ClaimOpts, gasFeeCap *big.Int) bool {
	if opts == nil || opts.OverrideAfterBlock == 0 || opts.CurrentBlockFn == nil || opts.RoundFees == nil {
		return false
	}

	if opts.CurrentBlockFn() < opts.OverrideAfterBlock {
		return false
	}

	if opts.ExpectedReward == nil || opts.ExpectedReward.Sign() <= 0 {
		return false
	}

	gasUnits := c.gasLimit
	if gasUnits <= 0 {
		gasUnits = minEstimatedGasLimit
	}

	txCost := new(big.Int).Mul(gasFeeCap, big.NewInt(int64(gasUnits)))
	totalSpent := new(big.Int).Add(txCost, opts.RoundFees)
	if opts.ExpectedReward.Cmp(totalSpent) < 0 {
		c.logger.Info("claim override: reward does not cover cost",
			"tx_cost", txCost,
			"round_fees", opts.RoundFees,
			"total_spent", totalSpent,
			"expected_reward", opts.ExpectedReward,
		)
		return false
	}
	return true
}

// Commit submits the obfusHash hash by sending a transaction to the blockchain.
func (c *contract) Commit(ctx context.Context, obfusHash []byte, round uint64) (common.Hash, error) {
	callData, err := c.incentivesContractABI.Pack("commit", common.BytesToHash(obfusHash), round)
	if err != nil {
		return common.Hash{}, err
	}
	request := &transaction.TxRequest{
		To:                   &c.incentivesContractAddress,
		Data:                 callData,
		GasLimit:             c.gasLimit,
		MinEstimatedGasLimit: minEstimatedGasLimit,
		Value:                big.NewInt(0),
		Description:          "commit transaction",
	}
	txHash, err := c.sendAndWait(ctx, request)
	if err != nil {
		return txHash, fmt.Errorf("commit: obfusHash %v: %w", common.BytesToHash(obfusHash), err)
	}

	return txHash, nil
}

// Reveal submits the storageDepth, reserveCommitmentHash and RandomNonce in a transaction to blockchain.
func (c *contract) Reveal(ctx context.Context, storageDepth uint8, reserveCommitmentHash []byte, RandomNonce []byte) (common.Hash, error) {
	callData, err := c.incentivesContractABI.Pack("reveal", storageDepth, common.BytesToHash(reserveCommitmentHash), common.BytesToHash(RandomNonce))
	if err != nil {
		return common.Hash{}, err
	}
	request := &transaction.TxRequest{
		To:                   &c.incentivesContractAddress,
		Data:                 callData,
		GasLimit:             c.gasLimit,
		MinEstimatedGasLimit: minEstimatedGasLimit,
		Value:                big.NewInt(0),
		Description:          "reveal transaction",
	}
	txHash, err := c.sendAndWait(ctx, request)
	if err != nil {
		return txHash, fmt.Errorf("reveal: storageDepth %d reserveCommitmentHash %v RandomNonce %v: %w", storageDepth, common.BytesToHash(reserveCommitmentHash), common.BytesToHash(RandomNonce), err)
	}

	return txHash, nil
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

func (c *contract) sendAndWait(ctx context.Context, request *transaction.TxRequest, opts ...transaction.RetryOption) (txHash common.Hash, err error) {
	if c.retryDelayRewrite != nil {
		opts = append(opts, transaction.WithRetryDelay(c.retryDelayRewrite))
	}

	defer func() {
		err = c.txService.UnwrapABIError(
			ctx,
			request,
			err,
			c.incentivesContractABI.Errors,
		)
	}()

	txHash, _, err = c.txService.SendWithRetry(ctx, request, opts...)
	return txHash, err
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

// capRetryDelay limits the retry delay for redistribution game transactions
// to redistributionGameTransactionsRetryDelay.
func capRetryDelay(d time.Duration) time.Duration {
	if d > redistributionGameTransactionsRetryDelay {
		return redistributionGameTransactionsRetryDelay
	}
	return d
}
