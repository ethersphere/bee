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
	"github.com/ethersphere/bee/pkg/postage"
	postagestoreMock "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postageMock "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/transaction"
	transactionMock "github.com/ethersphere/bee/pkg/transaction/mock"
)

func TestCreateBatch(t *testing.T) {
	defer func(b uint8) {
		postagecontract.BucketDepth = b
	}(postagecontract.BucketDepth)
	postagecontract.BucketDepth = 9
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

		expectedCallData, err := postagecontract.PostageStampABI.Pack("createBatch", owner, initialBalance, depth, postagecontract.BucketDepth, common.Hash{}, false)
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
			postagestoreMock.New(),
		)

		returnedID, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
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
			postagestoreMock.New(),
		)

		_, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
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
			postagestoreMock.New(),
		)

		_, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
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
		uint8(2),
		false,
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

func TestTopUpBatch(t *testing.T) {
	defer func(b uint8) {
		postagecontract.BucketDepth = b
	}(postagecontract.BucketDepth)
	postagecontract.BucketDepth = 9
	owner := common.HexToAddress("abcd")
	label := "label"
	postageStampAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	ctx := context.Background()
	topupBalance := big.NewInt(100)

	t.Run("ok", func(t *testing.T) {

		totalAmount := big.NewInt(102400)
		txHashApprove := common.HexToHash("abb0")
		txHashTopup := common.HexToHash("c3a7")
		batch := postagetesting.MustNewBatch(postagetesting.WithOwner(owner.Bytes()))
		batch.Depth = uint8(10)
		batch.BucketDepth = uint8(9)
		postageMock := postageMock.New(postageMock.WithIssuer(postage.NewStampIssuer(
			"label",
			"keyID",
			batch.ID,
			batch.Value,
			batch.Depth,
			batch.BucketDepth,
			batch.Start,
			batch.Immutable,
		)))
		batchStoreMock := postagestoreMock.New(postagestoreMock.WithBatch(batch))

		expectedCallData, err := postagecontract.PostageStampABI.Pack("topupBatch", batch.ID, topupBalance)
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
						return txHashTopup, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					} else if txHash == txHashTopup {
						return &types.Receipt{
							Logs: []*types.Log{
								newTopUpEvent(postageStampAddress, batch),
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
			batchStoreMock,
		)

		err = contract.TopUpBatch(ctx, batch.ID, topupBalance)
		if err != nil {
			t.Fatal(err)
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

		_, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
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

		_, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
		if !errors.Is(err, postagecontract.ErrInsufficientFunds) {
			t.Fatalf("expected error %v. got %v", postagecontract.ErrInsufficientFunds, err)
		}
	})
}

func newTopUpEvent(postageContractAddress common.Address, batch *postage.Batch) *types.Log {
	b, err := postagecontract.PostageStampABI.Events["BatchTopUp"].Inputs.NonIndexed().Pack(
		big.NewInt(0),
		big.NewInt(0),
	)
	if err != nil {
		panic(err)
	}
	return &types.Log{
		Address:     postageContractAddress,
		Data:        b,
		Topics:      []common.Hash{postagecontract.BatchTopUpTopic, common.BytesToHash(batch.ID)},
		BlockNumber: batch.Start + 1,
	}
}
