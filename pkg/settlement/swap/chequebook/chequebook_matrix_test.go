// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
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

// matrixCase is one cheque issued against a chequebook in a given cache state.
// A cheque that fits the cached figure does not read the chain, one that does
// not fit reloads the cache once before it is refused.
type matrixCase struct {
	name          string
	chain         int64 // chequebook balance on chain when the test starts
	warm          bool  // load the cache before the cheque
	expire        bool  // age the loaded cache past its validity
	issued        int64 // cheques issued before
	pending       int64 // withdrawal broadcast before
	minePending   bool  // mine and release the withdrawal before the cheque
	chainDelta    int64 // balance change on chain after the cache is loaded
	amount        int64
	wantErr       bool
	wantReads     int   // chain reads made by the cheque
	wantAvailable int64 // available balance on chain after the cheque
}

func TestChequebookCoveringBalanceMatrix(t *testing.T) {
	t.Parallel()

	cases := []matrixCase{
		// the first cheque loads the cache
		{name: "cold/fits", chain: 100, amount: 100, wantReads: 2, wantAvailable: 0},

		// a cheque that fits the cache does not read the chain
		{name: "warm/fits", chain: 100, warm: true, amount: 100, wantAvailable: 0},
		// a cheque that does not fit reloads once before it is refused
		{name: "warm/exceeds", chain: 100, warm: true, amount: 101, wantErr: true, wantReads: 2, wantAvailable: 100},

		// the reload picks up a top-up the cache has missed, and the cheque is checked against it
		{name: "warm/topped-up/exceeds-cache", chain: 100, warm: true, chainDelta: 50, amount: 101, wantReads: 2, wantAvailable: 49},
		{name: "warm/topped-up/exceeds-chain", chain: 100, warm: true, chainDelta: 50, amount: 151, wantErr: true, wantReads: 2, wantAvailable: 150},

		// an expired cache is reloaded even for a cheque that fits
		{name: "expired/fits", chain: 100, warm: true, expire: true, amount: 100, wantReads: 2, wantAvailable: 0},
		// a freshly loaded figure is not reloaded again when the cheque does not fit
		{name: "expired/exceeds", chain: 100, warm: true, expire: true, amount: 101, wantErr: true, wantReads: 2, wantAvailable: 100},

		// issued cheques are deducted
		{name: "warm/issued/fits", chain: 100, warm: true, issued: 40, amount: 60, wantAvailable: 0},
		{name: "warm/issued/exceeds", chain: 100, warm: true, issued: 40, amount: 61, wantErr: true, wantReads: 2, wantAvailable: 60},

		// a pending withdrawal is deducted, also after the reload
		{name: "warm/pending/fits", chain: 100, warm: true, pending: 40, amount: 60, wantAvailable: 0},
		{name: "warm/pending/exceeds", chain: 100, warm: true, pending: 40, amount: 61, wantErr: true, wantReads: 2, wantAvailable: 60},

		// a mined withdrawal is no longer deducted and the cache holds the balance after it
		{name: "warm/withdrawn/fits", chain: 100, warm: true, pending: 40, minePending: true, amount: 60, wantAvailable: 0},
		{name: "warm/withdrawn/exceeds", chain: 100, warm: true, pending: 40, minePending: true, amount: 61, wantErr: true, wantReads: 2, wantAvailable: 60},

		// accepted limitation: a decrease not made by this node is invisible until the cache expires
		{name: "warm/drained-elsewhere/fits-cache", chain: 100, warm: true, chainDelta: -50, amount: 60, wantAvailable: -10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service, chain := openMatrixChequebook(t, tc)

			before := chain.callCount()
			err := issueCheque(service, common.HexToAddress("0xdddd"), tc.amount)
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
		})
	}
}

func openMatrixChequebook(t *testing.T, tc matrixCase) (chequebook.Service, *chainMock) {
	t.Helper()

	chain := newChainMock(t, tc.chain)
	var (
		opts     []transactionmock.Option
		receiptC chan types.Receipt
	)
	if tc.pending > 0 {
		receiptC = make(chan types.Receipt, 1)
		errC := make(chan error, 1)
		t.Cleanup(func() { errC <- transaction.ErrMonitorClosed })
		opts = append(opts,
			transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(tc.pending)),
			transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
				return receiptC, errC, nil
			}),
		)
	}
	service := newChainChequebook(t, chain, erc20mock.New(), opts...)

	if tc.warm {
		expectAvailableBalance(t, service, tc.chain)
	}
	if tc.issued > 0 {
		if err := issueCheque(service, common.HexToAddress("0xdddd"), tc.issued); err != nil {
			t.Fatal(err)
		}
	}
	balance := tc.chain
	if tc.pending > 0 {
		if _, err := service.Withdraw(context.Background(), big.NewInt(tc.pending)); err != nil {
			t.Fatal(err)
		}
	}
	if tc.minePending {
		balance -= tc.pending
		chain.setBalance(balance)
		receiptC <- types.Receipt{Status: types.ReceiptStatusSuccessful}
		waitForPendingWithdrawals(t, service, 0)
	}
	if tc.expire {
		chequebook.SetCoveringBalanceCachedAt(service, time.Now().Add(-6*time.Minute))
	}
	if tc.chainDelta != 0 {
		chain.setBalance(balance + tc.chainDelta)
	}
	return service, chain
}

func TestChequebookRefreshFailureDropsCache(t *testing.T) {
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
		0,
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

	if err := issueCheque(service, common.HexToAddress("0xdddd"), 10); !errors.Is(err, rpcErr) {
		t.Fatalf("fitting cheque after a failed refresh: got %v, want %v", err, rpcErr)
	}

	fail = false
	calls := chain.callCount()
	if err := issueCheque(service, common.HexToAddress("0xdddd"), 10); err != nil {
		t.Fatal(err)
	}
	if got := chain.callCount() - calls; got != 2 {
		t.Fatalf("chain reads for the cheque after a failed refresh: got %d, want 2", got)
	}
	expectAvailableBalance(t, service, 90)
}

func TestChequebookWithdrawalReleaseReloadFailure(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 100)
	rpcErr := errors.New("rpc failed")
	var fail atomic.Bool
	receiptC := make(chan types.Receipt, 1)
	service, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) ([]byte, error) {
				if fail.Load() {
					return nil, rpcErr
				}
				return chain.call(ctx, request)
			}),
			transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(90)),
			transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
				return receiptC, make(chan error), nil
			}),
		),
		common.HexToAddress("0xabcd"),
		common.HexToAddress("0xfff"),
		storemock.NewStateStore(),
		&chequeSignerMock{sign: func(*chequebook.Cheque) ([]byte, error) { return nil, nil }},
		erc20mock.New(),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	expectAvailableBalance(t, service, 100)
	if _, err := service.Withdraw(context.Background(), big.NewInt(90)); err != nil {
		t.Fatal(err)
	}

	// the withdrawal is mined, but the reload after the hold fails
	chain.setBalance(10)
	fail.Store(true)
	receiptC <- types.Receipt{Status: types.ReceiptStatusSuccessful}
	waitForPendingWithdrawals(t, service, 0)
	fail.Store(false)

	if err := issueCheque(service, common.HexToAddress("0xdddd"), 50); !errors.Is(err, chequebook.ErrOutOfFunds) {
		t.Fatalf("issue against the pre-withdrawal balance: got %v, want %v", err, chequebook.ErrOutOfFunds)
	}
	if err := issueCheque(service, common.HexToAddress("0xdddd"), 10); err != nil {
		t.Fatal(err)
	}
	expectAvailableBalance(t, service, 0)
}

func TestChequebookCoveringBalanceReadTimeout(t *testing.T) {
	t.Parallel()

	chain := newChainMock(t, 100)
	var (
		stall       atomic.Bool
		stalledOnce sync.Once
	)
	stalled := make(chan struct{})
	receiptC := make(chan types.Receipt, 1)
	service, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) ([]byte, error) {
				if stall.Load() {
					stalledOnce.Do(func() { close(stalled) })
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return chain.call(ctx, request)
			}),
			transactionmock.WithABISend(&chequebookABI, common.HexToHash("0x1"), common.HexToAddress("0xabcd"), big.NewInt(0), "withdraw", big.NewInt(10)),
			transactionmock.WithWatchSentTransactionFunc(func(common.Hash) (chan types.Receipt, chan error, error) {
				return receiptC, make(chan error), nil
			}),
		),
		common.HexToAddress("0xabcd"),
		common.HexToAddress("0xfff"),
		storemock.NewStateStore(),
		&chequeSignerMock{sign: func(*chequebook.Cheque) ([]byte, error) { return nil, nil }},
		erc20mock.New(),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	chequebook.SetCoveringBalanceReadTimeout(service, 100*time.Millisecond)

	expectAvailableBalance(t, service, 100)
	if _, err := service.Withdraw(context.Background(), big.NewInt(10)); err != nil {
		t.Fatal(err)
	}
	stall.Store(true)
	receiptC <- types.Receipt{Status: types.ReceiptStatusSuccessful}
	select {
	case <-stalled:
	case <-time.After(5 * time.Second):
		t.Fatal("covering balance was not reloaded after the withdrawal")
	}

	done := make(chan error, 1)
	go func() {
		done <- issueCheque(service, common.HexToAddress("0xdddd"), 1)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("issue during a stalled reload: got %v, want %v", err, context.DeadlineExceeded)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("issue blocked behind a stalled covering balance reload")
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

	var (
		mu       sync.Mutex
		sent     int
		receipts = map[common.Hash]chan types.Receipt{}
	)
	chain := newChainMock(t, 100)
	service := newChainChequebook(t, chain, erc20mock.New(),
		transactionmock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			mu.Lock()
			defer mu.Unlock()
			sent++
			return common.BigToHash(big.NewInt(int64(sent))), nil
		}),
		transactionmock.WithWatchSentTransactionFunc(func(txHash common.Hash) (chan types.Receipt, chan error, error) {
			receiptC := make(chan types.Receipt, 1)
			mu.Lock()
			receipts[txHash] = receiptC
			mu.Unlock()
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

	deadline := time.Now().Add(5 * time.Second)
	var first chan types.Receipt
	for {
		mu.Lock()
		first = receipts[common.BigToHash(big.NewInt(1))]
		registered := len(receipts)
		mu.Unlock()
		if first != nil && registered == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("withdrawal was not watched")
		}
		time.Sleep(10 * time.Millisecond)
	}

	chain.setBalance(70)
	first <- types.Receipt{Status: types.ReceiptStatusSuccessful}
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
