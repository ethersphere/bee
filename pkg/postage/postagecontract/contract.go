// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postagecontract

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
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	BucketDepth = uint8(16)

	postageStampABI   = parseABI(PostageABI)
	erc20ABI          = parseABI(sw3abi.ERC20ABIv0_3_1)
	batchCreatedTopic = postageStampABI.Events["BatchCreated"].ID
	batchTopUpTopic   = postageStampABI.Events["BatchTopUp"].ID
	batchDiluteTopic  = postageStampABI.Events["BatchDepthIncrease"].ID

	ErrBatchCreate       = errors.New("batch creation failed")
	ErrInsufficientFunds = errors.New("insufficient token balance")
	ErrInvalidDepth      = errors.New("invalid depth")
	ErrBatchTopUp        = errors.New("batch topUp failed")
	ErrBatchDilute       = errors.New("batch dilute failed")
	ErrChainDisabled     = errors.New("chain disabled")
	ErrNotImplemented    = errors.New("not implemented")

	approveDescription     = "Approve tokens for postage operations"
	createBatchDescription = "Postage batch creation"
	topUpBatchDescription  = "Postage batch top up"
	diluteBatchDescription = "Postage batch dilute"
)

type Interface interface {
	CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (common.Hash, []byte, error)
	TopUpBatch(ctx context.Context, batchID []byte, topupBalance *big.Int) (common.Hash, error)
	DiluteBatch(ctx context.Context, batchID []byte, newDepth uint8) (common.Hash, error)
	PostageBatchExpirer
}

type PostageBatchExpirer interface {
	ExpireBatches(ctx context.Context) error
}

type postageContract struct {
	owner                  common.Address
	postageContractAddress common.Address
	bzzTokenAddress        common.Address
	transactionService     transaction.Service
	postageService         postage.Service
	postageStorer          postage.Storer
}

func New(
	owner,
	postageContractAddress,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
	postageService postage.Service,
	postageStorer postage.Storer,
	chainEnabled bool,
) Interface {
	if !chainEnabled {
		return new(noOpPostageContract)
	}

	return &postageContract{
		owner:                  owner,
		postageContractAddress: postageContractAddress,
		bzzTokenAddress:        bzzTokenAddress,
		transactionService:     transactionService,
		postageService:         postageService,
		postageStorer:          postageStorer,
	}
}

func (c *postageContract) ExpireBatches(ctx context.Context) error {
	for {
		exists, err := c.expiredBatchesExists(ctx)
		if err != nil {
			return fmt.Errorf("expired batches exist: %w", err)
		}
		if !exists {
			break
		}

		err = c.expireLimitedBatches(ctx, big.NewInt(50))
		if err != nil {
			return fmt.Errorf("expire limited batches: %w", err)
		}
	}
	return nil
}

func (c *postageContract) expiredBatchesExists(ctx context.Context) (bool, error) {
	callData, err := postageStampABI.Pack("expiredBatchesExist")
	if err != nil {
		return false, err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.postageContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := postageStampABI.Unpack("expiredBatchesExist", result)
	if err != nil {
		return false, err
	}
	return results[0].(bool), nil
}

func (c *postageContract) expireLimitedBatches(ctx context.Context, count *big.Int) error {
	callData, err := postageStampABI.Pack("expireLimited", count)
	if err != nil {
		return err
	}

	_, err = c.sendTransaction(sctx.SetGasLimit(ctx, 1_100_000), callData, "expire limited batches")
	if err != nil {
		return err
	}

	return nil
}

func (c *postageContract) sendApproveTransaction(ctx context.Context, amount *big.Int) (*types.Receipt, error) {
	callData, err := erc20ABI.Pack("approve", c.postageContractAddress, amount)
	if err != nil {
		return nil, err
	}

	txHash, err := c.transactionService.Send(ctx, &transaction.TxRequest{
		To:          &c.bzzTokenAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    65000,
		Value:       big.NewInt(0),
		Description: approveDescription,
	}, transaction.DefaultTipBoostPercent)
	if err != nil {
		return nil, err
	}

	receipt, err := c.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}

	if receipt.Status == 0 {
		return nil, transaction.ErrTransactionReverted
	}

	return receipt, nil
}

func (c *postageContract) sendTransaction(ctx context.Context, callData []byte, desc string) (*types.Receipt, error) {
	request := &transaction.TxRequest{
		To:          &c.postageContractAddress,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    sctx.GetGasLimitWithDefault(ctx, 1_000_000),
		Value:       big.NewInt(0),
		Description: desc,
	}

	txHash, err := c.transactionService.Send(ctx, request, transaction.DefaultTipBoostPercent)
	if err != nil {
		return nil, err
	}

	receipt, err := c.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}

	if receipt.Status == 0 {
		return nil, transaction.ErrTransactionReverted
	}

	return receipt, nil
}

func (c *postageContract) sendCreateBatchTransaction(ctx context.Context, owner common.Address, initialBalance *big.Int, depth uint8, nonce common.Hash, immutable bool) (*types.Receipt, error) {

	callData, err := postageStampABI.Pack("createBatch", owner, initialBalance, depth, BucketDepth, nonce, immutable)
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, createBatchDescription)
	if err != nil {
		return nil, fmt.Errorf("create batch: depth %d bucketDepth %d immutable %t: %w", depth, BucketDepth, immutable, err)
	}

	return receipt, nil
}

func (c *postageContract) sendTopUpBatchTransaction(ctx context.Context, batchID []byte, topUpAmount *big.Int) (*types.Receipt, error) {

	callData, err := postageStampABI.Pack("topUp", common.BytesToHash(batchID), topUpAmount)
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, topUpBatchDescription)
	if err != nil {
		return nil, fmt.Errorf("topup batch: amount %d: %w", topUpAmount.Int64(), err)
	}

	return receipt, nil
}

func (c *postageContract) sendDiluteTransaction(ctx context.Context, batchID []byte, newDepth uint8) (*types.Receipt, error) {

	callData, err := postageStampABI.Pack("increaseDepth", common.BytesToHash(batchID), newDepth)
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendTransaction(ctx, callData, diluteBatchDescription)
	if err != nil {
		return nil, fmt.Errorf("dilute batch: new depth %d: %w", newDepth, err)
	}

	return receipt, nil
}

func (c *postageContract) getBalance(ctx context.Context) (*big.Int, error) {
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
	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func (c *postageContract) CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (txHash common.Hash, batchID []byte, err error) {

	if depth <= BucketDepth {
		err = ErrInvalidDepth
		return
	}

	totalAmount := big.NewInt(0).Mul(initialBalance, big.NewInt(int64(1<<depth)))
	balance, err := c.getBalance(ctx)
	if err != nil {
		return
	}

	if balance.Cmp(totalAmount) < 0 {
		err = ErrInsufficientFunds
		return
	}

	err = c.ExpireBatches(ctx)
	if err != nil {
		return
	}

	_, err = c.sendApproveTransaction(ctx, totalAmount)
	if err != nil {
		return
	}

	nonce := make([]byte, 32)
	_, err = rand.Read(nonce)
	if err != nil {
		return
	}

	receipt, err := c.sendCreateBatchTransaction(ctx, c.owner, initialBalance, depth, common.BytesToHash(nonce), immutable)
	if err != nil {
		return
	}
	txHash = receipt.TxHash
	for _, ev := range receipt.Logs {
		if ev.Address == c.postageContractAddress && len(ev.Topics) > 0 && ev.Topics[0] == batchCreatedTopic {
			var createdEvent batchCreatedEvent
			err = transaction.ParseEvent(&postageStampABI, "BatchCreated", &createdEvent, *ev)

			if err != nil {
				return
			}

			batchID = createdEvent.BatchId[:]
			err = c.postageService.Add(postage.NewStampIssuer(
				label,
				c.owner.Hex(),
				batchID,
				initialBalance,
				createdEvent.Depth,
				createdEvent.BucketDepth,
				ev.BlockNumber,
				createdEvent.ImmutableFlag,
			))

			if err != nil {
				return
			}
			return
		}
	}
	err = ErrBatchCreate
	return
}

func (c *postageContract) TopUpBatch(ctx context.Context, batchID []byte, topupBalance *big.Int) (txHash common.Hash, err error) {

	batch, err := c.postageStorer.Get(batchID)
	if err != nil {
		return
	}

	totalAmount := big.NewInt(0).Mul(topupBalance, big.NewInt(int64(1<<batch.Depth)))
	balance, err := c.getBalance(ctx)
	if err != nil {
		return
	}

	if balance.Cmp(totalAmount) < 0 {
		err = ErrInsufficientFunds
		return
	}

	_, err = c.sendApproveTransaction(ctx, totalAmount)
	if err != nil {
		return
	}

	receipt, err := c.sendTopUpBatchTransaction(ctx, batch.ID, topupBalance)
	if err != nil {
		txHash = receipt.TxHash
		return
	}

	for _, ev := range receipt.Logs {
		if ev.Address == c.postageContractAddress && len(ev.Topics) > 0 && ev.Topics[0] == batchTopUpTopic {
			txHash = receipt.TxHash
			return
		}
	}

	err = ErrBatchTopUp
	return
}

func (c *postageContract) DiluteBatch(ctx context.Context, batchID []byte, newDepth uint8) (txHash common.Hash, err error) {

	batch, err := c.postageStorer.Get(batchID)
	if err != nil {
		return
	}

	if batch.Depth > newDepth {
		err = fmt.Errorf("new depth should be greater: %w", ErrInvalidDepth)
		return
	}

	err = c.ExpireBatches(ctx)
	if err != nil {
		return
	}

	receipt, err := c.sendDiluteTransaction(ctx, batch.ID, newDepth)
	if err != nil {
		return
	}
	txHash = receipt.TxHash
	for _, ev := range receipt.Logs {
		if ev.Address == c.postageContractAddress && len(ev.Topics) > 0 && ev.Topics[0] == batchDiluteTopic {
			return
		}
	}
	err = ErrBatchDilute
	return
}

type batchCreatedEvent struct {
	BatchId           [32]byte
	TotalAmount       *big.Int
	NormalisedBalance *big.Int
	Owner             common.Address
	Depth             uint8
	BucketDepth       uint8
	ImmutableFlag     bool
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for postage contract: %v", err))
	}
	return cabi
}

func LookupERC20Address(ctx context.Context, transactionService transaction.Service, postageContractAddress common.Address, chainEnabled bool) (common.Address, error) {
	if !chainEnabled {
		return common.Address{}, nil
	}

	callData, err := postageStampABI.Pack("bzzToken")
	if err != nil {
		return common.Address{}, err
	}

	request := &transaction.TxRequest{
		To:       &postageContractAddress,
		Data:     callData,
		GasPrice: nil,
		GasLimit: 0,
		Value:    big.NewInt(0),
	}

	data, err := transactionService.Call(ctx, request)
	if err != nil {
		return common.Address{}, err
	}

	return common.BytesToAddress(data), nil
}

type noOpPostageContract struct{}

func (m *noOpPostageContract) CreateBatch(context.Context, *big.Int, uint8, bool, string) (common.Hash, []byte, error) {
	return common.Hash{}, nil, nil
}
func (m *noOpPostageContract) TopUpBatch(context.Context, []byte, *big.Int) (common.Hash, error) {
	return common.Hash{}, ErrChainDisabled
}
func (m *noOpPostageContract) DiluteBatch(context.Context, []byte, uint8) (common.Hash, error) {
	return common.Hash{}, ErrChainDisabled
}

func (m *noOpPostageContract) ExpireBatches(context.Context) error {
	return ErrChainDisabled
}
