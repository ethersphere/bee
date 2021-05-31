// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
	"github.com/ethersphere/go-sw3-abi/sw3abi"
)

var (
	factoryABI              = transaction.ParseABIUnchecked(sw3abi.SimpleSwapFactoryABIv0_4_0)
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
				factoryAddress,
				erc20Address.Hash().Bytes(),
				"ERC20Address",
			),
		),
		factoryAddress,
		nil,
	)

	addr, err := factory.ERC20Address(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if addr != erc20Address {
		t.Fatalf("wrong erc20Address. wanted %x, got %x", erc20Address, addr)
	}
}

func backendWithCodeAt(codeMap map[common.Address]string) transaction.Backend {
	return backendmock.New(
		backendmock.WithCodeAtFunc(func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
			code, ok := codeMap[contract]
			if !ok {
				return nil, fmt.Errorf("called with wrong address. wanted one of %v, got %x", codeMap, contract)
			}
			if blockNumber != nil {
				return nil, errors.New("not called for latest block")
			}
			return common.FromHex(code), nil
		}),
	)
}

func TestFactoryVerifySelf(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	legacyFactory1 := common.HexToAddress("0xbbbb")
	legacyFactory2 := common.HexToAddress("0xcccc")

	t.Run("valid", func(t *testing.T) {
		factory := chequebook.NewFactory(
			backendWithCodeAt(map[common.Address]string{
				factoryAddress: sw3abi.SimpleSwapFactoryDeployedBinv0_4_0,
				legacyFactory1: sw3abi.SimpleSwapFactoryDeployedBinv0_3_1,
				legacyFactory2: sw3abi.SimpleSwapFactoryDeployedBinv0_3_1,
			}),
			transactionmock.New(),
			factoryAddress,
			[]common.Address{legacyFactory1, legacyFactory2},
		)

		err := factory.VerifyBytecode(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid deploy factory", func(t *testing.T) {
		factory := chequebook.NewFactory(
			backendWithCodeAt(map[common.Address]string{
				factoryAddress: "abcd",
			}),
			transactionmock.New(),
			factoryAddress,
			nil,
		)

		err := factory.VerifyBytecode(context.Background())
		if err == nil {
			t.Fatal("verified invalid factory")
		}
		if !errors.Is(err, chequebook.ErrInvalidFactory) {
			t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrInvalidFactory, err)
		}
	})

	t.Run("invalid legacy factories", func(t *testing.T) {
		factory := chequebook.NewFactory(
			backendWithCodeAt(map[common.Address]string{
				factoryAddress: sw3abi.SimpleSwapFactoryDeployedBinv0_4_0,
				legacyFactory1: sw3abi.SimpleSwapFactoryDeployedBinv0_3_1,
				legacyFactory2: "abcd",
			}),
			transactionmock.New(),
			factoryAddress,
			[]common.Address{legacyFactory1, legacyFactory2},
		)

		err := factory.VerifyBytecode(context.Background())
		if err == nil {
			t.Fatal("verified invalid factory")
		}
		if !errors.Is(err, chequebook.ErrInvalidFactory) {
			t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrInvalidFactory, err)
		}
	})
}

func TestFactoryVerifyChequebook(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	chequebookAddress := common.HexToAddress("0xefff")
	legacyFactory1 := common.HexToAddress("0xbbbb")
	legacyFactory2 := common.HexToAddress("0xcccc")

	t.Run("valid", func(t *testing.T) {
		factory := chequebook.NewFactory(
			backendmock.New(),
			transactionmock.New(
				transactionmock.WithABICall(
					&factoryABI,
					factoryAddress,
					common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001"),
					"deployedContracts",
					chequebookAddress,
				),
			),
			factoryAddress,
			[]common.Address{legacyFactory1, legacyFactory2},
		)
		err := factory.VerifyChequebook(context.Background(), chequebookAddress)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("valid legacy", func(t *testing.T) {
		factory := chequebook.NewFactory(
			backendmock.New(),
			transactionmock.New(
				transactionmock.WithABICallSequence(
					transactionmock.ABICall(
						&factoryABI,
						factoryAddress,
						common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000000"),
						"deployedContracts",
						chequebookAddress,
					),
					transactionmock.ABICall(
						&factoryABI,
						legacyFactory1,
						common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000000"),
						"deployedContracts",
						chequebookAddress,
					),
					transactionmock.ABICall(
						&factoryABI,
						legacyFactory2,
						common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001"),
						"deployedContracts",
						chequebookAddress,
					),
				)),
			factoryAddress,
			[]common.Address{legacyFactory1, legacyFactory2},
		)

		err := factory.VerifyChequebook(context.Background(), chequebookAddress)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		factory := chequebook.NewFactory(
			backendmock.New(),
			transactionmock.New(
				transactionmock.WithABICallSequence(
					transactionmock.ABICall(
						&factoryABI,
						factoryAddress,
						common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000000"),
						"deployedContracts",
						chequebookAddress,
					),
					transactionmock.ABICall(
						&factoryABI,
						legacyFactory1,
						common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000000"),
						"deployedContracts",
						chequebookAddress,
					),
					transactionmock.ABICall(
						&factoryABI,
						legacyFactory2,
						common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000000"),
						"deployedContracts",
						chequebookAddress,
					),
				)),
			factoryAddress,
			[]common.Address{legacyFactory1, legacyFactory2},
		)

		err := factory.VerifyChequebook(context.Background(), chequebookAddress)
		if err == nil {
			t.Fatal("verified invalid chequebook")
		}
		if !errors.Is(err, chequebook.ErrNotDeployedByFactory) {
			t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrNotDeployedByFactory, err)
		}
	})
}

func TestFactoryDeploy(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	issuerAddress := common.HexToAddress("0xefff")
	defaultTimeout := big.NewInt(1)
	deployTransactionHash := common.HexToHash("0xffff")
	deployAddress := common.HexToAddress("0xdddd")
	nonce := common.HexToHash("eeff")

	factory := chequebook.NewFactory(
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithABISend(&factoryABI, deployTransactionHash, factoryAddress, big.NewInt(0), "deploySimpleSwap", issuerAddress, defaultTimeout, nonce),
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
		nil,
	)

	txHash, err := factory.Deploy(context.Background(), issuerAddress, defaultTimeout, nonce)
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
		nil,
	)

	_, err := factory.WaitDeployed(context.Background(), deployTransactionHash)
	if err == nil {
		t.Fatal("returned failed chequebook deployment")
	}
	if !errors.Is(err, transaction.ErrTransactionReverted) {
		t.Fatalf("wrong error. wanted %v, got %v", transaction.ErrTransactionReverted, err)
	}
}
