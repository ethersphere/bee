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
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/go-storage-incentives-abi/postageabi"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	BucketDepth = uint8(16)

	postageStampABI   = parseABI(postageabi.PostageStampABIv0_1_0)
	erc20ABI          = parseABI(sw3abi.ERC20ABIv0_3_1)
	batchCreatedTopic = postageStampABI.Events["BatchCreated"].ID

	ErrBatchCreate       = errors.New("batch creation failed")
	ErrInsufficientFunds = errors.New("insufficient token balance")
	ErrInvalidDepth      = errors.New("invalid depth")
)

type Interface interface {
	CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, label string) ([]byte, error)
}

type postageContract struct {
	owner                  common.Address
	postageContractAddress common.Address
	bzzTokenAddress        common.Address
	transactionService     transaction.Service
	postageService         postage.Service
}

func New(
	owner,
	postageContractAddress,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
	postageService postage.Service,
) Interface {
	return &postageContract{
		owner:                  owner,
		postageContractAddress: postageContractAddress,
		bzzTokenAddress:        bzzTokenAddress,
		transactionService:     transactionService,
		postageService:         postageService,
	}
}

func (c *postageContract) sendApproveTransaction(ctx context.Context, amount *big.Int) (*types.Receipt, error) {
	callData, err := erc20ABI.Pack("approve", c.postageContractAddress, amount)
	if err != nil {
		return nil, err
	}

	txHash, err := c.transactionService.Send(ctx, &transaction.TxRequest{
		To:       &c.bzzTokenAddress,
		Data:     callData,
		GasPrice: sctx.GetGasPrice(ctx),
		GasLimit: 0,
		Value:    big.NewInt(0),
	})
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

func (c *postageContract) sendCreateBatchTransaction(ctx context.Context, owner common.Address, initialBalance *big.Int, depth uint8, nonce common.Hash) (*types.Receipt, error) {
	callData, err := postageStampABI.Pack("createBatch", owner, initialBalance, depth, nonce)
	if err != nil {
		return nil, err
	}

	request := &transaction.TxRequest{
		To:       &c.postageContractAddress,
		Data:     callData,
		GasPrice: sctx.GetGasPrice(ctx),
		GasLimit: 0,
		Value:    big.NewInt(0),
	}

	txHash, err := c.transactionService.Send(ctx, request)
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

func (c *postageContract) CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8, label string) ([]byte, error) {

	if depth < BucketDepth {
		return nil, ErrInvalidDepth
	}

	totalAmount := big.NewInt(0).Mul(initialBalance, big.NewInt(int64(1<<depth)))
	balance, err := c.getBalance(ctx)
	if err != nil {
		return nil, err
	}

	if balance.Cmp(totalAmount) < 0 {
		return nil, ErrInsufficientFunds
	}

	_, err = c.sendApproveTransaction(ctx, totalAmount)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 32)
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, err
	}

	receipt, err := c.sendCreateBatchTransaction(ctx, c.owner, initialBalance, depth, common.BytesToHash(nonce))
	if err != nil {
		return nil, err
	}

	for _, ev := range receipt.Logs {
		if ev.Address == c.postageContractAddress && ev.Topics[0] == batchCreatedTopic {
			var createdEvent batchCreatedEvent
			err = transaction.ParseEvent(&postageStampABI, "BatchCreated", &createdEvent, *ev)
			if err != nil {
				return nil, err
			}

			batchID := createdEvent.BatchId[:]

			c.postageService.Add(postage.NewStampIssuer(
				label,
				c.owner.Hex(),
				batchID,
				depth,
				BucketDepth,
			))

			return createdEvent.BatchId[:], nil
		}
	}

	return nil, ErrBatchCreate
}

type batchCreatedEvent struct {
	BatchId           [32]byte
	TotalAmount       *big.Int
	NormalisedBalance *big.Int
	Owner             common.Address
	Depth             uint8
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for postage contract: %v", err))
	}
	return cabi
}

func LookupERC20Address(ctx context.Context, transactionService transaction.Service, postageContractAddress common.Address) (common.Address, error) {
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
