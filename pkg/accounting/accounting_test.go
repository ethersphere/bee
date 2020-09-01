// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting_test

import (
	"context"
	"errors"
	"io/ioutil"
	"math"
	"testing"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	testPaymentThreshold      = 10000
	testPaymentThresholdLarge = math.MaxInt64 - 1
	testPaymentTolerance      = 1000
	testPrice                 = uint64(10)
)

// booking represents an accounting action and the expected result afterwards
type booking struct {
	peer            swarm.Address
	price           int64 // Credit if <0, Debit otherwise
	expectedBalance int64
}

// TestAccountingAddBalance does several accounting actions and verifies the balance after each steep
func TestAccountingAddBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThreshold,
		Logger:           logger,
		Store:            store,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	peer2Addr, err := swarm.ParseHexAddress("00112244")
	if err != nil {
		t.Fatal(err)
	}

	bookings := []booking{
		{peer: peer1Addr, price: 100, expectedBalance: 100},
		{peer: peer2Addr, price: 200, expectedBalance: 200},
		{peer: peer1Addr, price: 300, expectedBalance: 400},
		{peer: peer1Addr, price: -100, expectedBalance: 300},
		{peer: peer2Addr, price: -1000, expectedBalance: -800},
	}

	for i, booking := range bookings {
		if booking.price < 0 {
			err = acc.Reserve(booking.peer, uint64(-booking.price))
			if err != nil {
				t.Fatal(err)
			}
			err = acc.Credit(booking.peer, uint64(-booking.price))
			if err != nil {
				t.Fatal(err)
			}
			acc.Release(booking.peer, uint64(-booking.price))
		} else {
			err = acc.Debit(booking.peer, uint64(booking.price))
			if err != nil {
				t.Fatal(err)
			}
		}

		balance, err := acc.Balance(booking.peer)
		if err != nil {
			t.Fatal(err)
		}

		if balance != booking.expectedBalance {
			t.Fatalf("balance for peer %v not as expected after booking %d. got %d, wanted %d", booking.peer.String(), i, balance, booking.expectedBalance)
		}
	}
}

// TestAccountingAdd_persistentBalances tests that balances are actually persisted
// It creates an accounting instance, does some accounting
// Then it creates a new accounting instance with the same store and verifies the balances
func TestAccountingAdd_persistentBalances(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThreshold,
		Logger:           logger,
		Store:            store,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	peer2Addr, err := swarm.ParseHexAddress("00112244")
	if err != nil {
		t.Fatal(err)
	}

	peer1DebitAmount := testPrice
	err = acc.Debit(peer1Addr, peer1DebitAmount)
	if err != nil {
		t.Fatal(err)
	}

	peer2CreditAmount := 2 * testPrice
	err = acc.Credit(peer2Addr, peer2CreditAmount)
	if err != nil {
		t.Fatal(err)
	}

	acc, err = accounting.NewAccounting(accounting.Options{
		Logger: logger,
		Store:  store,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer1Balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}

	if peer1Balance != int64(peer1DebitAmount) {
		t.Fatalf("peer1Balance not loaded correctly. got %d, wanted %d", peer1Balance, peer1DebitAmount)
	}

	peer2Balance, err := acc.Balance(peer2Addr)
	if err != nil {
		t.Fatal(err)
	}

	if peer2Balance != -int64(peer2CreditAmount) {
		t.Fatalf("peer2Balance not loaded correctly. got %d, wanted %d", peer2Balance, -int64(peer2CreditAmount))
	}
}

// TestAccountingReserve tests that reserve returns an error if the payment threshold would be exceeded for a second time
func TestAccountingReserve(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThreshold,
		Logger:           logger,
		Store:            store,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	// it should allow to cross the threshold one time
	err = acc.Reserve(peer1Addr, testPaymentThreshold+1)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Reserve(peer1Addr, 1)
	if err == nil {
		t.Fatal("expected error from reserve")
	}

	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatalf("expected overdraft error from reserve, got %v", err)
	}
}

func TestAccountingOverflowsettle(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThresholdLarge,
		Logger:           logger,
		Store:            store,
	})

	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Credit(peer1Addr, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Credit(peer1Addr, 1)
	if err == nil {
		t.Fatal("Expecting error overflow from settle() func")
	}

	if !errors.Is(err, accounting.ErrOverflow) {
		t.Fatal("Expecting error overflow from settle() func")
	}
}

func TestAccountingOverflowReserve(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	settlement := &settlementMock{}

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThresholdLarge,
		Logger:           logger,
		Store:            store,
		Settlement:       settlement,
	})

	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Reserve(peer1Addr, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Reserve(peer1Addr, math.MaxInt64)
	if err == nil {
		t.Fatal("Expected error")
	}
	//

}

func TestAccountingOverflowNotifyPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	settlement := &settlementMock{}

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThresholdLarge,
		Logger:           logger,
		Store:            store,
		Settlement:       settlement,
	})

	if err != nil {
		t.Fatal(err)
	}
	//
	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Credit(peer1Addr, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.NotifyPayment(peer1Addr, math.MaxInt64)
	if err == nil {
		t.Fatal("Expected overflow from NotifyPayment")
	}

}

func TestAccountingOverflowDebit(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThresholdLarge,
		Logger:           logger,
		Store:            store,
	})

	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Debit(peer1Addr, math.MaxInt64-2)
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Debit(peer1Addr, math.MaxInt64-2)
	if err == nil {
		t.Fatal("Expected error from overflow Debit")
	}
	//
	if !errors.Is(err, accounting.ErrOverflow) {
		t.Fatalf("expected overflow error from Credit, got %v", err)
	}
	//
	err = acc.Debit(peer1Addr, math.MaxInt64)
	if err == nil {
		t.Fatal("Expected error from overflow Debit")
	}
	//
	if !errors.Is(err, accounting.ErrOverflow) {
		t.Fatalf("expected overflow error from Debit, got %v", err)
	}
}

func TestAccountingOverflowCredit(t *testing.T) {

	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThresholdLarge,
		Logger:           logger,
		Store:            store,
	})

	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Credit(peer1Addr, math.MaxInt64-2)
	if err != nil {
		t.Fatal(err)
	}
	//
	err = acc.Credit(peer1Addr, math.MaxInt64-2)
	if err == nil {
		t.Fatal("Expected error from overflow Credit")
	}
	//
	if !errors.Is(err, accounting.ErrOverflow) {
		t.Fatalf("expected overflow error from Credit, got %v", err)
	}
	//
	err = acc.Credit(peer1Addr, math.MaxInt64)
	if err == nil {
		t.Fatal("Expected error from overflow Credit")
	}
	//
	if !errors.Is(err, accounting.ErrOverflow) {
		t.Fatalf("expected overflow error from Credit, got %v", err)
	}
}

// TestAccountingDisconnect tests that exceeding the disconnect threshold with Debit returns a p2p.DisconnectError
func TestAccountingDisconnect(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThreshold,
		PaymentTolerance: testPaymentTolerance,
		Logger:           logger,
		Store:            store,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	// put the peer 1 unit away from disconnect
	err = acc.Debit(peer1Addr, testPaymentThreshold+testPaymentTolerance-1)
	if err != nil {
		t.Fatal("expected no error while still within tolerance")
	}

	// put the peer over thee threshold
	err = acc.Debit(peer1Addr, 1)
	if err == nil {
		t.Fatal("expected Add to return error")
	}

	var e *p2p.DisconnectError
	if !errors.As(err, &e) {
		t.Fatalf("expected DisconnectError, got %v", err)
	}
}

type settlementMock struct {
	paidAmount uint64
	paidPeer   swarm.Address
}

func (s *settlementMock) Pay(ctx context.Context, peer swarm.Address, amount uint64) error {
	s.paidPeer = peer
	s.paidAmount = amount
	return nil
}

// TestAccountingCallSettlement tests that settlement is called correctly if the payment threshold is hit
func TestAccountingCallSettlement(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	settlement := &settlementMock{}

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThreshold,
		PaymentTolerance: testPaymentTolerance,
		Logger:           logger,
		Store:            store,
		Settlement:       settlement,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Reserve(peer1Addr, testPaymentThreshold)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment treshold
	err = acc.Credit(peer1Addr, testPaymentThreshold)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, testPaymentThreshold)

	if !settlement.paidPeer.Equal(peer1Addr) {
		t.Fatalf("paid to wrong peer. got %v wanted %v", settlement.paidPeer, peer1Addr)
	}

	if settlement.paidAmount != testPaymentThreshold {
		t.Fatalf("paid wrong amount. got %d wanted %d", settlement.paidAmount, testPaymentThreshold)
	}

	balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("expected balance to be reset. got %d", balance)
	}

	// Assume 100 is reserved by some other request
	err = acc.Reserve(peer1Addr, 100)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until the expected debt exceeeds payment threshold
	expectedAmount := uint64(testPaymentThreshold - 100)
	err = acc.Reserve(peer1Addr, expectedAmount)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Credit(peer1Addr, expectedAmount)
	if err != nil {
		t.Fatal(err)
	}

	if !settlement.paidPeer.Equal(peer1Addr) {
		t.Fatalf("paid to wrong peer. got %v wanted %v", settlement.paidPeer, peer1Addr)
	}

	if settlement.paidAmount != expectedAmount {
		t.Fatalf("paid wrong amount. got %d wanted %d", settlement.paidAmount, expectedAmount)
	}
}

// TestAccountingNotifyPayment tests that payments adjust the balance and payment which put us into debt are rejected
func TestAccountingNotifyPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThreshold,
		PaymentTolerance: testPaymentTolerance,
		Logger:           logger,
		Store:            store,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	debtAmount := uint64(100)
	err = acc.Debit(peer1Addr, debtAmount+testPaymentTolerance)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.NotifyPayment(peer1Addr, debtAmount+testPaymentTolerance)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Debit(peer1Addr, debtAmount)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.NotifyPayment(peer1Addr, debtAmount+testPaymentTolerance+1)
	if err == nil {
		t.Fatal("expected payment to be rejected")
	}
}

func TestAccountingInvalidPaymentTolerance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	_, err := accounting.NewAccounting(accounting.Options{
		PaymentThreshold: testPaymentThreshold,
		PaymentTolerance: testPaymentThreshold/2 + 1,
		Logger:           logger,
		Store:            store,
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if err != accounting.ErrInvalidPaymentTolerance {
		t.Fatalf("got wrong error. got %v wanted %v", err, accounting.ErrInvalidPaymentTolerance)
	}
}
