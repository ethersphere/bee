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
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/postage"
	postagestoreMock "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postageMock "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	transactionMock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
)

var postageStampContractABI = abiutil.MustParseABI(chaincfg.Testnet.PostageStampABI)

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

		expectedCallDataForExpireLimitedBatches, err := postageStampContractABI.Pack("expireLimited", big.NewInt(50))
		if err != nil {
			t.Fatal("expected error")
		}

		expectedCallData, err := postageStampContractABI.Pack("createBatch", owner, initialBalance, depth, postagecontract.BucketDepth, common.Hash{}, false)
		if err != nil {
			t.Fatal(err)
		}

		lastPriceCallData, err := postageStampContractABI.Pack("lastPrice")
		if err != nil {
			t.Fatal(err)
		}

		minValidityBlocksCallData, err := postageStampContractABI.Pack("minimumValidityBlocks")
		if err != nil {
			t.Fatal(err)
		}

		counter := 0
		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					} else if *request.To == postageStampAddress {
						if bytes.Equal(expectedCallDataForExpireLimitedBatches[:32], request.Data[:32]) {
							return txHashApprove, nil
						}
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
					if *request.To == postageStampAddress {
						expectedCallDataForExpiredBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
						if err != nil {
							t.Fatal(err)
						}
						expectedRes := big.NewInt(1)
						expectedFalseRes := big.NewInt(0)
						if bytes.Equal(expectedCallDataForExpiredBatches[:32], request.Data[:32]) {
							{
								if counter > 1 {
									return expectedFalseRes.FillBytes(make([]byte, 32)), nil
								}
								for {
									counter++
									return expectedRes.FillBytes(make([]byte, 32)), nil
								}
							}
						}
						if bytes.Equal(lastPriceCallData, request.Data) {
							return big.NewInt(2).FillBytes(make([]byte, 32)), nil
						}
						if bytes.Equal(minValidityBlocksCallData, request.Data) {
							return big.NewInt(25).FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		_, returnedID, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(returnedID, batchID[:]) {
			t.Fatalf("got wrong batchId. wanted %v, got %v", batchID, returnedID)
		}

		si, _, err := postageMock.GetStampIssuer(returnedID)
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
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(),
			postageMock.New(),
			postagestoreMock.New(),
			true,
			false,
		)

		_, _, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
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
			postageStampContractABI,
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
			true,
			false,
		)

		_, _, err := contract.CreateBatch(ctx, initialBalance, depth, false, label)
		if !errors.Is(err, postagecontract.ErrInsufficientFunds) {
			t.Fatalf("expected error %v. got %v", postagecontract.ErrInsufficientFunds, err)
		}
	})

	t.Run("insufficient validity", func(t *testing.T) {
		depth := uint8(10)
		totalAmount := big.NewInt(102399)

		lastPriceCallData, err := postageStampContractABI.Pack("lastPrice")
		if err != nil {
			t.Fatal(err)
		}

		minValidityBlocksCallData, err := postageStampContractABI.Pack("minimumValidityBlocks")
		if err != nil {
			t.Fatal(err)
		}

		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return big.NewInt(0).Add(totalAmount, big.NewInt(1)).FillBytes(make([]byte, 32)), nil
					}
					if bytes.Equal(lastPriceCallData, request.Data) {
						return big.NewInt(2).FillBytes(make([]byte, 32)), nil
					}
					if bytes.Equal(minValidityBlocksCallData, request.Data) {
						return big.NewInt(100).FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock.New(),
			postagestoreMock.New(),
			true,
			false,
		)

		_, _, err = contract.CreateBatch(ctx, initialBalance, depth, false, label)
		if !errors.Is(err, postagecontract.ErrInsufficientValidity) {
			t.Fatalf("expected error %v. got %v", postagecontract.ErrInsufficientValidity, err)
		}
	})

}

func newCreateEvent(postageContractAddress common.Address, batchId common.Hash) *types.Log {
	event := postageStampContractABI.Events["BatchCreated"]
	b, err := event.Inputs.NonIndexed().Pack(
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
		Topics:  []common.Hash{event.ID, batchId},
	}
}

func TestTopUpBatch(t *testing.T) {
	defer func(b uint8) {
		postagecontract.BucketDepth = b
	}(postagecontract.BucketDepth)
	postagecontract.BucketDepth = 9
	owner := common.HexToAddress("abcd")
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

		expectedCallData, err := postageStampContractABI.Pack("topUp", common.BytesToHash(batch.ID), topupBalance)
		if err != nil {
			t.Fatal(err)
		}

		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					} else if *request.To == postageStampAddress {
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
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
			true,
			false,
		)

		_, err = contract.TopUpBatch(ctx, batch.ID, topupBalance)
		if err != nil {
			t.Fatal(err)
		}

		si, _, err := postageMock.GetStampIssuer(batch.ID)
		if err != nil {
			t.Fatal(err)
		}

		if si == nil {
			t.Fatal("stamp issuer not set")
		}
	})

	t.Run("batch doesn't exist", func(t *testing.T) {
		errNotFound := errors.New("not found")
		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(),
			postageMock.New(),
			postagestoreMock.New(postagestoreMock.WithGetErr(errNotFound, 0)),
			true,
			false,
		)

		_, err := contract.TopUpBatch(ctx, postagetesting.MustNewID(), topupBalance)
		if !errors.Is(err, errNotFound) {
			t.Fatal("expected error on topup of non existent batch")
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		totalAmount := big.NewInt(102399)
		batch := postagetesting.MustNewBatch(postagetesting.WithOwner(owner.Bytes()))
		batchStoreMock := postagestoreMock.New(postagestoreMock.WithBatch(batch))

		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
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
			batchStoreMock,
			true,
			false,
		)

		_, err := contract.TopUpBatch(ctx, batch.ID, topupBalance)
		if !errors.Is(err, postagecontract.ErrInsufficientFunds) {
			t.Fatalf("expected error %v. got %v", postagecontract.ErrInsufficientFunds, err)
		}
	})
}

func newTopUpEvent(postageContractAddress common.Address, batch *postage.Batch) *types.Log {
	event := postageStampContractABI.Events["BatchTopUp"]
	b, err := event.Inputs.NonIndexed().Pack(
		big.NewInt(0),
		big.NewInt(0),
	)
	if err != nil {
		panic(err)
	}
	return &types.Log{
		Address:     postageContractAddress,
		Data:        b,
		Topics:      []common.Hash{event.ID, common.BytesToHash(batch.ID)},
		BlockNumber: batch.Start + 1,
	}
}

func TestDiluteBatch(t *testing.T) {
	defer func(b uint8) {
		postagecontract.BucketDepth = b
	}(postagecontract.BucketDepth)
	postagecontract.BucketDepth = 9
	owner := common.HexToAddress("abcd")
	postageStampAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {

		txHashDilute := common.HexToHash("c3a7")
		batch := postagetesting.MustNewBatch(postagetesting.WithOwner(owner.Bytes()))
		batch.Depth = uint8(10)
		batch.BucketDepth = uint8(9)
		batch.Value = big.NewInt(100)
		newDepth := batch.Depth + 1
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

		expectedCallData, err := postageStampContractABI.Pack("increaseDepth", common.BytesToHash(batch.ID), newDepth)
		if err != nil {
			t.Fatal(err)
		}

		expectedCallDataForExpireLimitedBatches, err := postageStampContractABI.Pack("expireLimited", big.NewInt(50))
		if err != nil {
			t.Fatal("expected error")
		}

		txHashApprove := common.HexToHash("abb0")
		counter := 0
		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == postageStampAddress {
						if bytes.Equal(expectedCallDataForExpireLimitedBatches[:32], request.Data[:32]) {
							return txHashApprove, nil
						}
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDilute, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDilute {
						return &types.Receipt{
							Logs: []*types.Log{
								newDiluteEvent(postageStampAddress, batch),
							},
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageStampAddress {
						expectedCallDataForExpiredBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
						if err != nil {
							t.Fatal(err)
						}
						expectedRes := big.NewInt(1)
						expectedFalseRes := big.NewInt(0)
						if bytes.Equal(expectedCallDataForExpiredBatches[:32], request.Data[:32]) {
							{
								if counter > 1 {
									return expectedFalseRes.FillBytes(make([]byte, 32)), nil
								}
								for {
									counter++
									return expectedRes.FillBytes(make([]byte, 32)), nil
								}
							}
						}
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock,
			batchStoreMock,
			true,
			false,
		)

		_, err = contract.DiluteBatch(ctx, batch.ID, newDepth)
		if err != nil {
			t.Fatal(err)
		}

		si, _, err := postageMock.GetStampIssuer(batch.ID)
		if err != nil {
			t.Fatal(err)
		}

		if si == nil {
			t.Fatal("stamp issuer not set")
		}
	})

	t.Run("batch doesn't exist", func(t *testing.T) {
		errNotFound := errors.New("not found")
		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(),
			postageMock.New(),
			postagestoreMock.New(postagestoreMock.WithGetErr(errNotFound, 0)),
			true,
			false,
		)

		_, err := contract.DiluteBatch(ctx, postagetesting.MustNewID(), uint8(17))
		if !errors.Is(err, errNotFound) {
			t.Fatal("expected error on topup of non existent batch")
		}
	})

	t.Run("invalid depth", func(t *testing.T) {
		batch := postagetesting.MustNewBatch(postagetesting.WithOwner(owner.Bytes()))
		batch.Depth = uint8(16)
		batchStoreMock := postagestoreMock.New(postagestoreMock.WithBatch(batch))

		contract := postagecontract.New(
			owner,
			postageStampAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(),
			postageMock.New(),
			batchStoreMock,
			true,
			false,
		)

		_, err := contract.DiluteBatch(ctx, batch.ID, batch.Depth-1)
		if !errors.Is(err, postagecontract.ErrInvalidDepth) {
			t.Fatalf("expected error %v. got %v", postagecontract.ErrInvalidDepth, err)
		}
	})
}

func newDiluteEvent(postageContractAddress common.Address, batch *postage.Batch) *types.Log {
	event := postageStampContractABI.Events["BatchDepthIncrease"]
	b, err := event.Inputs.NonIndexed().Pack(
		uint8(0),
		big.NewInt(0),
	)
	if err != nil {
		panic(err)
	}
	return &types.Log{
		Address:     postageContractAddress,
		Data:        b,
		Topics:      []common.Hash{event.ID, common.BytesToHash(batch.ID)},
		BlockNumber: batch.Start + 1,
	}
}

func TestBatchExpirer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	bzzTokenAddress := common.HexToAddress("eeee")
	postageContractAddress := common.HexToAddress("ffff")
	postageMock := postageMock.New()
	owner := common.HexToAddress("abcd")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		expectedRes := big.NewInt(1)
		expectedFalseRes := big.NewInt(0)
		counter := 0
		expectedCallDataForExpiredBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
		if err != nil {
			t.Fatal(err)
		}
		contract := postagecontract.New(
			owner,
			postageContractAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageContractAddress {
						if bytes.Equal(expectedCallDataForExpiredBatches[:32], request.Data[:32]) {
							{
								if counter > 1 {
									return expectedFalseRes.FillBytes(make([]byte, 32)), nil
								}
								for {
									counter++
									return expectedRes.FillBytes(make([]byte, 32)), nil
								}
							}
						}
					}
					return nil, errors.New("unexpected call")
				}), transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, i int) (txHash common.Hash, err error) {
					return common.Hash{}, nil
				}), transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					return &types.Receipt{
						Status: 1,
					}, nil
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		err = contract.ExpireBatches(ctx)
		if err != nil {
			t.Fatal(err)
		}

	})

	t.Run("wrong call data for expired batches exist", func(t *testing.T) {
		t.Parallel()
		expectedCallDataForExpireLimitedBatches, err := postageStampContractABI.Pack("expireLimited", big.NewInt(2))
		if err != nil {
			t.Fatal("expected error")
		}
		contract := postagecontract.New(
			owner,
			postageContractAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageContractAddress {
						if !bytes.Equal(expectedCallDataForExpireLimitedBatches[:32], request.Data[:32]) {
							return nil, fmt.Errorf("wrong call data")
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		err = contract.ExpireBatches(ctx)
		if err == nil {
			t.Fatal("expected error")
		}

	})

	t.Run("wrong call data for expired limited batches", func(t *testing.T) {
		t.Parallel()
		expectedCallDataForExpireLimitedBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
		if err != nil {
			t.Fatal("expected error")
		}
		contract := postagecontract.New(
			owner,
			postageContractAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageContractAddress {
						if !bytes.Equal(expectedCallDataForExpireLimitedBatches[:32], request.Data[:32]) {
							return nil, fmt.Errorf("wrong call data")
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		err = contract.ExpireBatches(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("correct and incorrect call data", func(t *testing.T) {
		t.Parallel()

		_, err := postageStampContractABI.Pack("expiredBatchesExist")
		if err != nil {
			t.Fatal(err)
		}
		_, err = postageStampContractABI.Pack("expireLimited", big.NewInt(2))
		if err != nil {
			t.Fatal(err)
		}

		_, err = postageStampContractABI.Pack("expiredBatchesExist", "someVal")
		if err == nil {
			t.Fatal("expected error")
		}
		_, err = postageStampContractABI.Pack("expireLimited")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("wrong result for expire limited batches", func(t *testing.T) {
		t.Parallel()
		expectedRes := big.NewInt(1)
		expectedFalseRes := big.NewInt(0)
		counter := 0
		expectedCallDataForExpireLimitedBatches, err := postageStampContractABI.Pack("expireLimited", big.NewInt(1000))
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForExpiredBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
		if err != nil {
			t.Fatal(err)
		}
		contract := postagecontract.New(
			owner,
			postageContractAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageContractAddress {
						if bytes.Equal(expectedCallDataForExpiredBatches[:32], request.Data[:32]) {
							{
								if counter > 1 {
									return expectedFalseRes.FillBytes(make([]byte, 32)), nil
								}
								for {
									counter++
									return expectedRes.FillBytes(make([]byte, 32)), nil
								}
							}
						}
					}
					return nil, errors.New("unexpected call")
				}), transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, i int) (txHash common.Hash, err error) {
					if *request.To == postageContractAddress {
						if bytes.Equal(expectedCallDataForExpireLimitedBatches[:32], request.Data[:32]) {
							return txHash, fmt.Errorf("some error")
						}
					}
					return txHash, errors.New("unexpected call")
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		err = contract.ExpireBatches(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("wrong result for expired batches exist", func(t *testing.T) {
		t.Parallel()
		expectedCallDataForExpiredBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
		if err != nil {
			t.Fatal(err)
		}
		contract := postagecontract.New(
			owner,
			postageContractAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageContractAddress {
						if bytes.Equal(expectedCallDataForExpiredBatches[:32], request.Data[:32]) {
							{
								return nil, fmt.Errorf("some error")
							}
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		err = contract.ExpireBatches(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unpack err for expired batches exist", func(t *testing.T) {
		t.Parallel()
		expectedRes := big.NewInt(1)
		expectedCallDataForExpiredBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
		if err != nil {
			t.Fatal(err)
		}
		contract := postagecontract.New(
			owner,
			postageContractAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageContractAddress {
						if bytes.Equal(expectedCallDataForExpiredBatches[:32], request.Data[:32]) {
							{
								return []byte("someWrongData"), nil
							}
						}
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		err = contract.ExpireBatches(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("tx err for expire limited batches", func(t *testing.T) {
		t.Parallel()
		expectedRes := big.NewInt(1)
		expectedFalseRes := big.NewInt(0)
		counter := 0
		expectedCallDataForExpiredBatches, err := postageStampContractABI.Pack("expiredBatchesExist")
		if err != nil {
			t.Fatal(err)
		}
		contract := postagecontract.New(
			owner,
			postageContractAddress,
			postageStampContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == postageContractAddress {
						if bytes.Equal(expectedCallDataForExpiredBatches[:32], request.Data[:32]) {
							{
								if counter > 1 {
									return expectedFalseRes.FillBytes(make([]byte, 32)), nil
								}
								for {
									counter++
									return expectedRes.FillBytes(make([]byte, 32)), nil
								}
							}
						}
					}
					return nil, errors.New("unexpected call")
				}), transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, i int) (txHash common.Hash, err error) {
					return common.Hash{}, nil
				}), transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					return &types.Receipt{
						Status: 0,
					}, nil
				}),
			),
			postageMock,
			postagestoreMock.New(),
			true,
			false,
		)

		err = contract.ExpireBatches(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLookupERC20Address(t *testing.T) {
	postageStampContractAddress := common.HexToAddress("ffff")
	erc20Address := common.HexToAddress("ffff")

	addr, err := postagecontract.LookupERC20Address(
		context.Background(),
		transactionMock.New(
			transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
				if *request.To != postageStampContractAddress {
					return nil, fmt.Errorf("called wrong contract. wanted %v, got %v", postageStampContractAddress, request.To)
				}
				return common.BytesToHash(erc20Address.Bytes()).Bytes(), nil
			}),
		),
		postageStampContractAddress,
		postageStampContractABI,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	if addr != postageStampContractAddress {
		t.Fatalf("got wrong erc20 address. wanted %v, got %v", erc20Address, addr)
	}
}
