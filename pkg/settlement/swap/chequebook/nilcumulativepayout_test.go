// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	erc20mock "github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

// newNilPayoutChequeStore returns a cheque store backed by store, wired the way
// the node wires it, so that a corrupt statestore entry is what the service
// actually reads back.
func newNilPayoutChequeStore(store *rawStore, beneficiary common.Address) chequebook.ChequeStore {
	return chequebook.NewChequeStore(
		store,
		&factoryMock{verifyChequebook: func(context.Context, common.Address) error { return nil }},
		1,
		beneficiary,
		transactionmock.New(),
		func(*chequebook.SignedCheque, int64) (common.Address, error) { return common.Address{}, nil },
	)
}

// pendingBackend reports every transaction as pending, so CashoutStatus takes
// the branch that subtracts the cashout action payout from the last cheque.
func pendingBackend() transaction.Backend {
	return backendmock.New(
		backendmock.WithTransactionByHashFunc(func(context.Context, common.Hash) (*types.Transaction, bool, error) {
			return nil, true, nil
		}),
	)
}

// TestCashoutStatusNilCumulativePayoutLastCheque covers the guard in
// CashoutStatus that follows LastCheque. A last-received-cheque entry of `{}`
// unmarshals into a non-nil *SignedCheque whose CumulativePayout is a nil
// *big.Int; every CashoutStatus branch does arithmetic on it, so without the
// guard the arithmetic against the recorded cashout action panics. Reachable
// from GET /chequebook/cashout/{peer}.
func TestCashoutStatusNilCumulativePayoutLastCheque(t *testing.T) {
	t.Parallel()

	chequebookAddress := common.HexToAddress("0xcb")
	beneficiary := common.HexToAddress("0xbe")

	action, err := json.Marshal(&struct {
		TxHash common.Hash
		Cheque chequebook.SignedCheque
	}{
		TxHash: common.HexToHash("0xdddd"),
		Cheque: chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary,
				Chequebook:       chequebookAddress,
				CumulativePayout: big.NewInt(100),
			},
			Signature: []byte{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	store := newRawStore()
	store.setRaw(chequebook.LastReceivedChequeKey(chequebookAddress), []byte(`{}`))
	store.setRaw(chequebook.CashoutActionKey(chequebookAddress), action)

	cashoutService := chequebook.NewCashoutService(
		store,
		pendingBackend(),
		transactionmock.New(),
		newNilPayoutChequeStore(store, beneficiary),
	)

	_, err = cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("expected error wrapping %v, got %v", chequebook.ErrNoCheque, err)
	}
}

// TestCashoutStatusNilCumulativePayoutAction covers the second guard in
// CashoutStatus, on the cashout action loaded from its own statestore key. The
// last cheque here is valid, so only a corrupt cashoutActionKey entry reaches
// the guard.
func TestCashoutStatusNilCumulativePayoutAction(t *testing.T) {
	t.Parallel()

	chequebookAddress := common.HexToAddress("0xcb")
	beneficiary := common.HexToAddress("0xbe")

	lastCheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      beneficiary,
			Chequebook:       chequebookAddress,
			CumulativePayout: big.NewInt(500),
		},
		Signature: []byte{},
	}
	lastChequeData, err := json.Marshal(lastCheque)
	if err != nil {
		t.Fatal(err)
	}

	store := newRawStore()
	store.setRaw(chequebook.LastReceivedChequeKey(chequebookAddress), lastChequeData)
	// the cashout action is a distinct entry with the same shape
	store.setRaw(chequebook.CashoutActionKey(chequebookAddress), []byte(`{}`))

	cashoutService := chequebook.NewCashoutService(
		store,
		pendingBackend(),
		transactionmock.New(),
		newNilPayoutChequeStore(store, beneficiary),
	)

	_, err = cashoutService.CashoutStatus(context.Background(), chequebookAddress)
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("expected error wrapping %v, got %v", chequebook.ErrNoCheque, err)
	}
}

// TestCashChequeNilCumulativePayout covers the guard in CashCheque: the nil
// *big.Int would otherwise be dereferenced by the ABI packer.
func TestCashChequeNilCumulativePayout(t *testing.T) {
	t.Parallel()

	chequebookAddress := common.HexToAddress("0xcb")
	recipientAddress := common.HexToAddress("0xef")
	beneficiary := common.HexToAddress("0xbe")

	store := newRawStore()
	store.setRaw(chequebook.LastReceivedChequeKey(chequebookAddress), []byte(`{}`))

	cashoutService := chequebook.NewCashoutService(
		store,
		backendmock.New(),
		transactionmock.New(),
		newNilPayoutChequeStore(store, beneficiary),
	)

	_, err := cashoutService.CashCheque(context.Background(), chequebookAddress, recipientAddress)
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("expected error wrapping %v, got %v", chequebook.ErrNoCheque, err)
	}
}

// TestChequebookIssueNilCumulativePayout covers the sending-side mirror in
// Issue: a corrupt last-issued-cheque entry yields a non-nil cheque with a nil
// CumulativePayout, which Issue would otherwise add the amount to.
func TestChequebookIssueNilCumulativePayout(t *testing.T) {
	t.Parallel()

	address := common.HexToAddress("0xabcd")
	beneficiary := common.HexToAddress("0xdddd")
	ownerAddress := common.HexToAddress("0xfff")

	store := newRawStore()
	store.setRaw(chequebook.LastIssuedChequeKey(beneficiary), []byte(`{}`))

	chequebookService, err := chequebook.New(
		transactionmock.New(
			transactionmock.WithABICallSequence(
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(100).FillBytes(make([]byte, 32)), "balance"),
				transactionmock.ABICall(&chequebookABI, address, big.NewInt(0).FillBytes(make([]byte, 32)), "totalPaidOut"),
			),
		),
		address,
		ownerAddress,
		store,
		&chequeSignerMock{},
		erc20mock.New(),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = chequebookService.Issue(context.Background(), beneficiary, big.NewInt(20), func(*chequebook.SignedCheque) error {
		t.Fatal("cheque must not be sent")
		return nil
	})
	if !errors.Is(err, chequebook.ErrNoCheque) {
		t.Fatalf("expected error wrapping %v, got %v", chequebook.ErrNoCheque, err)
	}
}
