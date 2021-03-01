// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/pkg/settlement/swap/transaction/mock"
	"github.com/ethersphere/sw3-bindings/v3/simpleswapfactory"
)

func newTestFactory(t *testing.T, factoryAddress common.Address, backend transaction.Backend, transactionService transaction.Service, simpleSwapFactoryBinding chequebook.SimpleSwapFactoryBinding) (chequebook.Factory, error) {
	return chequebook.NewFactory(backend, transactionService, factoryAddress,
		func(addr common.Address, b bind.ContractBackend) (chequebook.SimpleSwapFactoryBinding, error) {
			if addr != factoryAddress {
				t.Fatalf("initialised binding with wrong address. wanted %x, got %x", factoryAddress, addr)
			}
			if b != backend {
				t.Fatal("initialised binding with wrong backend")
			}
			return simpleSwapFactoryBinding, nil
		})
}

func TestFactoryERC20Address(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	erc20Address := common.HexToAddress("0xeffff")
	factory, err := newTestFactory(
		t,
		factoryAddress,
		backendmock.New(),
		transactionmock.New(),
		&simpleSwapFactoryBindingMock{
			erc20Address: func(*bind.CallOpts) (common.Address, error) {
				return erc20Address, nil
			},
		})
	if err != nil {
		t.Fatal(err)
	}

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
	factory, err := newTestFactory(
		t,
		factoryAddress,
		backendmock.New(
			backendmock.WithCodeAtFunc(func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
				if contract != factoryAddress {
					t.Fatalf("called with wrong address. wanted %x, got %x", factoryAddress, contract)
				}
				if blockNumber != nil {
					t.Fatal("not called for latest block")
				}
				return common.FromHex(simpleswapfactory.SimpleSwapFactoryDeployedCode), nil
			}),
		),
		transactionmock.New(),
		&simpleSwapFactoryBindingMock{})
	if err != nil {
		t.Fatal(err)
	}

	err = factory.VerifyBytecode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestFactoryVerifySelfInvalidCode(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	factory, err := newTestFactory(
		t,
		factoryAddress,
		backendmock.New(
			backendmock.WithCodeAtFunc(func(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
				if contract != factoryAddress {
					t.Fatalf("called with wrong address. wanted %x, got %x", factoryAddress, contract)
				}
				if blockNumber != nil {
					t.Fatal("not called for latest block")
				}
				return common.FromHex(simpleswapfactory.AddressBin), nil
			}),
		),
		transactionmock.New(),
		&simpleSwapFactoryBindingMock{})
	if err != nil {
		t.Fatal(err)
	}

	err = factory.VerifyBytecode(context.Background())
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
	factory, err := newTestFactory(
		t,
		factoryAddress,
		backendmock.New(),
		transactionmock.New(),
		&simpleSwapFactoryBindingMock{
			deployedContracts: func(o *bind.CallOpts, address common.Address) (bool, error) {
				if address != chequebookAddress {
					t.Fatalf("checked for wrong contract. wanted %v, got %v", chequebookAddress, address)
				}
				return true, nil
			},
		})
	if err != nil {
		t.Fatal(err)
	}

	err = factory.VerifyChequebook(context.Background(), chequebookAddress)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFactoryVerifyChequebookInvalid(t *testing.T) {
	factoryAddress := common.HexToAddress("0xabcd")
	chequebookAddress := common.HexToAddress("0xefff")
	factory, err := newTestFactory(
		t,
		factoryAddress,
		backendmock.New(),
		transactionmock.New(),
		&simpleSwapFactoryBindingMock{
			deployedContracts: func(o *bind.CallOpts, address common.Address) (bool, error) {
				if address != chequebookAddress {
					t.Fatalf("checked for wrong contract. wanted %v, got %v", chequebookAddress, address)
				}
				return false, nil
			},
		})
	if err != nil {
		t.Fatal(err)
	}

	err = factory.VerifyChequebook(context.Background(), chequebookAddress)
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
	logData := common.Hex2Bytes("0xcccc")
	factory, err := newTestFactory(
		t,
		factoryAddress,
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (txHash common.Hash, err error) {
				if request.To != nil && *request.To != factoryAddress {
					t.Fatalf("sending to wrong address. wanted %x, got %x", factoryAddress, request.To)
				}
				if request.Value.Cmp(big.NewInt(0)) != 0 {
					t.Fatal("trying to send ether")
				}
				return deployTransactionHash, nil
			}),
			transactionmock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
				if txHash != deployTransactionHash {
					t.Fatalf("waiting for wrong transaction. wanted %x, got %x", deployTransactionHash, txHash)
				}
				return &types.Receipt{
					Status: 1,
					Logs: []*types.Log{
						{
							Data: logData,
						},
						{
							Address: factoryAddress,
							Data:    logData,
						},
					},
				}, nil
			}),
		),
		&simpleSwapFactoryBindingMock{
			parseSimpleSwapDeployed: func(log types.Log) (*simpleswapfactory.SimpleSwapFactorySimpleSwapDeployed, error) {
				if !bytes.Equal(log.Data, logData) {
					t.Fatal("trying to parse wrong log")
				}
				return &simpleswapfactory.SimpleSwapFactorySimpleSwapDeployed{
					ContractAddress: deployAddress,
				}, nil
			},
		})
	if err != nil {
		t.Fatal(err)
	}

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
	factory, err := newTestFactory(
		t,
		factoryAddress,
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
		&simpleSwapFactoryBindingMock{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = factory.WaitDeployed(context.Background(), deployTransactionHash)
	if err == nil {
		t.Fatal("returned failed chequebook deployment")
	}
	if !errors.Is(err, transaction.ErrTransactionReverted) {
		t.Fatalf("wrong error. wanted %v, got %v", transaction.ErrTransactionReverted, err)
	}
}
