// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/pkg/settlement/swap/transaction/mock"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	factoryABI              = transaction.ParseABIUnchecked(sw3abi.SimpleSwapFactoryABIv0_3_1)
	simpleSwapDeployedEvent = factoryABI.Events["SimpleSwapDeployed"]
)

func TestFactoryERC20Address(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	erc20Address := common.HexToAddress("0xeffff")
	factory := chequebook.NewFactory(
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithABICall(
				&factoryABI,
				erc20Address.Hash().Bytes(),
				"ERC20Address",
			),
		),
		factoryAddress,
	)

	addr, err := factory.ERC20Address(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if addr != erc20Address {
		t.Fatalf("wrong erc20Address. wanted %x, got %x", erc20Address, addr)
	}
}

func TestFactoryVerifySelf(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	factory := chequebook.NewFactory(
		backendmock.New(
			backendmock.WithCodeAtFunc(func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
				if contract != factoryAddress {
					t.Fatalf("called with wrong address. wanted %x, got %x", factoryAddress, contract)
				}
				if blockNumber != nil {
					t.Fatal("not called for latest block")
				}
				return common.FromHex(sw3abi.SimpleSwapFactoryDeployedBinv0_3_1), nil
			}),
		),
		transactionmock.New(),
		factoryAddress,
	)

	err := factory.VerifyBytecode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestFactoryVerifySelfInvalidCode(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	factory := chequebook.NewFactory(
		backendmock.New(
			backendmock.WithCodeAtFunc(func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
				if contract != factoryAddress {
					t.Fatalf("called with wrong address. wanted %x, got %x", factoryAddress, contract)
				}
				if blockNumber != nil {
					t.Fatal("not called for latest block")
				}
				return common.FromHex("abcd"), nil
			}),
		),
		transactionmock.New(),
		factoryAddress,
	)

	err := factory.VerifyBytecode(context.Background())
	if err == nil {
		t.Fatal("verified invalid factory")
	}
	if !errors.Is(err, chequebook.ErrInvalidFactory) {
		t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrInvalidFactory, err)
	}
}

func TestFactoryVerifyChequebook(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	chequebookAddress := common.HexToAddress("0xefff")
	factory := chequebook.NewFactory(
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithABICall(
				&factoryABI,
				common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001"),
				"deployedContracts",
				chequebookAddress,
			),
		),
		factoryAddress,
	)
	err := factory.VerifyChequebook(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFactoryVerifyChequebookInvalid(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	chequebookAddress := common.HexToAddress("0xefff")
	factory := chequebook.NewFactory(
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithABICall(
				&factoryABI,
				common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000000"),
				"deployedContracts",
				chequebookAddress,
			),
		),
		factoryAddress,
	)

	err := factory.VerifyChequebook(context.Background(), chequebookAddress)
	if err == nil {
		t.Fatal("verified invalid chequebook")
	}
	if !errors.Is(err, chequebook.ErrNotDeployedByFactory) {
		t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrNotDeployedByFactory, err)
	}
}

func TestFactoryDeploy(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	issuerAddress := common.HexToAddress("0xefff")
	defaultTimeout := big.NewInt(1)
	deployTransactionHash := common.HexToHash("0xffff")
	deployAddress := common.HexToAddress("0xdddd")
	factory := chequebook.NewFactory(
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithABISend(&factoryABI, deployTransactionHash, factoryAddress, big.NewInt(0), "deploySimpleSwap", issuerAddress, defaultTimeout),
			transactionmock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
				if txHash != deployTransactionHash {
					t.Fatalf("waiting for wrong transaction. wanted %x, got %x", deployTransactionHash, txHash)
				}
				logData, err := simpleSwapDeployedEvent.Inputs.NonIndexed().Pack(deployAddress)
				if err != nil {
					t.Fatal(err)
				}
				return &types.Receipt{
					Status: 1,
					Logs: []*types.Log{
						{
							Data: logData,
						},
						{
							Address: factoryAddress,
							Topics:  []common.Hash{simpleSwapDeployedEvent.ID},
							Data:    logData,
						},
					},
				}, nil
			},
			)),
		factoryAddress,
	)

	txHash, err := factory.Deploy(context.Background(), issuerAddress, defaultTimeout)
	if err != nil {
		t.Fatal(err)
	}

	if txHash != deployTransactionHash {
		t.Fatalf("returning wrong transaction hash. wanted %x, got %x", deployTransactionHash, txHash)
	}

	chequebookAddress, err := factory.WaitDeployed(context.Background(), txHash)
	if err != nil {
		t.Fatal(err)
	}

	if chequebookAddress != deployAddress {
		t.Fatalf("returning wrong address. wanted %x, got %x", deployAddress, chequebookAddress)
	}
}

func TestFactoryDeployReverted(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	deployTransactionHash := common.HexToHash("0xffff")
	factory := chequebook.NewFactory(
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
				if txHash != deployTransactionHash {
					t.Fatalf("waiting for wrong transaction. wanted %x, got %x", deployTransactionHash, txHash)
				}
				return &types.Receipt{
					Status: 0,
				}, nil
			}),
		),
		factoryAddress,
	)

	_, err := factory.WaitDeployed(context.Background(), deployTransactionHash)
	if err == nil {
		t.Fatal("returned failed chequebook deployment")
	}
	if !errors.Is(err, transaction.ErrTransactionReverted) {
		t.Fatalf("wrong error. wanted %v, got %v", transaction.ErrTransactionReverted, err)
	}
}
