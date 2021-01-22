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
	"github.com/ethersphere/bee/pkg/postage/listener"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/sw3-bindings/v2/simpleswapfactory"
)

var (
	postageStampABI   = parseABI(listener.PostageStampABIJSON)
	erc20ABI          = parseABI(simpleswapfactory.ERC20ABI)
	batchCreatedTopic = postageStampABI.Events["BatchCreated"].ID

	ErrBatchCreate = errors.New("batch creation failed")
)

type Interface interface {
	CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8) ([]byte, error)
}

type postageContract struct {
	owner               common.Address
	postageStampAddress common.Address
	bzzTokenAddress     common.Address
	transactionService  transaction.Service
	postageService      postage.Service
}

func New(
	owner common.Address,
	postageStampAddress common.Address,
	bzzTokenAddress common.Address,
	transactionService transaction.Service,
	postageService postage.Service,
) Interface {
	return &postageContract{
		owner:               owner,
		postageStampAddress: postageStampAddress,
		bzzTokenAddress:     bzzTokenAddress,
		transactionService:  transactionService,
		postageService:      postageService,
	}
}

func (c *postageContract) sendApproveTransaction(ctx context.Context, amount *big.Int) (*types.Receipt, error) {
	callData, err := erc20ABI.Pack("approve", c.postageStampAddress, amount)
	if err != nil {
		return nil, err
	}

	txHash, err := c.transactionService.Send(ctx, &transaction.TxRequest{
		To:       c.bzzTokenAddress,
		Data:     callData,
		GasPrice: nil,
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
		To:       c.postageStampAddress,
		Data:     callData,
		GasPrice: nil,
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

func (c *postageContract) CreateBatch(ctx context.Context, initialBalance *big.Int, depth uint8) ([]byte, error) {
	_, err := c.sendApproveTransaction(ctx, big.NewInt(0).Mul(initialBalance, big.NewInt(int64(1<<depth))))
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
		if ev.Address == c.postageStampAddress && ev.Topics[0] == batchCreatedTopic {
			var createdEvent batchCreatedEvent
			err = parseEvent(&postageStampABI, "BatchCreated", &createdEvent, *ev)
			if err != nil {
				return nil, err
			}

			batchID := createdEvent.BatchId[:]

			c.postageService.Add(postage.NewStampIssuer(
				"label",
				"keyid",
				batchID,
				depth,
				8,
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

func parseEvent(a *abi.ABI, eventName string, c interface{}, e types.Log) error {
	err := a.Unpack(c, eventName, e.Data)
	if err != nil {
		return err
	}

	var indexed abi.Arguments
	for _, arg := range a.Events[eventName].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	return abi.ParseTopics(c, indexed, e.Topics[1:])
}
