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

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	erc20mock "github.com/ethersphere/bee/pkg/settlement/swap/erc20/mock"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/pkg/settlement/swap/transaction/mock"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
)

func newTestChequebook(
	t *testing.T,
	backend transaction.Backend,
	transactionService transaction.Service,
	address,
	ownerAdress common.Address,
	store storage.StateStorer,
	chequeSigner chequebook.ChequeSigner,
	erc20 erc20.Service,
	simpleSwapBinding chequebook.SimpleSwapBinding,
) (chequebook.Service, error) {
	return chequebook.New(
		backend,
		transactionService,
		address,
		ownerAdress,
		store,
		chequeSigner,
		erc20,
		func(addr common.Address, b bind.ContractBackend) (chequebook.SimpleSwapBinding, error) {
			if addr != address {
				t.Fatalf("initialised binding with wrong address. wanted %x, got %x", address, addr)
			}
			if b != backend {
				t.Fatal("initialised binding with wrong backend")
			}
			return simpleSwapBinding, nil
		},
	)
}

func TestChequebookAddress(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(),
		&simpleSwapBindingMock{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if chequebookService.Address() != address {
		t.Fatalf("returned wrong address. wanted %x, got %x", address, chequebookService.Address())
	}
}

func TestChequebookBalance(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	balance := big.NewInt(10)
	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(),
		&simpleSwapBindingMock{
			balance: func(*bind.CallOpts) (*big.Int, error) {
				return balance, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	returnedBalance, err := chequebookService.Balance(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if returnedBalance.Cmp(balance) != 0 {
		t.Fatalf("returned wrong balance. wanted %d, got %d", balance, returnedBalance)
	}
}

func TestChequebookDeposit(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	balance := big.NewInt(30)
	depositAmount := big.NewInt(20)
	txHash := common.HexToHash("0xdddd")
	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(
			erc20mock.WithBalanceOfFunc(func(ctx context.Context, address common.Address) (*big.Int, error) {
				if address != ownerAdress {
					return nil, errors.New("getting balance of wrong address")
				}
				return balance, nil
			}),
			erc20mock.WithTransferFunc(func(ctx context.Context, to common.Address, value *big.Int) (common.Hash, error) {
				if to != address {
					return common.Hash{}, fmt.Errorf("sending to wrong address. wanted %x, got %x", address, to)
				}
				if depositAmount.Cmp(value) != 0 {
					return common.Hash{}, fmt.Errorf("sending wrong value. wanted %d, got %d", depositAmount, value)
				}
				return txHash, nil
			}),
		),
		&simpleSwapBindingMock{},
	)
	if err != nil {
		t.Fatal(err)
	}

	returnedTxHash, err := chequebookService.Deposit(context.Background(), depositAmount)
	if err != nil {
		t.Fatal(err)
	}

	if txHash != returnedTxHash {
		t.Fatalf("returned wrong transaction hash. wanted %v, got %v", txHash, returnedTxHash)
	}
}

func TestChequebookWaitForDeposit(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	txHash := common.HexToHash("0xdddd")
	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithWaitForReceiptFunc(func(ctx context.Context, tx common.Hash) (*types.Receipt, error) {
				if tx != txHash {
					t.Fatalf("waiting for wrong transaction. wanted %x, got %x", txHash, tx)
				}
				return &types.Receipt{
					Status: 1,
				}, nil
			}),
		),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(),
		&simpleSwapBindingMock{},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = chequebookService.WaitForDeposit(context.Background(), txHash)
	if err != nil {
		t.Fatal(err)
	}
}

func TestChequebookWaitForDepositReverted(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	txHash := common.HexToHash("0xdddd")
	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithWaitForReceiptFunc(func(ctx context.Context, tx common.Hash) (*types.Receipt, error) {
				if tx != txHash {
					t.Fatalf("waiting for wrong transaction. wanted %x, got %x", txHash, tx)
				}
				return &types.Receipt{
					Status: 0,
				}, nil
			}),
		),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(),
		&simpleSwapBindingMock{},
	)
	if err != nil {
		t.Fatal(err)
	}

	err = chequebookService.WaitForDeposit(context.Background(), txHash)
	if err == nil {
		t.Fatal("expected reverted error")
	}
	if !errors.Is(err, transaction.ErrTransactionReverted) {
		t.Fatalf("wrong error. wanted %v, got %v", transaction.ErrTransactionReverted, err)
	}
}

func TestChequebookIssue(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	beneficiary := common.HexToAddress("0xdddd")
	ownerAdress := common.HexToAddress("0xfff")
	store := storemock.NewStateStore()
	amount := big.NewInt(20)
	amount2 := big.NewInt(30)
	expectedCumulative := big.NewInt(50)
	sig := common.Hex2Bytes("0xffff")
	chequeSigner := &chequeSignerMock{}

	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(),
		address,
		ownerAdress,
		store,
		chequeSigner,
		erc20mock.New(),
		&simpleSwapBindingMock{
			balance: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(100), nil
			},
			totalPaidOut: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(0), nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// issue a cheque
	expectedCheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      beneficiary,
			CumulativePayout: amount,
			Chequebook:       address,
		},
		Signature: sig,
	}

	chequeSigner.sign = func(cheque *chequebook.Cheque) ([]byte, error) {
		if !cheque.Equal(&expectedCheque.Cheque) {
			t.Fatalf("wrong cheque. wanted %v got %v", expectedCheque.Cheque, cheque)
		}
		return sig, nil
	}

	_, err = chequebookService.Issue(context.Background(), beneficiary, amount, func(cheque *chequebook.SignedCheque) error {
		if !cheque.Equal(expectedCheque) {
			t.Fatalf("wrong cheque. wanted %v got %v", expectedCheque, cheque)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	lastCheque, err := chequebookService.LastCheque(beneficiary)
	if err != nil {
		t.Fatal(err)
	}

	if !lastCheque.Equal(expectedCheque) {
		t.Fatalf("wrong cheque stored. wanted %v got %v", expectedCheque, lastCheque)
	}

	// issue another cheque for the same beneficiary
	expectedCheque = &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      beneficiary,
			CumulativePayout: expectedCumulative,
			Chequebook:       address,
		},
		Signature: sig,
	}

	chequeSigner.sign = func(cheque *chequebook.Cheque) ([]byte, error) {
		if !cheque.Equal(&expectedCheque.Cheque) {
			t.Fatalf("wrong cheque. wanted %v got %v", expectedCheque, cheque)
		}
		return sig, nil
	}

	_, err = chequebookService.Issue(context.Background(), beneficiary, amount2, func(cheque *chequebook.SignedCheque) error {
		if !cheque.Equal(expectedCheque) {
			t.Fatalf("wrong cheque. wanted %v got %v", expectedCheque, cheque)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	lastCheque, err = chequebookService.LastCheque(beneficiary)
	if err != nil {
		t.Fatal(err)
	}

	if !lastCheque.Equal(expectedCheque) {
		t.Fatalf("wrong cheque stored. wanted %v got %v", expectedCheque, lastCheque)
	}

	// issue another cheque for the different beneficiary
	expectedChequeOwner := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      ownerAdress,
			CumulativePayout: amount,
			Chequebook:       address,
		},
		Signature: sig,
	}

	chequeSigner.sign = func(cheque *chequebook.Cheque) ([]byte, error) {
		if !cheque.Equal(&expectedChequeOwner.Cheque) {
			t.Fatalf("wrong cheque. wanted %v got %v", expectedCheque, cheque)
		}
		return sig, nil
	}

	_, err = chequebookService.Issue(context.Background(), ownerAdress, amount, func(cheque *chequebook.SignedCheque) error {
		if !cheque.Equal(expectedChequeOwner) {
			t.Fatalf("wrong cheque. wanted %v got %v", expectedChequeOwner, cheque)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	lastCheque, err = chequebookService.LastCheque(ownerAdress)
	if err != nil {
		t.Fatal(err)
	}

	if !lastCheque.Equal(expectedChequeOwner) {
		t.Fatalf("wrong cheque stored. wanted %v got %v", expectedChequeOwner, lastCheque)
	}

	// finally check this did not interfere with the beneficiary cheque
	lastCheque, err = chequebookService.LastCheque(beneficiary)
	if err != nil {
		t.Fatal(err)
	}

	if !lastCheque.Equal(expectedCheque) {
		t.Fatalf("wrong cheque stored. wanted %v got %v", expectedCheque, lastCheque)
	}
}

func TestChequebookIssueErrorSend(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	beneficiary := common.HexToAddress("0xdddd")
	ownerAdress := common.HexToAddress("0xfff")
	store := storemock.NewStateStore()
	amount := big.NewInt(20)
	sig := common.Hex2Bytes("0xffff")
	chequeSigner := &chequeSignerMock{}

	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(),
		address,
		ownerAdress,
		store,
		chequeSigner,
		erc20mock.New(),
		&simpleSwapBindingMock{
			balance: func(*bind.CallOpts) (*big.Int, error) {
				return amount, nil
			},
			totalPaidOut: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(0), nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	chequeSigner.sign = func(cheque *chequebook.Cheque) ([]byte, error) {
		return sig, nil
	}

	_, err = chequebookService.Issue(context.Background(), beneficiary, amount, func(cheque *chequebook.SignedCheque) error {
		return errors.New("err")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// verify the cheque was not saved
	_, err = chequebookService.LastCheque(beneficiary)
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrNoCheque, err)
	}
}

func TestChequebookIssueOutOfFunds(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	beneficiary := common.HexToAddress("0xdddd")
	ownerAdress := common.HexToAddress("0xfff")
	store := storemock.NewStateStore()
	amount := big.NewInt(20)

	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
		&simpleSwapBindingMock{
			balance: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(0), nil
			},
			totalPaidOut: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(0), nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = chequebookService.Issue(context.Background(), beneficiary, amount, func(cheque *chequebook.SignedCheque) error {
		return nil
	})
	if !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrOutOfFunds, err)
	}

	// verify the cheque was not saved
	_, err = chequebookService.LastCheque(beneficiary)

	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("wrong error. wanted %v, got %v", chequebook.ErrNoCheque, err)
	}
}

func TestChequebookWithdraw(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	balance := big.NewInt(30)
	withdrawAmount := big.NewInt(20)
	txHash := common.HexToHash("0xdddd")
	store := storemock.NewStateStore()
	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithSendFunc(func(c context.Context, request *transaction.TxRequest) (common.Hash, error) {
				if request.To != nil && *request.To != address {
					t.Fatalf("sending to wrong contract. wanted %x, got %x", address, request.To)
				}
				if request.Value.Cmp(big.NewInt(0)) != 0 {
					t.Fatal("sending ether to token contract")
				}
				return txHash, nil
			}),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
		&simpleSwapBindingMock{
			balance: func(*bind.CallOpts) (*big.Int, error) {
				return balance, nil
			},
			totalPaidOut: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(0), nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	returnedTxHash, err := chequebookService.Withdraw(context.Background(), withdrawAmount)
	if err != nil {
		t.Fatal(err)
	}

	if txHash != returnedTxHash {
		t.Fatalf("returned wrong transaction hash. wanted %v, got %v", txHash, returnedTxHash)
	}
}

func TestChequebookWithdrawInsufficientFunds(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	balance := big.NewInt(30)
	withdrawAmount := big.NewInt(20)
	txHash := common.HexToHash("0xdddd")
	store := storemock.NewStateStore()
	chequebookService, err := newTestChequebook(
		t,
		backendmock.New(),
		transactionmock.New(
			transactionmock.WithSendFunc(func(c context.Context, request *transaction.TxRequest) (common.Hash, error) {
				if request.To != nil && *request.To != address {
					t.Fatalf("sending to wrong contract. wanted %x, got %x", address, request.To)
				}
				if request.Value.Cmp(big.NewInt(0)) != 0 {
					t.Fatal("sending ether to token contract")
				}
				return txHash, nil
			}),
			transactionmock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
				return balance.FillBytes(make([]byte, 32)), nil
			}),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
		&simpleSwapBindingMock{
			balance: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(0), nil
			},
			totalPaidOut: func(*bind.CallOpts) (*big.Int, error) {
				return big.NewInt(0), nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = chequebookService.Withdraw(context.Background(), withdrawAmount)
	if !errors.Is(err, chequebook.ErrInsufficientFunds) {
		t.Fatalf("got wrong error. wanted %v, got %v", chequebook.ErrInsufficientFunds, err)
	}
}
