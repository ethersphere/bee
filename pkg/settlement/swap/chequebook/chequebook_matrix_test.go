// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	storemock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

// matrixCase is one cell of the covering-balance matrix.
// Chain changes that happen after the cache is warm are invisible to a cheque
// that still fits the cached figure. A cheque that does not fit reloads first.
type matrixCase struct {
	name             string
	chain            int64
	warm             bool
	issued           int64
	pending          int64
	expire           bool
	deposit          bool
	addChain         int64
	applyAdd         bool
	minePending      bool
	amount           int64
	wantErr          bool
	wantAvailable    int64
	wantReads        int
	wantChainBalance int64
}

func TestChequebookCoveringBalanceMatrix(t *testing.T) {
	t.Parallel()

	beneficiary := common.HexToAddress("0xdddd")
	cases := []matrixCase{
		// empty, cache cold: the first issue reads the chain once
		{name: "empty/cold/issue-1", chain: 0, amount: 1, wantErr: true, wantAvailable: 0, wantReads: 2},
		{name: "empty/cold/deposit-mined-50/issue-1", chain: 0, applyAdd: true, addChain: 50, amount: 1, wantAvailable: 49, wantReads: 2, wantChainBalance: 50},
		{name: "empty/cold/deposit-mined-50/issue-50", chain: 0, applyAdd: true, addChain: 50, amount: 50, wantAvailable: 0, wantReads: 2, wantChainBalance: 50},
		{name: "empty/cold/deposit-mined-50/issue-51", chain: 0, applyAdd: true, addChain: 50, amount: 51, wantErr: true, wantAvailable: 50, wantReads: 2, wantChainBalance: 50},
		{name: "empty/cold/deposit-broadcast/issue-1", chain: 0, deposit: true, amount: 1, wantErr: true, wantAvailable: 0, wantReads: 2},

		// empty, cache warm at 0: a cheque reloads, still sees 0 until the chain changes
		{name: "empty/warm/issue-1", chain: 0, warm: true, amount: 1, wantErr: true, wantAvailable: 0, wantReads: 2},
		{name: "empty/warm/deposit-mined-50/issue-1", chain: 0, warm: true, applyAdd: true, addChain: 50, amount: 1, wantAvailable: 49, wantReads: 2, wantChainBalance: 50},
		{name: "empty/warm/deposit-mined-50/issue-50", chain: 0, warm: true, applyAdd: true, addChain: 50, amount: 50, wantAvailable: 0, wantReads: 2, wantChainBalance: 50},
		{name: "empty/warm/deposit-mined-50/issue-51", chain: 0, warm: true, applyAdd: true, addChain: 50, amount: 51, wantErr: true, wantAvailable: 50, wantReads: 2, wantChainBalance: 50},
		{name: "empty/warm/top-up-50/issue-1", chain: 0, warm: true, applyAdd: true, addChain: 50, amount: 1, wantAvailable: 49, wantReads: 2, wantChainBalance: 50},
		{name: "empty/warm/deposit-broadcast/issue-1", chain: 0, warm: true, deposit: true, amount: 1, wantErr: true, wantAvailable: 0, wantReads: 2},

		// 100, cache cold
		{name: "100/cold/issue-1", chain: 100, amount: 1, wantAvailable: 99, wantReads: 2, wantChainBalance: 100},
		{name: "100/cold/issue-100", chain: 100, amount: 100, wantAvailable: 0, wantReads: 2, wantChainBalance: 100},
		{name: "100/cold/issue-101", chain: 100, amount: 101, wantErr: true, wantAvailable: 100, wantReads: 2, wantChainBalance: 100},
		{name: "100/cold/deposit-mined-50/issue-150", chain: 100, applyAdd: true, addChain: 50, amount: 150, wantAvailable: 0, wantReads: 2, wantChainBalance: 150},
		{name: "100/cold/deposit-mined-50/issue-151", chain: 100, applyAdd: true, addChain: 50, amount: 151, wantErr: true, wantAvailable: 150, wantReads: 2, wantChainBalance: 150},

		// 100, cache warm: a fitting cheque does not read the chain
		{name: "100/warm/issue-1", chain: 100, warm: true, amount: 1, wantAvailable: 99, wantChainBalance: 100},
		{name: "100/warm/issue-100", chain: 100, warm: true, amount: 100, wantAvailable: 0, wantChainBalance: 100},
		{name: "100/warm/issue-101", chain: 100, warm: true, amount: 101, wantErr: true, wantAvailable: 100, wantReads: 2, wantChainBalance: 100},
		{name: "100/warm/deposit-mined-50/issue-1", chain: 100, warm: true, deposit: true, applyAdd: true, addChain: 50, amount: 1, wantAvailable: 99, wantChainBalance: 150},
		{name: "100/warm/deposit-mined-50/issue-100", chain: 100, warm: true, deposit: true, applyAdd: true, addChain: 50, amount: 100, wantAvailable: 0, wantChainBalance: 150},
		{name: "100/warm/deposit-mined-50/issue-101", chain: 100, warm: true, deposit: true, applyAdd: true, addChain: 50, amount: 101, wantAvailable: 49, wantReads: 2, wantChainBalance: 150},
		{name: "100/warm/deposit-mined-50/issue-151", chain: 100, warm: true, deposit: true, applyAdd: true, addChain: 50, amount: 151, wantErr: true, wantAvailable: 150, wantReads: 2, wantChainBalance: 150},
		{name: "100/warm/top-up-50/issue-101", chain: 100, warm: true, applyAdd: true, addChain: 50, amount: 101, wantAvailable: 49, wantReads: 2, wantChainBalance: 150},
		{name: "100/warm/deposit-broadcast/issue-100", chain: 100, warm: true, deposit: true, amount: 100, wantAvailable: 0, wantChainBalance: 100},
		{name: "100/warm/deposit-broadcast/issue-101", chain: 100, warm: true, deposit: true, amount: 101, wantErr: true, wantAvailable: 100, wantReads: 2, wantChainBalance: 100},

		// 100, 40 already issued, cache warm: 60 left in the cache
		{name: "100/warm/issued-40/issue-60", chain: 100, warm: true, issued: 40, amount: 60, wantAvailable: 0, wantChainBalance: 100},
		{name: "100/warm/issued-40/issue-61", chain: 100, warm: true, issued: 40, amount: 61, wantErr: true, wantAvailable: 60, wantReads: 2, wantChainBalance: 100},
		{name: "100/warm/issued-40/top-up-50/issue-60", chain: 100, warm: true, issued: 40, applyAdd: true, addChain: 50, amount: 60, wantAvailable: 0, wantChainBalance: 150},
		{name: "100/warm/issued-40/top-up-50/issue-61", chain: 100, warm: true, issued: 40, applyAdd: true, addChain: 50, amount: 61, wantAvailable: 49, wantReads: 2, wantChainBalance: 150},
		{name: "100/warm/issued-40/top-up-50/issue-111", chain: 100, warm: true, issued: 40, applyAdd: true, addChain: 50, amount: 111, wantErr: true, wantAvailable: 110, wantReads: 2, wantChainBalance: 150},

		// withdrawal of 40 in flight, chain still 100
		{name: "100/warm/pending-40/issue-60", chain: 100, warm: true, pending: 40, amount: 60, wantAvailable: 0, wantChainBalance: 100},
		{name: "100/warm/pending-40/issue-61", chain: 100, warm: true, pending: 40, amount: 61, wantErr: true, wantAvailable: 60, wantReads: 2, wantChainBalance: 100},
		{name: "100/warm/pending-40/deposit-broadcast/issue-60", chain: 100, warm: true, pending: 40, deposit: true, amount: 60, wantAvailable: 0, wantChainBalance: 100},
		{name: "100/warm/pending-40/top-up-50/issue-60", chain: 100, warm: true, pending: 40, applyAdd: true, addChain: 50, amount: 60, wantAvailable: 0, wantChainBalance: 150},
		{name: "100/warm/pending-40/top-up-50/issue-61", chain: 100, warm: true, pending: 40, applyAdd: true, addChain: 50, amount: 61, wantAvailable: 49, wantReads: 2, wantChainBalance: 150},
		{name: "100/warm/pending-40/mined/issue-60", chain: 100, warm: true, pending: 40, minePending: true, amount: 60, wantAvailable: 0, wantReads: 2, wantChainBalance: 60},
		{name: "100/warm/pending-40/mined/issue-61", chain: 100, warm: true, pending: 40, minePending: true, amount: 61, wantErr: true, wantAvailable: 60, wantReads: 2, wantChainBalance: 60},

		// 40 issued and 40 withdrawing
		{name: "100/warm/issued-40/pending-40/issue-20", chain: 100, warm: true, issued: 40, pending: 40, amount: 20, wantAvailable: 0, wantChainBalance: 100},
		{name: "100/warm/issued-40/pending-40/issue-21", chain: 100, warm: true, issued: 40, pending: 40, amount: 21, wantErr: true, wantAvailable: 20, wantReads: 2, wantChainBalance: 100},
		{name: "100/warm/issued-40/pending-40/deposit-and-top-up-50/issue-20", chain: 100, warm: true, issued: 40, pending: 40, deposit: true, applyAdd: true, addChain: 50, amount: 20, wantAvailable: 0, wantChainBalance: 150},
		{name: "100/warm/issued-40/pending-40/deposit-and-top-up-50/issue-21", chain: 100, warm: true, issued: 40, pending: 40, deposit: true, applyAdd: true, addChain: 50, amount: 21, wantAvailable: 49, wantReads: 2, wantChainBalance: 150},
		{name: "100/warm/issued-40/pending-40/mined/issue-20", chain: 100, warm: true, issued: 40, pending: 40, minePending: true, amount: 20, wantAvailable: 0, wantReads: 2, wantChainBalance: 60},
		{name: "100/warm/issued-40/pending-40/mined/issue-21", chain: 100, warm: true, issued: 40, pending: 40, minePending: true, amount: 21, wantErr: true, wantAvailable: 20, wantReads: 2, wantChainBalance: 60},

		// cache expired: one chain read, no second reload in the same cheque
		{name: "100/expired/issue-100", chain: 100, warm: true, expire: true, amount: 100, wantAvailable: 0, wantReads: 2, wantChainBalance: 100},
		{name: "100/expired/issue-101", chain: 100, warm: true, expire: true, amount: 101, wantErr: true, wantAvailable: 100, wantReads: 2, wantChainBalance: 100},
		{name: "100/expired/top-up-50/issue-150", chain: 100, warm: true, expire: true, applyAdd: true, addChain: 50, amount: 150, wantAvailable: 0, wantReads: 2, wantChainBalance: 150},
		{name: "100/expired/top-up-50/issue-151", chain: 100, warm: true, expire: true, applyAdd: true, addChain: 50, amount: 151, wantErr: true, wantAvailable: 150, wantReads: 2, wantChainBalance: 150},

		// cache says 100, chain has already dropped to 50, and nothing forces a reload while the cheque fits
		{name: "100/warm/chain-dropped-50/issue-50", chain: 100, warm: true, applyAdd: true, addChain: -50, amount: 50, wantAvailable: 50, wantChainBalance: 50},
		{name: "100/warm/chain-dropped-50/issue-60", chain: 100, warm: true, applyAdd: true, addChain: -50, amount: 60, wantAvailable: 40, wantChainBalance: 50},
		{name: "100/warm/chain-dropped-50/issue-101", chain: 100, warm: true, applyAdd: true, addChain: -50, amount: 101, wantErr: true, wantAvailable: 50, wantReads: 2, wantChainBalance: 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service, chain, receiptC, errC := openMatrixChequebook(t, tc.chain, tc.issued, tc.pending, tc.warm)
			if errC != nil {
				t.Cleanup(func() { errC <- transaction.ErrMonitorClosed })
			}
			if tc.expire {
				chequebook.SetCoveringBalanceCachedAt(service, time.Now().Add(-6*time.Minute))
			}
			if tc.deposit {
				if _, err := service.Deposit(context.Background(), big.NewInt(1)); err != nil {
					t.Fatal(err)
				}
			}
			if tc.minePending {
				chain.setBalance(tc.chain - tc.pending)
				receiptC <- types.Receipt{Status: types.ReceiptStatusSuccessful}
				waitForPendingWithdrawals(t, service, 0)
			}
			if tc.applyAdd {
				chain.setBalance(tc.chain + tc.addChain)
			}

			before := chain.callCount()
			err := issueCheque(service, beneficiary, tc.amount)
			if tc.wantErr {
				if !errors.Is(err, chequebook.ErrOutOfFunds) {
					t.Fatalf("got %v, want %v", err, chequebook.ErrOutOfFunds)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got := chain.callCount() - before; got != tc.wantReads {
				t.Fatalf("chain reads for the cheque: got %d, want %d", got, tc.wantReads)
			}
			expectAvailableBalance(t, service, tc.wantAvailable)
			if tc.wantChainBalance != 0 || tc.chain == 0 && tc.amount == 1 && tc.wantErr {
				onChain, err := service.Balance(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if onChain.Cmp(big.NewInt(tc.wantChainBalance)) != 0 {
					t.Fatalf("on-chain balance: got %d, want %d", onChain, tc.wantChainBalance)
				}
			}
		})
	}
}

func openMatrixChequebook(t *testing.T, chainBal, issued, pending int64, warm bool) (chequebook.Service, *chainMock, chan types.Receipt, chan error) {
	t.Helper()

	chain := newChainMock(t, chainBal)
	var (
		opts     []transactionmock.Option
		receiptC chan types.Receipt
		errC     chan error
	)
	erc20Opts := []erc20mock.Option{
		erc20mock.WithBalanceOfFunc(func(context.Context, common.Address) (*big.Int, error) {
			return big.NewInt(1000), nil
		}),
		erc20mock.WithTransferFunc(func(context.Context, common.Address, *big.Int) (common.Hash, error) {
			return common.HexToHash("0xde"), nil
		}),
	}
	if pending > 0 {
		receiptC = make(chan types.Receipt, 1)
		errC = make(chan error, 1)
		opts = append(opts,
			transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(pending)),
			transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
				return receiptC, errC, nil
			}),
		)
	}
	service := newChainChequebook(t, chain, erc20mock.New(erc20Opts...), opts...)
	if warm {
		expectAvailableBalance(t, service, chainBal)
	}
	if issued > 0 {
		if err := issueCheque(service, common.HexToAddress("0xdddd"), issued); err != nil {
			t.Fatal(err)
		}
	}
	if pending > 0 {
		if _, err := service.Withdraw(context.Background(), big.NewInt(pending)); err != nil {
			t.Fatal(err)
		}
	}
	return service, chain, receiptC, errC
}

func TestChequebookCashoutBetweenCoveringReads(t *testing.T) {
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	totalPaidOutCall, err := chequebookABI.Pack("totalPaidOut")
	if err != nil {
		t.Fatal(err)
	}
	balanceCall, err := chequebookABI.Pack("balance")
	if err != nil {
		t.Fatal(err)
	}

	var methods []string
	service, err := chequebook.New(
		transactionmock.New(transactionmock.WithCallFunc(func(_ context.Context, request *transaction.TxRequest) ([]byte, error) {
			switch {
			case bytes.Equal(request.Data, totalPaidOutCall):
				methods = append(methods, "totalPaidOut")
				return big.NewInt(0).FillBytes(make([]byte, 32)), nil
			case bytes.Equal(request.Data, balanceCall):
				methods = append(methods, "balance")
				// 30 was cashed after totalPaidOut was read and before balance was read
				return big.NewInt(70).FillBytes(make([]byte, 32)), nil
			default:
				return nil, errors.New("unexpected call")
			}
		})),
		address,
		common.HexToAddress("0xfff"),
		storemock.NewStateStore(),
		&chequeSignerMock{sign: func(*chequebook.Cheque) ([]byte, error) { return nil, nil }},
		erc20mock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}

	expectAvailableBalance(t, service, 70)
	if len(methods) != 2 || methods[0] != "totalPaidOut" || methods[1] != "balance" {
		t.Fatalf("read order: got %v, want [totalPaidOut balance]", methods)
	}
}

func TestChequebookRefreshFailureKeepsCache(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 100)
	rpcErr := errors.New("rpc failed")
	var fail bool
	service, err := chequebook.New(
		transactionmock.New(transactionmock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) ([]byte, error) {
			if fail {
				return nil, rpcErr
			}
			return chain.call(ctx, request)
		})),
		common.HexToAddress("0xabcd"),
		common.HexToAddress("0xfff"),
		storemock.NewStateStore(),
		&chequeSignerMock{sign: func(*chequebook.Cheque) ([]byte, error) { return nil, nil }},
		erc20mock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}

	expectAvailableBalance(t, service, 100)
	fail = true
	err = issueCheque(service, common.HexToAddress("0xdddd"), 101)
	if !errors.Is(err, rpcErr) {
		t.Fatalf("got %v, want %v", err, rpcErr)
	}

	fail = false
	calls := chain.callCount()
	if err := issueCheque(service, common.HexToAddress("0xdddd"), 10); err != nil {
		t.Fatal(err)
	}
	if chain.callCount() != calls {
		t.Fatalf("fitting cheque read the chain after a failed refresh: calls %d to %d", calls, chain.callCount())
	}
	expectAvailableBalance(t, service, 90)
}

func TestChequebookWithdrawalMonitorClosed(t *testing.T) {
	t.Parallel()

	errC := make(chan error, 1)
	chain := newChainMock(t, 100)
	service := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(40)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return make(chan types.Receipt), errC, nil
		}),
	)

	if _, err := service.Withdraw(context.Background(), big.NewInt(40)); err != nil {
		t.Fatal(err)
	}
	errC <- transaction.ErrMonitorClosed
	time.Sleep(20 * time.Millisecond)
	if chequebook.PendingWithdrawals(service) != 1 {
		t.Fatalf("pending withdrawals: got %d, want 1", chequebook.PendingWithdrawals(service))
	}
	expectAvailableBalance(t, service, 60)
	if err := issueCheque(service, common.HexToAddress("0xdddd"), 61); !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("got %v, want %v", err, chequebook.ErrOutOfFunds)
	}
}

func TestChequebookWithdrawalReverted(t *testing.T) {
	t.Parallel()

	receiptC := make(chan types.Receipt, 1)
	chain := newChainMock(t, 100)
	service := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(40)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return receiptC, make(chan error), nil
		}),
	)

	if _, err := service.Withdraw(context.Background(), big.NewInt(40)); err != nil {
		t.Fatal(err)
	}
	receiptC <- types.Receipt{Status: types.ReceiptStatusFailed}
	waitForPendingWithdrawals(t, service, 0)
	expectAvailableBalance(t, service, 100)
}

func TestChequebookTwoPendingWithdrawals(t *testing.T) {
	t.Parallel()

	var sent int
	receipts := map[common.Hash]chan types.Receipt{}
	chain := newChainMock(t, 100)
	service := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			sent++
			return common.BigToHash(big.NewInt(int64(sent))), nil
		}),
		transactionmock.WithWatchSentTransactionFunc(func(txHash common.Hash) (chan types.Receipt, chan error, error) {
			receiptC := make(chan types.Receipt, 1)
			receipts[txHash] = receiptC
			return receiptC, make(chan error), nil
		}),
	)

	if _, err := service.Withdraw(context.Background(), big.NewInt(30)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Withdraw(context.Background(), big.NewInt(40)); err != nil {
		t.Fatal(err)
	}
	expectAvailableBalance(t, service, 30)

	chain.setBalance(70)
	receipts[common.BigToHash(big.NewInt(1))] <- types.Receipt{Status: types.ReceiptStatusSuccessful}
	waitForPendingWithdrawals(t, service, 1)
	expectAvailableBalance(t, service, 30)
}

func TestChequebookIssueWhileChequeIsInFlight(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 100)
	service := newChainChequebook(t, chain, erc20mock.New())
	expectAvailableBalance(t, service, 100)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := service.Issue(context.Background(), common.HexToAddress("0xdddd"), big.NewInt(60), func(*chequebook.SignedCheque) error {
			close(started)
			<-release
			return nil
		})
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first cheque was not reserved")
	}
	if err := issueCheque(service, common.HexToAddress("0xeee"), 50); !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("cheque issued against a reserved amount: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := issueCheque(service, common.HexToAddress("0xeee"), 40); err != nil {
		t.Fatal(err)
	}
}

func TestChequebookWaitForDepositDropsCache(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 0)
	service := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithWaitForReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
			return &types.Receipt{Status: 1}, nil
		}),
	)
	expectAvailableBalance(t, service, 0)
	chain.setBalance(100)
	if err := service.WaitForDeposit(context.Background(), common.HexToHash("0x1")); err != nil {
		t.Fatal(err)
	}
	expectAvailableBalance(t, service, 100)
}

func TestChequebookBalanceIgnoresPendingWithdrawal(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 100)
	errC := make(chan error, 1)
	t.Cleanup(func() { errC <- transaction.ErrMonitorClosed })
	service := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(40)),
		transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
			return make(chan types.Receipt), errC, nil
		}),
	)

	if _, err := service.Withdraw(context.Background(), big.NewInt(40)); err != nil {
		t.Fatal(err)
	}
	onChain, err := service.Balance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if onChain.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("on-chain balance: got %d, want 100", onChain)
	}
	expectAvailableBalance(t, service, 60)
}

func TestChequebookAvailableBalanceCopy(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 100)
	service := newChainChequebook(t, chain, erc20mock.New())
	available, err := service.AvailableBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	available.SetInt64(1)
	expectAvailableBalance(t, service, 100)
}
