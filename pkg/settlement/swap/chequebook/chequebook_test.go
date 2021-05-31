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
	erc20mock "github.com/ethersphere/bee/pkg/settlement/swap/erc20/mock"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/transaction"
	transactionmock "github.com/ethersphere/bee/pkg/transaction/mock"
)

func TestChequebookAddress(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	chequebookService, err := chequebook.New(
		transactionmock.New(),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(),
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
	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICall(&chequebookABI, address, balance.FillBytes(make([]byte, 32)), "balance"),
		),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(),
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
	chequebookService, err := chequebook.New(
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
	chequebookService, err := chequebook.New(
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
	chequebookService, err := chequebook.New(
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

	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(100).FillBytes(make([]byte, 32)), "balance"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(100).FillBytes(make([]byte, 32)), "balance"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(100).FillBytes(make([]byte, 32)), "balance"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
			),
		),
		address,
		ownerAdress,
		store,
		chequeSigner,
		erc20mock.New(),
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

	chequebookService, err := chequebook.New(
		transactionmock.New(),
		address,
		ownerAdress,
		store,
		chequeSigner,
		erc20mock.New(),
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

	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "balance"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
			),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
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
	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, balance.FillBytes(make([]byte, 32)), "balance"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
			),
			transactionmock.WithABISend(&chequebookABI, txHash, address, big.NewInt(0), "withdraw", withdrawAmount),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
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
	withdrawAmount := big.NewInt(20)
	txHash := common.HexToHash("0xdddd")
	store := storemock.NewStateStore()
	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABISend(&chequebookABI, txHash, address, big.NewInt(0), "withdraw", withdrawAmount),
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "balance"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
			),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = chequebookService.Withdraw(context.Background(), withdrawAmount)
	if !errors.Is(err, chequebook.ErrInsufficientFunds) {
		t.Fatalf("got wrong error. wanted %v, got %v", chequebook.ErrInsufficientFunds, err)
	}
}

func TestStoreCheque(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	store := storemock.NewStateStore()
	chequebookService, _ := chequebook.New(
		nil,
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
	)

	ten := new(big.Int).SetInt64(10)
	cheque := new(chequebook.SignedCheque)
	cheque.CumulativePayout = new(big.Int).SetInt64(100)

	store.Put("swap_chequebook_total_issued_", new(big.Int).SetInt64(99))

	_ = chequebookService.StoreCheque(address, cheque, ten)

	var totalIssued *big.Int
	var expected = new(big.Int).SetInt64(109)
	if store.Get("swap_chequebook_total_issued_", &totalIssued); totalIssued.Cmp(expected) != 0 {
		t.Errorf("expected %d, got %d", ten, totalIssued)
	}

	var gotCheque *chequebook.SignedCheque
	chequeKey := "swap_chequebook_last_issued_cheque_000000000000000000000000000000000000abcd"
	store.Get(chequeKey, &gotCheque)
	if gotCheque.CumulativePayout.Cmp(cheque.CumulativePayout) != 0 {
		t.Errorf("bad payout value, want %d, got %d", cheque.CumulativePayout, gotCheque.CumulativePayout)
	}
}
