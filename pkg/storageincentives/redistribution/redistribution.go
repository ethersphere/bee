// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

const (
	loggerName      = "redistributionContract"
	BoostTipPercent = 50

	retryBaseDelay     = 5 * time.Second
	claimRetryInterval = 15 * time.Second
)

// ErrMaxTxCostExceeded is returned when the upper-bound cost of a redistribution
// transaction (gas limit × max fee per gas) would exceed the configured limit.
var ErrMaxTxCostExceeded = errors.New("redistribution tx cost exceeds max tx cost limit")

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
	ExpectedReward(ctx context.Context) (*big.Int, error)
}

type contract struct {
	overlay                   swarm.Address
	owner                     common.Address
	logger                    log.Logger
	txService                 transaction.Service
	incentivesContractAddress common.Address
	incentivesContractABI     abi.ABI
	postageContractAddress    common.Address
	postageContractABI        abi.ABI
	maxTxCost                 *big.Int
	maxTxCostTolerancePercent uint64
	gasLimit                  uint64
}

// Option configures the redistribution contract wrapper.
type Option func(*contract)

// WithMaxTxCost sets the maximum total wei the node is willing to spend on a
// single redistribution transaction (gas limit × max fee per gas). When wei is
// zero, no limit is enforced. tolerancePercent expands the limit (e.g. 5 means
// allow up to 105% of wei); use 0 for a strict cap.
func WithMaxTxCost(wei uint64, tolerancePercent uint64) Option {
	return func(c *contract) {
		if wei > 0 {
			c.maxTxCost = new(big.Int).SetUint64(wei)
			c.maxTxCostTolerancePercent = tolerancePercent
		}
	}
}

func New(
	overlay swarm.Address,
	owner common.Address,
	logger log.Logger,
	txService transaction.Service,
	incentivesContractAddress common.Address,
	incentivesContractABI abi.ABI,
	postageContractAddress common.Address,
	postageContractABI abi.ABI,
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
		postageContractAddress:    postageContractAddress,
		postageContractABI:        postageContractABI,
		gasLimit:                  gasLimit,
	}
	for _, o := range opts {
		o(c)
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

// Claim sends a transaction to blockchain if a win is claimed. When the
// configured max-tx-cost is exceeded, Claim waits claimRetryInterval and
// retries instead of returning ErrMaxTxCostExceeded. After OverrideAfterBlock
// (see ClaimOpts), if economics justify it, the limit is bypassed for one send.
func (c *contract) Claim(ctx context.Context, proofs ChunkInclusionProofs, opts *ClaimOpts) (common.Hash, error) {
	callData, err := c.incentivesContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
	if err != nil {
		return common.Hash{}, err
	}

	for {
		sendCtx, prepErr := c.prepareSendCtx(ctx, BoostTipPercent)
		if prepErr == nil {
			request := c.newTxRequest(sendCtx, callData, "claim win transaction")
			txHash, sendErr := c.sendAndWaitWithRetry(sendCtx, request, BoostTipPercent)
			if sendErr != nil {
				return txHash, fmt.Errorf("claim: %w", sendErr)
			}
			return txHash, nil
		}

		if !errors.Is(prepErr, ErrMaxTxCostExceeded) {
			return common.Hash{}, fmt.Errorf("claim: %w", prepErr)
		}

		if c.tryClaimOverride(opts) {
			gasFeeCap, ok := c.canOverrideClaim(ctx, opts)
			if ok {
				c.logger.Warning("claim: max-tx-cost overridden",
					"expected_reward", opts.ExpectedReward,
					"round_fees", opts.RoundFees,
					"override_after_block", opts.OverrideAfterBlock,
				)
				overrideCtx := sctx.SetGasPrice(ctx, gasFeeCap)
				request := c.newTxRequest(overrideCtx, callData, "claim win transaction (override)")
				txHash, sendErr := c.sendAndWaitWithRetry(overrideCtx, request, BoostTipPercent)
				if sendErr != nil {
					return txHash, fmt.Errorf("claim: %w", sendErr)
				}
				return txHash, nil
			}
		}

		c.logger.Info("claim: tx cost exceeds limit, waiting", "retry_in", claimRetryInterval)
		select {
		case <-ctx.Done():
			return common.Hash{}, ctx.Err()
		case <-time.After(claimRetryInterval):
		}
	}
}

func (c *contract) tryClaimOverride(opts *ClaimOpts) bool {
	if opts == nil || opts.OverrideAfterBlock == 0 || opts.CurrentBlockFn == nil {
		return false
	}
	return opts.CurrentBlockFn() >= opts.OverrideAfterBlock
}

func (c *contract) canOverrideClaim(ctx context.Context, opts *ClaimOpts) (*big.Int, bool) {
	if opts.ExpectedReward == nil {
		reward, err := c.ExpectedReward(ctx)
		if err != nil {
			c.logger.Warning("error getting expected claim reward", "error", err)
		}
		opts.ExpectedReward = reward
	}

	if opts.ExpectedReward == nil || opts.RoundFees == nil {
		return nil, false
	}
	if opts.ExpectedReward.Sign() <= 0 {
		return nil, false
	}

	gasUnits := int64(max(sctx.GetGasLimit(ctx), c.gasLimit))
	if gasUnits <= 0 {
		gasUnits = 500_000
	}

	estimated, gasFeeCap, err := c.txService.EstimateTxCost(ctx, gasUnits, BoostTipPercent)
	if err != nil {
		c.logger.Warning("claim override: estimate failed", "error", err)
		return nil, false
	}

	totalSpent := new(big.Int).Add(estimated, opts.RoundFees)
	if opts.ExpectedReward.Cmp(totalSpent) < 0 {
		c.logger.Info("claim override: reward does not cover upper-bound cost",
			"estimated", estimated,
			"round_fees", opts.RoundFees,
			"expected_reward", opts.ExpectedReward,
		)
		return nil, false
	}
	return gasFeeCap, true
}

// Commit submits the obfusHash hash by sending a transaction to the blockchain.
func (c *contract) Commit(ctx context.Context, obfusHash []byte, round uint64) (common.Hash, error) {
	callData, err := c.incentivesContractABI.Pack("commit", common.BytesToHash(obfusHash), round)
	if err != nil {
		return common.Hash{}, err
	}
	for {
		ctx, err = c.prepareSendCtx(ctx, BoostTipPercent)
		if err == nil {
			break
		}

		select {
		case <-ctx.Done():
			return common.Hash{}, err
		case <-time.After(retryBaseDelay):
			continue
		}
	}

	request := c.newTxRequest(ctx, callData, "commit transaction")

	txHash, err := c.sendAndWaitWithRetry(ctx, request, BoostTipPercent)
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
	for {
		ctx, err = c.prepareSendCtx(ctx, BoostTipPercent)
		if err == nil {
			break
		}

		select {
		case <-ctx.Done():
			return common.Hash{}, err
		case <-time.After(retryBaseDelay):
			continue
		}
	}
	request := c.newTxRequest(ctx, callData, "reveal transaction")
	txHash, err := c.sendAndWaitWithRetry(ctx, request, BoostTipPercent)
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

func (c *contract) sendAndWaitWithRetry(ctx context.Context, request *transaction.TxRequest, boostPercent int) (txHash common.Hash, err error) {
	defer func() {
		err = c.txService.UnwrapABIError(
			ctx,
			request,
			err,
			c.incentivesContractABI.Errors,
		)
	}()

	for {
		txHash, err = c.txService.Send(ctx, request, boostPercent)
		if err == nil {
			break
		}

		if isCritical(err) {
			return txHash, err
		}

		c.logger.Warning("send failed, will retry", "error", err, "description", request.Description)

		if len(txHash.Bytes()) == 0 {
			select {
			case <-ctx.Done():
				return txHash, ctx.Err()
			case <-time.After(retryBaseDelay):
				continue
			}
		}

		txHash, err = c.resendWithRetry(ctx, txHash)
		if err != nil {
			return common.Hash{}, err
		}
		break
	}

	receipt, err := c.txService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return common.Hash{}, err
	}

	if receipt.Status == 0 {
		return txHash, transaction.ErrTransactionReverted
	}

	return txHash, nil
}

func (c *contract) resendWithRetry(ctx context.Context, txHash common.Hash) (common.Hash, error) {
	for {
		err := c.txService.ResendTransaction(ctx, txHash)
		if err != nil {
			if isCritical(err) {
				return txHash, err
			}
			select {
			case <-ctx.Done():
				return txHash, ctx.Err()
			case <-time.After(retryBaseDelay):
				continue
			}
		}
		return txHash, nil
	}
}

func isCritical(err error) bool {
	if errors.Is(err, transaction.ErrTransactionReverted) {
		return true
	}
	if errors.Is(err, transaction.ErrTransactionCancelled) {
		return true
	}
	if errors.Is(err, transaction.ErrFeeCapExceeded) {
		return true
	}

	s := err.Error()
	nonRetryable := []string{
		"specified gas price",
		"below current base fee",
		"AlreadyCommitted",
		"AlreadyRevealed",
		"AlreadyClaimed",
		"NotCommitPhase",
		"NotRevealPhase",
		"NotClaimPhase",
		"CommitRoundOver",
		"CommitRoundNotStarted",
		"PhaseLastBlock",
		"OutOfDepth",
		"OutOfDepthReveal",
		"OutOfDepthClaim",
		"NotStaked",
		"MustStake2Rounds",
		"NoReveals",
		"NoCommitsReceived",
		"execution reverted",
		"insufficient funds",
	}
	for _, sub := range nonRetryable {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ExpectedReward returns the current pot value from the PostageStamp contract via eth_call.
func (c *contract) ExpectedReward(ctx context.Context) (*big.Int, error) {
	callData, err := c.postageContractABI.Pack("totalPot")
	if err != nil {
		return nil, fmt.Errorf("pack totalPot: %w", err)
	}

	result, err := c.txService.Call(ctx, &transaction.TxRequest{
		To:   &c.postageContractAddress,
		Data: callData,
	})
	if err != nil {
		return nil, fmt.Errorf("call totalPot: %w", err)
	}

	results, err := c.postageContractABI.Unpack("totalPot", result)
	if err != nil {
		return nil, fmt.Errorf("unpack totalPot: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("totalPot returned no results")
	}

	pot, ok := results[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("totalPot unexpected type %T", results[0])
	}

	return pot, nil
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

// prepareSendCtx enforces max tx cost (when configured) and pins maxFeePerGas
// into ctx so the subsequent Send cannot use a higher fee than checked.
func (c *contract) prepareSendCtx(ctx context.Context, boostPercent int) (context.Context, error) {
	if c.maxTxCost == nil {
		return ctx, nil
	}
	ok, _, gasFeeCap, err := c.isTxCostAcceptable(ctx, boostPercent)
	if err != nil {
		return ctx, err
	}
	if !ok {
		return ctx, ErrMaxTxCostExceeded
	}
	if gasFeeCap != nil {
		ctx = sctx.SetGasPrice(ctx, gasFeeCap)
	}
	return ctx, nil
}

func (c *contract) newTxRequest(ctx context.Context, callData []byte, description string) *transaction.TxRequest {
	pinnedGasFeeCap := sctx.GetGasPrice(ctx)
	return &transaction.TxRequest{
		To:                   &c.incentivesContractAddress,
		Data:                 callData,
		GasPrice:             pinnedGasFeeCap,
		GasFeeCap:            pinnedGasFeeCap,
		GasLimit:             max(sctx.GetGasLimit(ctx), c.gasLimit),
		MinEstimatedGasLimit: 500_000,
		Value:                big.NewInt(0),
		Description:          description,
	}
}

func (c *contract) isTxCostAcceptable(ctx context.Context, tip int) (ok bool, estimated *big.Int, gasFeeCap *big.Int, err error) {
	gasUnits := int64(max(sctx.GetGasLimit(ctx), c.gasLimit))
	if gasUnits <= 0 {
		gasUnits = 500_000
	}

	// If the caller pinned maxFeePerGas in context, use it for total-cost estimate
	// since that is the actual cap used when creating the transaction request.
	if pinnedGasFeeCap := sctx.GetGasPrice(ctx); pinnedGasFeeCap != nil {
		if pinnedGasFeeCap.Sign() <= 0 {
			return false, nil, nil, errors.New("gas price must be greater than zero")
		}
		gasFeeCap = new(big.Int).Set(pinnedGasFeeCap)
		estimated = new(big.Int).Mul(big.NewInt(gasUnits), gasFeeCap)
	} else {
		estimated, gasFeeCap, err = c.txService.EstimateTxCost(ctx, gasUnits, tip)
		if err != nil {
			return false, nil, nil, err
		}
	}

	if c.maxTxCost == nil {
		return true, estimated, gasFeeCap, nil
	}

	tol := c.maxTxCostTolerancePercent
	threshold := new(big.Int).Mul(c.maxTxCost, big.NewInt(int64(100+tol)))
	threshold.Div(threshold, big.NewInt(100))
	return estimated.Cmp(threshold) <= 0, estimated, gasFeeCap, nil
}
