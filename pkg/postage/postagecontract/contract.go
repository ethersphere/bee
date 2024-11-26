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

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	BucketDepth = uint8(16)

	erc20ABI = abiutil.MustParseABI(sw3abi.ERC20ABIv0_6_5)

	ErrBatchCreate          = errors.New("batch creation failed")
	ErrInsufficientFunds    = errors.New("insufficient token balance")
	ErrInvalidDepth         = errors.New("invalid depth")
	ErrBatchTopUp           = errors.New("batch topUp failed")
	ErrBatchDilute          = errors.New("batch dilute failed")
	ErrChainDisabled        = errors.New("chain disabled")
	ErrNotImplemented       = errors.New("not implemented")
	ErrInsufficientValidity = errors.New("insufficient validity")

	approveDescription     = "Approve tokens for postage operations"
	createBatchDescription = "Postage batch creation"
	topUpBatchDescription  = "Postage batch top up"
	diluteBatchDescription = "Postage batch dilute"
)

type Interface interface {
	CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, immutable bool, label string) (common.Hash, []byte, error)
	TopUpBatch(ctx context.Context, batchID []byte, topupBalance *big.Int) (common.Hash, error)
	DiluteBatch(ctx context.Context, batchID []byte, newDepth uint8) (common.Hash, error)
	Paused(ctx context.Context) (bool, error)
	PostageBatchExpirer
}

type PostageBatchExpirer interface {
	ExpireBatches(ctx context.Context) error
}

type postageContract struct {
	owner                       common.Address
	postageStampContractAddress common.Address
	postageStampContractABI     abi.ABI
	bzzTokenAddress             common.Address
	transactionService          transaction.Service
	postageService              postage.Service
	postageStorer               postage.Storer

	// Cached postage stamp contract event topics.
	batchCreatedTopic       common.Hash
	batchTopUpTopic         common.Hash
	batchDepthIncreaseTopic common.Hash

	gasLimit uint64
}

func New(
	owner common.Address,
	postageStampContractAddress common.Address,
	postageStampContractABI abi.ABI,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
	postageService postage.Service,
	postageStorer postage.Storer,
	chainEnabled bool,
	setGasLimit bool,
) Interface {
	if !chainEnabled {
		return new(noOpPostageContract)
	}

	var gasLimit uint64
	if setGasLimit {
		gasLimit = transaction.DefaultGasLimit
	}

	return &postageContract{
		owner:                       owner,
		postageStampContractAddress: postageStampContractAddress,
		postageStampContractABI:     postageStampContractABI,
		bzzTokenAddress:             bzzTokenAddress,
		transactionService:          transactionService,
		postageService:              postageService,
		postageStorer:               postageStorer,

		batchCreatedTopic:       postageStampContractABI.Events["BatchCreated"].ID,
		batchTopUpTopic:         postageStampContractABI.Events["BatchTopUp"].ID,
		batchDepthIncreaseTopic: postageStampContractABI.Events["BatchDepthIncrease"].ID,

		gasLimit: gasLimit,
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

		err = c.expireLimitedBatches(ctx, big.NewInt(25))
		if err != nil {
			return fmt.Errorf("expire limited batches: %w", err)
		}
	}
	return nil
}

func (c *postageContract) expiredBatchesExists(ctx context.Context) (bool, error) {
	callData, err := c.postageStampContractABI.Pack("expiredBatchesExist")
	if err != nil {
		return false, err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.postageStampContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := c.postageStampContractABI.Unpack("expiredBatchesExist", result)
	if err != nil {
		return false, err
	}
	return results[0].(bool), nil
}

func (c *postageContract) expireLimitedBatches(ctx context.Context, count *big.Int) error {
	callData, err := c.postageStampContractABI.Pack("expireLimited", count)
	if err != nil {
		return err
	}

	_, err = c.sendTransaction(ctx, callData, "expire limited batches")
	if err != nil {
		return err
	}

	return nil
}

func (c *postageContract) sendApproveTransaction(ctx context.Context, amount *big.Int) (receipt *types.Receipt, err error) {
	callData, err := erc20ABI.Pack("approve", c.postageStampContractAddress, amount)
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
			c.postageStampContractABI.Errors,
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

func (c *postageContract) sendTransaction(ctx context.Context, callData []byte, desc string) (receipt *types.Receipt, err error) {
	request := &transaction.TxRequest{
		To:          &c.postageStampContractAddress,
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
			c.postageStampContractABI.Errors,
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

func (c *postageContract) sendCreateBatchTransaction(ctx context.Context, owner common.Address, initialBalance *big.Int, depth uint8, nonce common.Hash, immutable bool) (*types.Receipt, error) {

	callData, err := c.postageStampContractABI.Pack("createBatch", owner, initialBalance, depth, BucketDepth, nonce, immutable)
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

	callData, err := c.postageStampContractABI.Pack("topUp", common.BytesToHash(batchID), topUpAmount)
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

	callData, err := c.postageStampContractABI.Pack("increaseDepth", common.BytesToHash(batchID), newDepth)
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

func (c *postageContract) getProperty(ctx context.Context, propertyName string, out any) error {
	callData, err := c.postageStampContractABI.Pack(propertyName)
	if err != nil {
		return err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.postageStampContractAddress,
		Data: callData,
	})
	if err != nil {
		return err
	}

	results, err := c.postageStampContractABI.Unpack(propertyName, result)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return errors.New("unexpected empty results")
	}

	abi.ConvertType(results[0], out)
	return nil
}

func (c *postageContract) getMinInitialBalance(ctx context.Context) (uint64, error) {
	var lastPrice uint64
	err := c.getProperty(ctx, "lastPrice", &lastPrice)
	if err != nil {
		return 0, err
	}
	var minimumValidityBlocks uint64
	err = c.getProperty(ctx, "minimumValidityBlocks", &minimumValidityBlocks)
	if err != nil {
		return 0, err
	}
	return lastPrice * minimumValidityBlocks, nil
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
		err = fmt.Errorf("insufficient balance. amount %d, balance %d: %w", totalAmount, balance, ErrInsufficientFunds)
		return
	}

	minInitialBalance, err := c.getMinInitialBalance(ctx)
	if err != nil {
		return
	}
	if initialBalance.Cmp(big.NewInt(int64(minInitialBalance))) <= 0 {
		err = fmt.Errorf("insufficient initial balance for 24h minimum validity. balance %d, minimum amount: %d: %w", initialBalance, minInitialBalance, ErrInsufficientValidity)
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
		if ev.Address == c.postageStampContractAddress && len(ev.Topics) > 0 && ev.Topics[0] == c.batchCreatedTopic {
			var createdEvent batchCreatedEvent
			err = transaction.ParseEvent(&c.postageStampContractABI, "BatchCreated", &createdEvent, *ev)

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
		if ev.Address == c.postageStampContractAddress && len(ev.Topics) > 0 && ev.Topics[0] == c.batchTopUpTopic {
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
		if ev.Address == c.postageStampContractAddress && len(ev.Topics) > 0 && ev.Topics[0] == c.batchDepthIncreaseTopic {
			return
		}
	}
	err = ErrBatchDilute
	return
}

func (c *postageContract) Paused(ctx context.Context) (bool, error) {
	callData, err := c.postageStampContractABI.Pack("paused")
	if err != nil {
		return false, err
	}

	result, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.postageStampContractAddress,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := c.postageStampContractABI.Unpack("paused", result)
	if err != nil {
		return false, err
	}

	if len(results) == 0 {
		return false, errors.New("unexpected empty results")
	}

	return results[0].(bool), nil
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

func (m *noOpPostageContract) Paused(context.Context) (bool, error) {
	return false, nil
}

func (m *noOpPostageContract) ExpireBatches(context.Context) error {
	return ErrChainDisabled
}

func LookupERC20Address(ctx context.Context, transactionService transaction.Service, postageStampContractAddress common.Address, postageStampContractABI abi.ABI, chainEnabled bool) (common.Address, error) {
	if !chainEnabled {
		return common.Address{}, nil
	}

	callData, err := postageStampContractABI.Pack("bzzToken")
	if err != nil {
		return common.Address{}, err
	}

	request := &transaction.TxRequest{
		To:       &postageStampContractAddress,
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
