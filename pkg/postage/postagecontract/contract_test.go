// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postagecontract_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	postageMock "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	transactionMock "github.com/ethersphere/bee/pkg/settlement/swap/transaction/mock"
)

func TestCreateBatch(t *testing.T) {
	owner := common.HexToAddress("abcd")
	label := "label"
	postageStampAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	ctx := context.Background()
	initialBalance := big.NewInt(100)

	t.Run("ok", func(t *testing.T) {
		depth := uint8(10)
		totalAmount := big.NewInt(102400)
		txHashApprove := common.HexToHash("abb0")
		txHashCreate := common.HexToHash("c3a7")
		batchID := common.HexToHash("dddd")
		postageMock := postageMock.New()

		expectedCallData, err := postagecontract.PostageStampABI.Pack("createBatch", owner, initialBalance, depth, common.Hash{})
		if err != nil {
			t.Fatal(err)
		}

		contract := postagecontract.New(
			owner,
			postageStampAddress,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					} else if *request.To == postageStampAddress {
						if !bytes.Equal(expectedCallData[:100], request.Data[:100]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashCreate, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					} else if txHash == txHashCreate {
						return &types.Receipt{
							Logs: []*types.Log{
								newCreateEvent(postageStampAddress, batchID),
							},
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock,
		)

		returnedID, err := contract.CreateBatch(ctx, initialBalance, depth, label)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(returnedID, batchID[:]) {
			t.Fatalf("got wrong batchId. wanted %v, got %v", batchID, returnedID)
		}

		si, err := postageMock.GetStampIssuer(returnedID)
		if err != nil {
			t.Fatal(err)
		}

		if si == nil {
			t.Fatal("stamp issuer not set")
		}
	})

	t.Run("invalid depth", func(t *testing.T) {
		depth := uint8(9)

		contract := postagecontract.New(
			owner,
			postageStampAddress,
			bzzTokenAddress,
			transactionMock.New(),
			postageMock.New(),
		)

		_, err := contract.CreateBatch(ctx, initialBalance, depth, label)
		if !errors.Is(err, postagecontract.ErrInvalidDepth) {
			t.Fatalf("expected error %v. got %v", postagecontract.ErrInvalidDepth, err)
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		depth := uint8(10)
		totalAmount := big.NewInt(102399)

		contract := postagecontract.New(
			owner,
			postageStampAddress,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return big.NewInt(0).Sub(totalAmount, big.NewInt(1)).FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock.New(),
		)

		_, err := contract.CreateBatch(ctx, initialBalance, depth, label)
		if !errors.Is(err, postagecontract.ErrInsufficientFunds) {
			t.Fatalf("expected error %v. got %v", postagecontract.ErrInsufficientFunds, err)
		}
	})
}

func newCreateEvent(postageContractAddress common.Address, batchId common.Hash) *types.Log {
	b, err := postagecontract.PostageStampABI.Events["BatchCreated"].Inputs.NonIndexed().Pack(
		big.NewInt(0),
		big.NewInt(0),
		common.Address{},
		uint8(1),
	)
	if err != nil {
		panic(err)
	}
	return &types.Log{
		Address: postageContractAddress,
		Data:    b,
		Topics:  []common.Hash{postagecontract.BatchCreatedTopic, batchId},
	}
}

func TestLookupERC20Address(t *testing.T) {
	postageStampAddress := common.HexToAddress("ffff")
	erc20Address := common.HexToAddress("ffff")

	addr, err := postagecontract.LookupERC20Address(
		context.Background(),
		transactionMock.New(
			transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
				if *request.To != postageStampAddress {
					return nil, fmt.Errorf("called wrong contract. wanted %v, got %v", postageStampAddress, request.To)
				}
				return erc20Address.Hash().Bytes(), nil
			}),
		),
		postageStampAddress,
	)
	if err != nil {
		t.Fatal(err)
	}

	if addr != postageStampAddress {
		t.Fatalf("got wrong erc20 address. wanted %v, got %v", erc20Address, addr)
	}
}
