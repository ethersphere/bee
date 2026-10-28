// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

func TestChequebookAddress(t *testing.T) {
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	chequebookService, err := chequebook.New(
		transactionmock.New(),
		address,
		ownerAdress,
		nil,
		&chequeSignerMock{},
		erc20mock.New(),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	if chequebookService.Address() != address {
		t.Fatalf("returned wrong address. wanted %x, got %x", address, chequebookService.Address())
	}
}

func TestChequebookBalance(t *testing.T) {
	t.Parallel()

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
		0,
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
	t.Parallel()

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
		0,
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
	t.Parallel()

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
		0,
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
	t.Parallel()

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
		0,
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
	t.Parallel()

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
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(100).FillBytes(make([]byte, 32)), "balance"),
			),
		),
		address,
		ownerAdress,
		store,
		chequeSigner,
		erc20mock.New(),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	// issue a cheque
	expectedCheque := &chequebook.SignedCheque{
		Beneficiary:      beneficiary,
		CumulativePayout: amount,
		Chequebook:       address,
		Signature:        sig,
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
		Beneficiary:      beneficiary,
		CumulativePayout: expectedCumulative,
		Chequebook:       address,
		Signature:        sig,
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
		Beneficiary:      ownerAdress,
		CumulativePayout: amount,
		Chequebook:       address,
		Signature:        sig,
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
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	beneficiary := common.HexToAddress("0xdddd")
	ownerAdress := common.HexToAddress("0xfff")
	store := storemock.NewStateStore()
	amount := big.NewInt(20)
	sig := common.Hex2Bytes("0xffff")
	chequeSigner := &chequeSignerMock{}

	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(100).FillBytes(make([]byte, 32)), "balance"),
			),
		),
		address,
		ownerAdress,
		store,
		chequeSigner,
		erc20mock.New(),
		0,
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
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	beneficiary := common.HexToAddress("0xdddd")
	ownerAdress := common.HexToAddress("0xfff")
	store := storemock.NewStateStore()
	amount := big.NewInt(20)

	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "balance"),
			),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
		0,
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
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	balance := big.NewInt(30)
	withdrawAmount := big.NewInt(20)
	txHash := common.HexToHash("0xdddd")
	store := storemock.NewStateStore()
	errC := make(chan error, 1)
	t.Cleanup(func() { errC <- transaction.ErrMonitorClosed })
	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
				transactionmock.ABICall(&chequebookABI, address, balance.FillBytes(make([]byte, 32)), "balance"),
			),
			transactionmock.WithABISend(&chequebookABI, txHash, address, big.NewInt(0), "withdraw", withdrawAmount),
			transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
				return make(chan types.Receipt), errC, nil
			}),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
		0,
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
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	ownerAdress := common.HexToAddress("0xfff")
	withdrawAmount := big.NewInt(20)
	txHash := common.HexToHash("0xdddd")
	store := storemock.NewStateStore()
	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABISend(&chequebookABI, txHash, address, big.NewInt(0), "withdraw", withdrawAmount),
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "balance"),
			),
		),
		address,
		ownerAdress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = chequebookService.Withdraw(context.Background(), withdrawAmount)
	if !errors.Is(err, chequebook.ErrInsufficientFunds) {
		t.Fatalf("got wrong error. wanted %v, got %v", chequebook.ErrInsufficientFunds, err)
	}
}

// chainMock answers balance and totalPaidOut calls with values a test can change,
// to simulate transactions being mined.
type chainMock struct {
	mu                   sync.Mutex
	balance              *big.Int
	calls                int
	balanceCallData      []byte
	totalPaidOutCallData []byte
}

func newChainMock(t *testing.T, balance int64) *chainMock {
	t.Helper()

	balanceCallData, err := chequebookABI.Pack("balance")
	if err != nil {
		t.Fatal(err)
	}
	totalPaidOutCallData, err := chequebookABI.Pack("totalPaidOut")
	if err != nil {
		t.Fatal(err)
	}
	return &chainMock{
		balance:              big.NewInt(balance),
		balanceCallData:      balanceCallData,
		totalPaidOutCallData: totalPaidOutCallData,
	}
}

func (c *chainMock) call(_ context.Context, request *transaction.TxRequest) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls++
	switch {
	case bytes.Equal(request.Data, c.balanceCallData):
		return new(big.Int).Set(c.balance).FillBytes(make([]byte, 32)), nil
	case bytes.Equal(request.Data, c.totalPaidOutCallData):
		return make([]byte, 32), nil
	default:
		return nil, errors.New("unexpected call")
	}
}

func (c *chainMock) setBalance(balance int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.balance = big.NewInt(balance)
}

func (c *chainMock) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func newChainChequebook(t *testing.T, chain *chainMock, erc20Service erc20.Service, opts ...transactionmock.Option) chequebook.Service {
	t.Helper()

	opts = append(opts, transactionmock.WithCallFunc(chain.call))
	chequebookService, err := chequebook.New(
		transactionmock.New(opts...),
		common.HexToAddress("0xabcd"),
		common.HexToAddress("0xfff"),
		storemock.NewStateStore(),
		&chequeSignerMock{sign: func(*chequebook.Cheque) ([]byte, error) { return nil, nil }},
		erc20Service,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	chequebook.SetWithdrawalReceiptHold(chequebookService, 0)
	return chequebookService
}

func issueCheque(s chequebook.Service, beneficiary common.Address, amount int64) error {
	_, err := s.Issue(context.Background(), beneficiary, big.NewInt(amount), func(*chequebook.SignedCheque) error {
		return nil
	})
	return err
}

func expectAvailableBalance(t *testing.T, s chequebook.Service, want int64) {
	t.Helper()

	available, err := s.AvailableBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if available.Cmp(big.NewInt(want)) != 0 {
		t.Fatalf("unexpected available balance: got %d, want %d", available, want)
	}
}

func waitForPendingWithdrawals(t *testing.T, s chequebook.Service, want int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for chequebook.PendingWithdrawals(s) != want {
		if time.Now().After(deadline) {
			t.Fatalf("pending withdrawals: got %d, want %d", chequebook.PendingWithdrawals(s), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestChequebookWithdrawalCancelled(t *testing.T) {
	t.Parallel()

	errC := make(chan error, 1)
	chain := newChainMock(t, 100)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(50)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return make(chan types.Receipt), errC, nil
		}),
	)

	if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(50)); err != nil {
		t.Fatal(err)
	}
	expectAvailableBalance(t, chequebookService, 50)

	errC <- transaction.ErrTransactionCancelled
	waitForPendingWithdrawals(t, chequebookService, 0)
	expectAvailableBalance(t, chequebookService, 100)
}

func TestChequebookWithdrawSendError(t *testing.T) {
	t.Parallel()

	sendErr := errors.New("send failed")
	chain := newChainMock(t, 100)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			return common.Hash{}, sendErr
		}),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			t.Fatal("unexpected watch of an unsent withdrawal")
			return nil, nil, nil
		}),
	)

	if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(50)); !errors.Is(err, sendErr) {
		t.Fatalf("got %v, want %v", err, sendErr)
	}
	expectAvailableBalance(t, chequebookService, 100)
}

func TestChequebookPendingWithdrawalSurvivesReload(t *testing.T) {
	t.Parallel()

	beneficiary := common.HexToAddress("0xdddd")
	withdrawHash := common.HexToHash("0x1")
	receiptC := make(chan types.Receipt, 1)
	chain := newChainMock(t, 100)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(
		erc20mock.WithBalanceOfFunc(func(context.Context, common.Address) (*big.Int, error) {
			return big.NewInt(1000), nil
		}),
		erc20mock.WithTransferFunc(func(context.Context, common.Address, *big.Int) (common.Hash, error) {
			return common.HexToHash("0x2"), nil
		}),
	),
		transactionmock.WithABISend(&chequebookABI, withdrawHash, common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(90)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return receiptC, make(chan error), nil
		}),
	)

	if err := issueCheque(chequebookService, beneficiary, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(90)); err != nil {
		t.Fatal(err)
	}
	if _, err := chequebookService.Deposit(context.Background(), big.NewInt(1)); err != nil {
		t.Fatal(err)
	}
	// the cheque does not fit, so the cache is reloaded from a chain that does not show the withdrawal yet
	before := chain.callCount()
	if err := issueCheque(chequebookService, beneficiary, 50); !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("issue while withdrawal and deposit are pending: got %v, want %v", err, chequebook.ErrOutOfFunds)
	}
	if got := chain.callCount() - before; got != 2 {
		t.Fatalf("chain reads for the refused cheque: got %d, want 2", got)
	}

	// both transactions mined: 100 - 90 + 1, and only the first cheque was issued
	chain.setBalance(11)
	receiptC <- types.Receipt{TxHash: withdrawHash, Status: types.ReceiptStatusSuccessful}
	waitForPendingWithdrawals(t, chequebookService, 0)
	expectAvailableBalance(t, chequebookService, 1)
}

func TestChequebookAvailableBalanceAfterDepositMined(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 0)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(
		erc20mock.WithBalanceOfFunc(func(context.Context, common.Address) (*big.Int, error) {
			return big.NewInt(1000), nil
		}),
		erc20mock.WithTransferFunc(func(context.Context, common.Address, *big.Int) (common.Hash, error) {
			return common.HexToHash("0x1"), nil
		}),
	))

	// the refused cheque leaves the empty chequebook in the cache
	if err := issueCheque(chequebookService, common.HexToAddress("0xdddd"), 10); !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("got %v, want %v", err, chequebook.ErrOutOfFunds)
	}
	if _, err := chequebookService.Deposit(context.Background(), big.NewInt(100)); err != nil {
		t.Fatal(err)
	}
	chain.setBalance(100)

	expectAvailableBalance(t, chequebookService, 100)
}

func TestChequebookWithdrawAfterExternalTopUp(t *testing.T) {
	t.Parallel()

	errC := make(chan error, 1)
	t.Cleanup(func() { errC <- transaction.ErrMonitorClosed })
	chain := newChainMock(t, 100)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(120)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return make(chan types.Receipt), errC, nil
		}),
	)
	expectAvailableBalance(t, chequebookService, 100)

	// tokens arrive without Deposit, the cache still holds 100
	chain.setBalance(150)
	before := chain.callCount()
	if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(120)); err != nil {
		t.Fatalf("withdraw after an external top-up: %v", err)
	}
	if got := chain.callCount() - before; got != 2 {
		t.Fatalf("chain reads for the withdrawal: got %d, want 2", got)
	}
	expectAvailableBalance(t, chequebookService, 30)
}

func TestChequebookWithdrawalReleaseDuringIssue(t *testing.T) {
	t.Parallel()

	const (
		balance  = 100
		withdraw = 50
		amount   = 5
		workers  = (balance - withdraw) / amount
	)
	receiptC := make(chan types.Receipt, 1)
	chain := newChainMock(t, balance)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(withdraw)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return receiptC, make(chan error), nil
		}),
	)
	expectAvailableBalance(t, chequebookService, balance)
	if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(withdraw)); err != nil {
		t.Fatal(err)
	}

	var (
		wg     sync.WaitGroup
		issued atomic.Int64
	)
	start := make(chan struct{})
	for i := range workers {
		wg.Go(func() {
			<-start
			err := issueCheque(chequebookService, common.BigToAddress(big.NewInt(int64(i+1))), amount)
			switch {
			case err == nil:
				issued.Add(amount)
			case !errors.Is(err, chequebook.ErrOutOfFunds):
				t.Error(err)
			}
		})
	}
	chain.setBalance(balance - withdraw)
	close(start)
	receiptC <- types.Receipt{Status: types.ReceiptStatusSuccessful}
	wg.Wait()
	waitForPendingWithdrawals(t, chequebookService, 0)

	// what was refused while the release raced the cheques is still available
	for range balance / amount {
		if issueCheque(chequebookService, common.HexToAddress("0xdddd"), amount) != nil {
			break
		}
		issued.Add(amount)
	}
	if got := issued.Load(); got != balance-withdraw {
		t.Fatalf("issued %d, want %d", got, balance-withdraw)
	}
}

func TestChequebookWithdrawalReceiptIgnoresLaggingBalance(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const hold = 25 * time.Second

		beneficiary := common.HexToAddress("0xdddd")
		receiptC := make(chan types.Receipt, 1)
		chain := newChainMock(t, 100)
		chequebookService := newChainChequebook(t, chain, erc20mock.New(),
			transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(90)),
			transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
				return receiptC, make(chan error), nil
			}),
		)
		chequebook.SetWithdrawalReceiptHold(chequebookService, hold)

		expectAvailableBalance(t, chequebookService, 100)
		if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(90)); err != nil {
			t.Fatal(err)
		}

		// the receipt is in, but the chain still shows the pre-withdrawal balance
		receiptC <- types.Receipt{Status: types.ReceiptStatusSuccessful}
		synctest.Wait()
		if got := chequebook.PendingWithdrawals(chequebookService); got != 1 {
			t.Fatalf("pending withdrawals during the hold: got %d, want 1", got)
		}
		if err := issueCheque(chequebookService, beneficiary, 11); !errors.Is(err, chequebook.ErrOutOfFunds) {
			t.Fatalf("issued against a lagging balance: %v", err)
		}
		if err := issueCheque(chequebookService, beneficiary, 10); err != nil {
			t.Fatal(err)
		}

		chain.setBalance(10)
		time.Sleep(hold)
		synctest.Wait()
		if got := chequebook.PendingWithdrawals(chequebookService); got != 0 {
			t.Fatalf("pending withdrawals after the hold: got %d, want 0", got)
		}
		expectAvailableBalance(t, chequebookService, 0)
	})
}

func TestChequebookWithdrawalWatchFailureKeepsDeduction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		watchErr   error
		monitorErr error
	}{
		{name: "monitor closed", monitorErr: transaction.ErrMonitorClosed},
		{name: "watch failed", watchErr: transaction.ErrUnknownTransaction},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				errC := make(chan error, 1)
				chain := newChainMock(t, 100)
				chequebookService := newChainChequebook(t, chain, erc20mock.New(),
					transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(40)),
					transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
						if tc.watchErr != nil {
							return nil, nil, tc.watchErr
						}
						return make(chan types.Receipt), errC, nil
					}),
				)

				if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(40)); err != nil {
					t.Fatal(err)
				}
				if tc.monitorErr != nil {
					errC <- tc.monitorErr
				}
				synctest.Wait()

				if got := chequebook.PendingWithdrawals(chequebookService); got != 1 {
					t.Fatalf("pending withdrawals: got %d, want 1", got)
				}
				expectAvailableBalance(t, chequebookService, 60)
			})
		})
	}
}

func TestChequebookWithdrawNegativeAmount(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 100)
	chequebookService := newChainChequebook(t, chain, erc20mock.New())

	if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(-1000)); err == nil {
		t.Fatal("negative withdrawal accepted")
	}
	expectAvailableBalance(t, chequebookService, 100)
}

func TestChequebookConcurrentIssueAndWithdraw(t *testing.T) {
	t.Parallel()

	const (
		balance     = 100
		amount      = 10
		issuers     = 10
		withdrawers = 5
	)

	var (
		mu       sync.Mutex
		sent     int64
		watchErr []chan error
	)
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, errC := range watchErr {
			errC <- transaction.ErrMonitorClosed
		}
	})

	chain := newChainMock(t, balance)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			mu.Lock()
			defer mu.Unlock()
			sent++
			return common.BigToHash(big.NewInt(sent)), nil
		}),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			mu.Lock()
			defer mu.Unlock()
			errC := make(chan error, 1)
			watchErr = append(watchErr, errC)
			return make(chan types.Receipt), errC, nil
		}),
	)

	var (
		wg    sync.WaitGroup
		spent atomic.Int64
	)
	record := func(err error, allowed error) {
		switch {
		case err == nil:
			spent.Add(amount)
		case !errors.Is(err, allowed):
			t.Error(err)
		}
	}
	for i := range issuers {
		wg.Go(func() {
			record(issueCheque(chequebookService, common.BigToAddress(big.NewInt(int64(i+1))), amount), chequebook.ErrOutOfFunds)
		})
	}
	for range withdrawers {
		wg.Go(func() {
			_, err := chequebookService.Withdraw(context.Background(), big.NewInt(amount))
			record(err, chequebook.ErrInsufficientFunds)
		})
	}
	wg.Wait()

	// what was refused concurrently is still available to cheques
	for range balance / amount {
		if issueCheque(chequebookService, common.HexToAddress("0xdddd"), amount) != nil {
			break
		}
		spent.Add(amount)
	}
	if got := spent.Load(); got != balance {
		t.Fatalf("issued and withdrawn %d against a balance of %d", got, balance)
	}
}

func TestStateStoreKeys(t *testing.T) {
	t.Parallel()

	address := common.HexToAddress("0xabcd")

	expected := "swap_cashout_000000000000000000000000000000000000abcd"
	if chequebook.CashoutActionKey(address) != expected {
		t.Fatalf("wrong cashout action key. wanted %s, got %s", expected, chequebook.CashoutActionKey(address))
	}

	expected = "swap_chequebook_last_issued_cheque_000000000000000000000000000000000000abcd"
	if chequebook.LastIssuedChequeKey(address) != expected {
		t.Fatalf("wrong last issued cheque key. wanted %s, got %s", expected, chequebook.LastIssuedChequeKey(address))
	}

	expected = "swap_chequebook_last_received_cheque__000000000000000000000000000000000000abcd"
	if chequebook.LastReceivedChequeKey(address) != expected {
		t.Fatalf("wrong last received cheque key. wanted %s, got %s", expected, chequebook.LastReceivedChequeKey(address))
	}
}
