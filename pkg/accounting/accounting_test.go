// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting_test

import (
	"context"
	"errors"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	mockSettlement "github.com/ethersphere/bee/pkg/settlement/swap/mock"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	testPrice = uint64(10)
)

var (
	testPaymentTolerance = big.NewInt(1000)
	testPaymentEarly     = big.NewInt(1000)
	testPaymentThreshold = big.NewInt(10000)
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

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, nil)
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
			err = acc.Reserve(context.Background(), booking.peer, uint64(-booking.price))
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

		if balance.Int64() != booking.expectedBalance {
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

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, nil)
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

	acc, err = accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	peer1Balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}

	if peer1Balance.Uint64() != peer1DebitAmount {
		t.Fatalf("peer1Balance not loaded correctly. got %d, wanted %d", peer1Balance, peer1DebitAmount)
	}

	peer2Balance, err := acc.Balance(peer2Addr)
	if err != nil {
		t.Fatal(err)
	}

	if peer2Balance.Int64() != -int64(peer2CreditAmount) {
		t.Fatalf("peer2Balance not loaded correctly. got %d, wanted %d", peer2Balance, -int64(peer2CreditAmount))
	}
}

// TestAccountingReserve tests that reserve returns an error if the payment threshold would be exceeded
func TestAccountingReserve(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	// it should allow to cross the threshold one time
	err = acc.Reserve(context.Background(), peer1Addr, testPaymentThreshold.Uint64()+1)
	if err == nil {
		t.Fatal("expected error from reserve")
	}

	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatalf("expected overdraft error from reserve, got %v", err)
	}
}

// TestAccountingDisconnect tests that exceeding the disconnect threshold with Debit returns a p2p.DisconnectError
func TestAccountingDisconnect(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	// put the peer 1 unit away from disconnect
	err = acc.Debit(peer1Addr, testPaymentThreshold.Uint64()+testPaymentTolerance.Uint64()-1)
	if err != nil {
		t.Fatal("expected no error while still within tolerance")
	}

	// put the peer over thee threshold
	err = acc.Debit(peer1Addr, 1)
	if err == nil {
		t.Fatal("expected Add to return error")
	}

	var e *p2p.BlockPeerError
	if !errors.As(err, &e) {
		t.Fatalf("expected BlockPeerError, got %v", err)
	}
}

// TestAccountingCallSettlement tests that settlement is called correctly if the payment threshold is hit
func TestAccountingCallSettlement(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	settlement := mockSettlement.New()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, settlement, nil)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Reserve(context.Background(), peer1Addr, testPaymentThreshold.Uint64())
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment treshold
	err = acc.Credit(peer1Addr, testPaymentThreshold.Uint64())
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, testPaymentThreshold.Uint64())

	// try another request
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, 1)

	totalSent, err := settlement.TotalSent(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(testPaymentThreshold) != 0 {
		t.Fatalf("paid wrong amount. got %d wanted %d", totalSent, testPaymentThreshold)
	}

	balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != 0 {
		t.Fatalf("expected balance to be reset. got %d", balance)
	}

	// Assume 100 is reserved by some other request
	err = acc.Reserve(context.Background(), peer1Addr, 100)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until the expected debt exceeeds payment threshold
	expectedAmount := testPaymentThreshold.Uint64() - 100
	err = acc.Reserve(context.Background(), peer1Addr, expectedAmount)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Credit(peer1Addr, expectedAmount)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, expectedAmount)

	// try another request
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, 1)

	totalSent, err = settlement.TotalSent(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(new(big.Int).Add(new(big.Int).SetUint64(expectedAmount), testPaymentThreshold)) != 0 {
		t.Fatalf("paid wrong amount. got %d wanted %d", totalSent, new(big.Int).Add(new(big.Int).SetUint64(expectedAmount), testPaymentThreshold))
	}

	acc.Release(peer1Addr, 100)
}

// TestAccountingCallSettlementEarly tests that settlement is called correctly if the payment threshold minus early payment is hit
func TestAccountingCallSettlementEarly(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	settlement := mockSettlement.New()
	debt := uint64(500)
	earlyPayment := big.NewInt(1000)

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, earlyPayment, logger, store, settlement, nil)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Credit(peer1Addr, debt)
	if err != nil {
		t.Fatal(err)
	}

	payment := testPaymentThreshold.Uint64() - earlyPayment.Uint64()
	err = acc.Reserve(context.Background(), peer1Addr, payment)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, payment)

	totalSent, err := settlement.TotalSent(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(new(big.Int).SetUint64(debt)) != 0 {
		t.Fatalf("paid wrong amount. got %d wanted %d", totalSent, testPaymentThreshold)
	}

	balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != 0 {
		t.Fatalf("expected balance to be reset. got %d", balance)
	}
}

func TestAccountingSurplusBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	settlement := mockSettlement.New()

	acc, err := accounting.NewAccounting(testPaymentThreshold, big.NewInt(0), big.NewInt(0), logger, store, settlement, nil)
	if err != nil {
		t.Fatal(err)
	}
	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	// Try Debiting a large amount to peer so balance is large positive
	err = acc.Debit(peer1Addr, testPaymentThreshold.Uint64()-1)
	if err != nil {
		t.Fatal(err)
	}
	// Notify of incoming payment from same peer, so balance goes to 0 with surplusbalance 2
	err = acc.NotifyPayment(peer1Addr, new(big.Int).Add(testPaymentThreshold, big.NewInt(1)))
	if err != nil {
		t.Fatal("Unexpected overflow from doable NotifyPayment")
	}
	//sanity check surplus balance
	val, err := acc.SurplusBalance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Surplusbalance")
	}
	if val.Int64() != 2 {
		t.Fatal("Not expected surplus balance")
	}
	//sanity check balance
	val, err = acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Balance")
	}
	if val.Int64() != 0 {
		t.Fatal("Not expected balance")
	}
	// Notify of incoming payment from same peer, so balance goes to 0 with surplusbalance 10002 (testpaymentthreshold+2)
	err = acc.NotifyPayment(peer1Addr, testPaymentThreshold)
	if err != nil {
		t.Fatal("Unexpected error from NotifyPayment")
	}
	//sanity check surplus balance
	val, err = acc.SurplusBalance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Surplusbalance")
	}
	if val.Int64() != testPaymentThreshold.Int64()+2 {
		t.Fatal("Unexpected surplus balance")
	}
	//sanity check balance
	val, err = acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Balance")
	}
	if val.Int64() != 0 {
		t.Fatal("Not expected balance, expected 0")
	}
	// Debit for same peer, so balance stays 0 with surplusbalance decreasing to 2
	err = acc.Debit(peer1Addr, testPaymentThreshold.Uint64())
	if err != nil {
		t.Fatal("Unexpected error from Credit")
	}
	// samity check surplus balance
	val, err = acc.SurplusBalance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Surplusbalance")
	}
	if val.Int64() != 2 {
		t.Fatal("Unexpected surplus balance")
	}
	//sanity check balance
	val, err = acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Balance")
	}
	if val.Int64() != 0 {
		t.Fatal("Not expected balance, expected 0")
	}
	// Debit for same peer, so balance goes to 9998 (testpaymentthreshold - 2) with surplusbalance decreasing to 0
	err = acc.Debit(peer1Addr, testPaymentThreshold.Uint64())
	if err != nil {
		t.Fatal("Unexpected error from NotifyPayment")
	}
	// samity check surplus balance
	val, err = acc.SurplusBalance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Surplusbalance")
	}
	if val.Int64() != 0 {
		t.Fatal("Unexpected surplus balance")
	}
	//sanity check balance
	val, err = acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal("Error checking Balance")
	}
	if val.Int64() != testPaymentThreshold.Int64()-2 {
		t.Fatal("Not expected balance, expected 0")
	}
}

// TestAccountingNotifyPayment tests that payments adjust the balance and payment which put us into debt are rejected
func TestAccountingNotifyPayment(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	debtAmount := uint64(100)
	err = acc.Debit(peer1Addr, debtAmount+testPaymentTolerance.Uint64())
	if err != nil {
		t.Fatal(err)
	}

	err = acc.NotifyPayment(peer1Addr, new(big.Int).SetUint64(debtAmount+testPaymentTolerance.Uint64()))
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Debit(peer1Addr, debtAmount)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.NotifyPayment(peer1Addr, new(big.Int).SetUint64(debtAmount+testPaymentTolerance.Uint64()+1))
	if err != nil {
		t.Fatal(err)
	}
}

type pricingMock struct {
	called           bool
	peer             swarm.Address
	paymentThreshold *big.Int
}

func (p *pricingMock) AnnouncePaymentThreshold(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error {
	p.called = true
	p.peer = peer
	p.paymentThreshold = paymentThreshold
	return nil
}

func (p *pricingMock) AnnouncePaymentThresholdAndPriceTable(ctx context.Context, peer swarm.Address, paymentThreshold *big.Int) error {
	p.called = true
	p.peer = peer
	p.paymentThreshold = paymentThreshold
	return nil
}

func TestAccountingConnected(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	_, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, pricing)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = pricing.AnnouncePaymentThreshold(context.Background(), peer1Addr, testPaymentThreshold)
	if err != nil {
		t.Fatal(err)
	}

	if !pricing.called {
		t.Fatal("expected pricing to be called")
	}

	if !pricing.peer.Equal(peer1Addr) {
		t.Fatalf("paid to wrong peer. got %v wanted %v", pricing.peer, peer1Addr)
	}

	if pricing.paymentThreshold != testPaymentThreshold {
		t.Fatalf("paid wrong amount. got %d wanted %d", pricing.paymentThreshold, testPaymentThreshold)
	}
}

func TestAccountingNotifyPaymentThreshold(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}
	settlement := mockSettlement.New()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, big.NewInt(0), logger, store, settlement, pricing)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	debt := uint64(50)
	lowerThreshold := uint64(100)

	err = acc.NotifyPaymentThreshold(peer1Addr, new(big.Int).SetUint64(lowerThreshold))
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Credit(peer1Addr, debt)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Reserve(context.Background(), peer1Addr, lowerThreshold)
	if err != nil {
		t.Fatal(err)
	}

	totalSent, err := settlement.TotalSent(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}

	if totalSent.Cmp(new(big.Int).SetUint64(debt)) != 0 {
		t.Fatalf("paid wrong amount. got %d wanted %d", totalSent, debt)
	}
}
