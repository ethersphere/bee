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
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = chequebookService.Withdraw(context.Background(), withdrawAmount)
	if !errors.Is(err, chequebook.ErrInsufficientFunds) {
		t.Fatalf("got wrong error. wanted %v, got %v", chequebook.ErrInsufficientFunds, err)
	}
}

func TestChequebookCoveringBalanceCache(t *testing.T) {
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	rpcErr := errors.New("rpc failed")

	totalPaidOutCallData, err := chequebookABI.Pack("totalPaidOut")
	if err != nil {
		t.Fatal(err)
	}
	balanceCallData, err := chequebookABI.Pack("balance")
	if err != nil {
		t.Fatal(err)
	}

	var calls int
	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) ([]byte, error) {
				calls++
				switch calls {
				case 1:
					if !bytes.Equal(request.Data, totalPaidOutCallData) {
						t.Fatalf("unexpected call data on call %d", calls)
					}
					return big.NewInt(0).FillBytes(make([]byte, 32)), nil
				case 2:
					if !bytes.Equal(request.Data, balanceCallData) {
						t.Fatalf("unexpected call data on call %d", calls)
					}
					return nil, rpcErr
				case 3, 5:
					if !bytes.Equal(request.Data, totalPaidOutCallData) {
						t.Fatalf("unexpected call data on call %d", calls)
					}
					return big.NewInt(0).FillBytes(make([]byte, 32)), nil
				case 4, 6:
					if !bytes.Equal(request.Data, balanceCallData) {
						t.Fatalf("unexpected call data on call %d", calls)
					}
					return big.NewInt(100).FillBytes(make([]byte, 32)), nil
				default:
					t.Fatalf("unexpected rpc call %d", calls)
					return nil, errors.New("unexpected call")
				}
			}),
		),
		address,
		common.HexToAddress("0xfff"),
		storemock.NewStateStore(),
		&chequeSignerMock{},
		erc20mock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = chequebookService.AvailableBalance(context.Background())
	if !errors.Is(err, rpcErr) {
		t.Fatalf("wrong error. wanted %v, got %v", rpcErr, err)
	}
	if calls != 2 {
		t.Fatalf("unexpected rpc call count after failed load: got %d, want 2", calls)
	}

	available, err := chequebookService.AvailableBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if available.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("unexpected available balance: got %d, want 100", available)
	}
	if calls != 4 {
		t.Fatalf("unexpected rpc call count after successful load: got %d, want 4", calls)
	}

	chequebook.SetCoveringBalanceCachedAt(chequebookService, time.Now().Add(-3*time.Minute))
	available, err = chequebookService.AvailableBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if available.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("unexpected available balance: got %d, want 100", available)
	}
	if calls != 4 {
		t.Fatalf("unexpected rpc call count while cache is valid: got %d, want 4", calls)
	}

	chequebook.SetCoveringBalanceCachedAt(chequebookService, time.Now().Add(-6*time.Minute))
	available, err = chequebookService.AvailableBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if available.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("unexpected available balance: got %d, want 100", available)
	}
	if calls != 6 {
		t.Fatalf("unexpected rpc call count after cache expiry: got %d, want 6", calls)
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
	)
	if err != nil {
		t.Fatal(err)
	}
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

func TestChequebookIssueAfterCoveringBalanceChange(t *testing.T) {
	t.Parallel()

	beneficiary := common.HexToAddress("0xdddd")

	t.Run("deposit mined then issue", func(t *testing.T) {
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

		// empty chequebook, cache loaded
		expectAvailableBalance(t, chequebookService, 0)

		if _, err := chequebookService.Deposit(context.Background(), big.NewInt(100)); err != nil {
			t.Fatal(err)
		}
		if err := issueCheque(chequebookService, beneficiary, 10); !errors.Is(err, chequebook.ErrOutOfFunds) {
			t.Fatalf("issue before the deposit is mined: got %v, want %v", err, chequebook.ErrOutOfFunds)
		}

		chain.setBalance(100)
		total, err := chequebookService.Balance(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if total.Cmp(big.NewInt(100)) != 0 {
			t.Fatalf("on-chain balance: got %d, want 100", total)
		}
		if err := issueCheque(chequebookService, beneficiary, 10); err != nil {
			t.Fatalf("issue after the deposit is mined: %v", err)
		}
		expectAvailableBalance(t, chequebookService, 90)
	})

	t.Run("external top-up", func(t *testing.T) {
		t.Parallel()

		chain := newChainMock(t, 0)
		chequebookService := newChainChequebook(t, chain, erc20mock.New())
		expectAvailableBalance(t, chequebookService, 0)

		// tokens arrive without Deposit or WaitForDeposit
		chain.setBalance(100)
		if err := issueCheque(chequebookService, beneficiary, 10); err != nil {
			t.Fatalf("issue after an external top-up: %v", err)
		}
		expectAvailableBalance(t, chequebookService, 90)
	})

	t.Run("pending withdrawal survives deposit", func(t *testing.T) {
		t.Parallel()

		withdrawHash := common.HexToHash("0x1")
		receiptC := make(chan types.Receipt, 1)
		errC := make(chan error, 1)
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
				return receiptC, errC, nil
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
		if err := issueCheque(chequebookService, beneficiary, 50); !errors.Is(err, chequebook.ErrOutOfFunds) {
			t.Fatalf("issue while withdrawal and deposit are pending: got %v, want %v", err, chequebook.ErrOutOfFunds)
		}

		// both transactions mined: 100 - 90 + 1, and only the first cheque was issued
		chain.setBalance(11)
		receiptC <- types.Receipt{TxHash: withdrawHash, Status: types.ReceiptStatusSuccessful}
		waitForPendingWithdrawals(t, chequebookService, 0)
		expectAvailableBalance(t, chequebookService, 1)
	})
}

func TestChequebookIssueAfterDepositMined(t *testing.T) {
	t.Parallel()

	beneficiary := common.HexToAddress("0xdddd")
	chain := newChainMock(t, 0)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(
		erc20mock.WithBalanceOfFunc(func(context.Context, common.Address) (*big.Int, error) {
			return big.NewInt(1000), nil
		}),
		erc20mock.WithTransferFunc(func(context.Context, common.Address, *big.Int) (common.Hash, error) {
			return common.HexToHash("0x1"), nil
		}),
	))

	// the cache holds the empty chequebook when the deposit is broadcast
	expectAvailableBalance(t, chequebookService, 0)

	if _, err := chequebookService.Deposit(context.Background(), big.NewInt(100)); err != nil {
		t.Fatal(err)
	}
	if err := issueCheque(chequebookService, beneficiary, 10); !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("issue before deposit is mined: got %v, want %v", err, chequebook.ErrOutOfFunds)
	}

	chain.setBalance(100)

	if err := issueCheque(chequebookService, beneficiary, 10); err != nil {
		t.Fatalf("issue after deposit is mined: %v", err)
	}
	expectAvailableBalance(t, chequebookService, 90)
}

func TestChequebookIssueOutOfFundsReloadsOnce(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 0)
	chequebookService := newChainChequebook(t, chain, erc20mock.New())

	expectAvailableBalance(t, chequebookService, 0)
	if got := chain.callCount(); got != 2 {
		t.Fatalf("rpc calls after cache load: got %d, want 2", got)
	}

	err := issueCheque(chequebookService, common.HexToAddress("0xdddd"), 10)
	if !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("got %v, want %v", err, chequebook.ErrOutOfFunds)
	}
	if got := chain.callCount(); got != 4 {
		t.Fatalf("rpc calls after refused issue: got %d, want 4", got)
	}
}

func TestChequebookPendingWithdrawal(t *testing.T) {
	t.Parallel()

	beneficiary := common.HexToAddress("0xdddd")
	txHash := common.HexToHash("0x1")
	receiptC := make(chan types.Receipt, 1)
	errC := make(chan error, 1)

	chain := newChainMock(t, 100)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, txHash, common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(50)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return receiptC, errC, nil
		}),
	)

	if err := issueCheque(chequebookService, beneficiary, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := chequebookService.Withdraw(context.Background(), big.NewInt(50)); err != nil {
		t.Fatal(err)
	}

	// reload from chain while the withdrawal is still pending
	chequebook.SetCoveringBalanceCachedAt(chequebookService, time.Now().Add(-6*time.Minute))

	if err := issueCheque(chequebookService, beneficiary, 50); !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("issue during pending withdrawal: got %v, want %v", err, chequebook.ErrOutOfFunds)
	}
	expectAvailableBalance(t, chequebookService, 40)

	chain.setBalance(50)
	receiptC <- types.Receipt{TxHash: txHash, Status: types.ReceiptStatusSuccessful}
	waitForPendingWithdrawals(t, chequebookService, 0)

	calls := chain.callCount()
	expectAvailableBalance(t, chequebookService, 40)
	if chain.callCount() == calls {
		t.Fatal("covering balance was not reloaded after the withdrawal was mined")
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

func TestChequebookWithdrawWatchError(t *testing.T) {
	t.Parallel()

	var watches atomic.Int32
	chain := newChainMock(t, 100)
	chequebookService := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(50)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			watches.Add(1)
			return nil, nil, transaction.ErrUnknownTransaction
		}),
	)

	txHash, err := chequebookService.Withdraw(context.Background(), big.NewInt(50))
	if !errors.Is(err, transaction.ErrUnknownTransaction) {
		t.Fatalf("got %v, want %v", err, transaction.ErrUnknownTransaction)
	}
	if txHash != common.HexToHash("0x1") {
		t.Fatalf("returned hash %x", txHash)
	}
	if watches.Load() != 1 {
		t.Fatalf("watch attempts: got %d, want 1", watches.Load())
	}
	if chequebook.PendingWithdrawals(chequebookService) != 1 {
		t.Fatalf("pending withdrawals: got %d, want 1", chequebook.PendingWithdrawals(chequebookService))
	}
	expectAvailableBalance(t, chequebookService, 50)
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

func TestChequebookConcurrentIssueAndWithdraw(t *testing.T) {
	t.Parallel()

	const (
		balance = 100
		amount  = 10
		workers = 10
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
		wg     sync.WaitGroup
		spent  atomic.Int64
		errsMu sync.Mutex
		errs   []error
	)
	record := func(err error, allowed error) {
		switch {
		case err == nil:
			spent.Add(amount)
		case !errors.Is(err, allowed):
			errsMu.Lock()
			errs = append(errs, err)
			errsMu.Unlock()
		}
	}
	for i := range workers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			record(issueCheque(chequebookService, common.BigToAddress(big.NewInt(int64(i+1))), amount), chequebook.ErrOutOfFunds)
		}()
		go func() {
			defer wg.Done()
			_, err := chequebookService.Withdraw(context.Background(), big.NewInt(amount))
			record(err, chequebook.ErrInsufficientFunds)
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := spent.Load(); got > balance {
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
