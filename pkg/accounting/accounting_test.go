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
	"time"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	testPrice       = uint64(10)
	testRefreshRate = int64(1000)
)

var (
	testPaymentTolerance = big.NewInt(1000)
	testPaymentEarly     = big.NewInt(1000)
	testPaymentThreshold = big.NewInt(10000)
)

type paymentCall struct {
	peer   swarm.Address
	amount *big.Int
}

// booking represents an accounting action and the expected result afterwards
type booking struct {
	peer              swarm.Address
	price             int64 // Credit if <0, Debit otherwise
	expectedBalance   int64
	originatedBalance int64
	originatedCredit  bool
	amount            int64
	notifyPaymentSent bool
}

// TestAccountingAddBalance does several accounting actions and verifies the balance after each steep
func TestAccountingAddBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
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
			err = acc.Credit(booking.peer, uint64(-booking.price), true)
			if err != nil {
				t.Fatal(err)
			}
			acc.Release(booking.peer, uint64(-booking.price))
		} else {
			debitAction := acc.PrepareDebit(booking.peer, uint64(booking.price))
			err = debitAction.Apply()
			if err != nil {
				t.Fatal(err)
			}
			debitAction.Cleanup()
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

// TestAccountingAddBalance does several accounting actions and verifies the balance after each steep
func TestAccountingAddOriginatedBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	bookings := []booking{
		// originated credit
		{peer: peer1Addr, price: -200, expectedBalance: -200, originatedBalance: -200, originatedCredit: true},
		// forwarder credit
		{peer: peer1Addr, price: -200, expectedBalance: -400, originatedBalance: -200, originatedCredit: false},
		// inconsequential debit not moving balance closer to 0 than originbalance is to 0
		{peer: peer1Addr, price: 100, expectedBalance: -300, originatedBalance: -200},
		// consequential debit moving balance closer to 0 than originbalance, therefore also moving originated balance along
		{peer: peer1Addr, price: 200, expectedBalance: -100, originatedBalance: -100},
		// notifypaymentsent that moves originated balance into positive domain
		{peer: peer1Addr, amount: 200, expectedBalance: 100, originatedBalance: 100, notifyPaymentSent: true},
		// inconsequential debit because originated balance is in the positive domain
		{peer: peer1Addr, price: 100, expectedBalance: 200, originatedBalance: 100},
		// originated credit moving the originated balance back into the negative domain, should be limited to the expectedbalance
		{peer: peer1Addr, price: -300, expectedBalance: -100, originatedBalance: -200, originatedCredit: true},
	}

	for i, booking := range bookings {
		if booking.notifyPaymentSent {
			acc.NotifyPaymentSent(booking.peer, big.NewInt(booking.amount), nil)

		} else {

			if booking.price < 0 {
				err = acc.Reserve(context.Background(), booking.peer, uint64(-booking.price))
				if err != nil {
					t.Fatal(err)
				}
				err = acc.Credit(booking.peer, uint64(-booking.price), booking.originatedCredit)
				if err != nil {
					t.Fatal(err)
				}
				acc.Release(booking.peer, uint64(-booking.price))
			} else {
				debitAction := acc.PrepareDebit(booking.peer, uint64(booking.price))
				err = debitAction.Apply()
				if err != nil {
					t.Fatal(err)
				}
				debitAction.Cleanup()
			}

		}

		balance, err := acc.Balance(booking.peer)
		if err != nil {
			t.Fatal(err)
		}

		if balance.Int64() != booking.expectedBalance {
			t.Fatalf("balance for peer %v not as expected after booking %d. got %d, wanted %d", booking.peer.String(), i, balance, booking.expectedBalance)
		}

		originatedBalance, err := acc.OriginatedBalance(booking.peer)
		if err != nil {
			t.Fatal(err)
		}

		if originatedBalance.Int64() != booking.originatedBalance {
			t.Fatalf("originated balance for peer %v not as expected after booking %d. got %d, wanted %d", booking.peer.String(), i, originatedBalance, booking.originatedBalance)
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

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
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
	debitAction := acc.PrepareDebit(peer1Addr, peer1DebitAmount)
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	peer2CreditAmount := 2 * testPrice
	err = acc.Credit(peer2Addr, peer2CreditAmount, true)
	if err != nil {
		t.Fatal(err)
	}

	acc, err = accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
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

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
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

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	// put the peer 1 unit away from disconnect
	debitAction := acc.PrepareDebit(peer1Addr, testPaymentThreshold.Uint64()+testPaymentTolerance.Uint64()-1)
	err = debitAction.Apply()
	if err != nil {
		t.Fatal("expected no error while still within tolerance")
	}
	debitAction.Cleanup()

	// put the peer over thee threshold
	debitAction = acc.PrepareDebit(peer1Addr, 1)
	err = debitAction.Apply()
	if err == nil {
		t.Fatal("expected Add to return error")
	}
	debitAction.Cleanup()

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

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int, shadowBalance *big.Int) (*big.Int, int64, error) {
		refreshchan <- paymentCall{peer: peer, amount: amount}
		return amount, 0, nil
	}

	pay := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
	}

	acc.SetRefreshFunc(f)
	acc.SetPayFunc(pay)

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	requestPrice := testPaymentThreshold.Uint64() - 1000

	err = acc.Reserve(context.Background(), peer1Addr, requestPrice)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment treshold
	err = acc.Credit(peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, requestPrice)

	// try another request
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(requestPrice))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}

	if acc.IsPaymentOngoing(peer1Addr) {
		t.Fatal("triggered monetary settlement")
	}

	acc.Release(peer1Addr, 1)

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

	// Credit until the expected debt exceeds payment threshold
	expectedAmount := testPaymentThreshold.Uint64() - 101
	err = acc.Reserve(context.Background(), peer1Addr, expectedAmount)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Credit(peer1Addr, expectedAmount, true)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, expectedAmount)

	// try another request to trigger settlement
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, 1)

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(expectedAmount))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, expectedAmount)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}

	if acc.IsPaymentOngoing(peer1Addr) {
		t.Fatal("triggered monetary settlement")
	}

	acc.Release(peer1Addr, 100)
}

func TestAccountingCallSettlementMonetary(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)
	paychan := make(chan paymentCall, 1)

	notTimeSettledAmount := big.NewInt(testRefreshRate * 2)

	acc.SetRefreshFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int, shadowBalance *big.Int) (*big.Int, int64, error) {
		refreshchan <- paymentCall{peer: peer, amount: amount}
		return new(big.Int).Sub(amount, notTimeSettledAmount), 0, nil
	})

	acc.SetPayFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		paychan <- paymentCall{peer: peer, amount: amount}
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	requestPrice := testPaymentThreshold.Uint64() - 1000

	err = acc.Reserve(context.Background(), peer1Addr, requestPrice)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment treshold
	err = acc.Credit(peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, requestPrice)

	// try another request
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(requestPrice))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}

	select {
	case call := <-paychan:
		if call.amount.Cmp(notTimeSettledAmount) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, notTimeSettledAmount)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}

	acc.Release(peer1Addr, 1)

	balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Cmp(new(big.Int).Neg(notTimeSettledAmount)) != 0 {
		t.Fatalf("expected balance to be adjusted. got %d", balance)
	}

	acc.SetRefreshFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int, shadowBalance *big.Int) (*big.Int, int64, error) {
		refreshchan <- paymentCall{peer: peer, amount: amount}
		return big.NewInt(0), 0, nil
	})

	// Credit until the expected debt exceeds payment threshold
	expectedAmount := testPaymentThreshold.Uint64()

	err = acc.Reserve(context.Background(), peer1Addr, expectedAmount)
	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatalf("expected overdraft, got %v", err)
	}

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(notTimeSettledAmount) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, notTimeSettledAmount)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for refreshment")
	}

	select {
	case <-paychan:
		t.Fatal("pay called twice")
	case <-time.After(1 * time.Second):
	}
}

func TestAccountingCallSettlementTooSoon(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)
	paychan := make(chan paymentCall, 1)

	ts := int64(1000)

	acc.SetRefreshFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int, shadowBalance *big.Int) (*big.Int, int64, error) {
		refreshchan <- paymentCall{peer: peer, amount: amount}
		return amount, ts, nil
	})

	acc.SetPayFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		paychan <- paymentCall{peer: peer, amount: amount}
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	requestPrice := testPaymentThreshold.Uint64() - 1000

	err = acc.Reserve(context.Background(), peer1Addr, requestPrice)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment treshold
	err = acc.Credit(peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, requestPrice)

	// try another request
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(requestPrice))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}

	acc.Release(peer1Addr, 1)

	balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("expected balance to be adjusted. got %d", balance)
	}

	acc.SetTime(ts)

	err = acc.Reserve(context.Background(), peer1Addr, requestPrice)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment treshold
	err = acc.Credit(peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, requestPrice)

	// try another request
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-refreshchan:
		t.Fatal("sent refreshment")
	default:
	}

	select {
	case call := <-paychan:
		if call.amount.Cmp(big.NewInt(int64(requestPrice))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("payment not sent")
	}

	acc.Release(peer1Addr, 1)

	acc.NotifyPaymentSent(peer1Addr, big.NewInt(int64(requestPrice)), errors.New("error"))
	acc.SetTime(ts + 1)

	// try another request
	err = acc.Reserve(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(requestPrice))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	default:
		t.Fatal("no refreshment")
	}
}

// TestAccountingCallSettlementEarly tests that settlement is called correctly if the payment threshold minus early payment is hit
func TestAccountingCallSettlementEarly(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	debt := uint64(500)
	earlyPayment := big.NewInt(1000)

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, earlyPayment, logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int, shadowBalance *big.Int) (*big.Int, int64, error) {
		refreshchan <- paymentCall{peer: peer, amount: amount}
		return amount, 0, nil
	}

	acc.SetRefreshFunc(f)

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Credit(peer1Addr, debt, true)
	if err != nil {
		t.Fatal(err)
	}

	payment := testPaymentThreshold.Uint64() - earlyPayment.Uint64()
	err = acc.Reserve(context.Background(), peer1Addr, payment)
	if err != nil {
		t.Fatal(err)
	}

	acc.Release(peer1Addr, payment)

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(debt))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, debt)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
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

	acc, err := accounting.NewAccounting(testPaymentThreshold, big.NewInt(0), big.NewInt(0), logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}
	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	// Try Debiting a large amount to peer so balance is large positive
	debitAction := acc.PrepareDebit(peer1Addr, testPaymentThreshold.Uint64()-1)
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()
	// Notify of incoming payment from same peer, so balance goes to 0 with surplusbalance 2
	err = acc.NotifyPaymentReceived(peer1Addr, new(big.Int).Add(testPaymentThreshold, big.NewInt(1)))
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
	err = acc.NotifyPaymentReceived(peer1Addr, testPaymentThreshold)
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
	debitAction = acc.PrepareDebit(peer1Addr, testPaymentThreshold.Uint64())
	err = debitAction.Apply()
	if err != nil {
		t.Fatal("Unexpected error from Credit")
	}
	debitAction.Cleanup()
	// sanity check surplus balance
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
	debitAction = acc.PrepareDebit(peer1Addr, testPaymentThreshold.Uint64())
	err = debitAction.Apply()
	if err != nil {
		t.Fatal("Unexpected error from Debit")
	}
	debitAction.Cleanup()
	// sanity check surplus balance
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
func TestAccountingNotifyPaymentReceived(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, nil, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	debtAmount := uint64(100)
	debitAction := acc.PrepareDebit(peer1Addr, debtAmount+testPaymentTolerance.Uint64())
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	err = acc.NotifyPaymentReceived(peer1Addr, new(big.Int).SetUint64(debtAmount+testPaymentTolerance.Uint64()))
	if err != nil {
		t.Fatal(err)
	}

	debitAction = acc.PrepareDebit(peer1Addr, debtAmount)
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	err = acc.NotifyPaymentReceived(peer1Addr, new(big.Int).SetUint64(debtAmount+testPaymentTolerance.Uint64()+1))
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

	_, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate))
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

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, big.NewInt(0), logger, store, pricing, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int, shadowBalance *big.Int) (*big.Int, int64, error) {
		refreshchan <- paymentCall{peer: peer, amount: amount}
		return amount, 0, nil
	}

	acc.SetRefreshFunc(f)

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

	err = acc.Credit(peer1Addr, debt, true)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Reserve(context.Background(), peer1Addr, lowerThreshold)
	if err == nil {
		t.Fatal(err)
	}

	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatal(err)
	}

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(debt))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, debt)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}
}

func TestAccountingPeerDebt(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, big.NewInt(0), logger, store, pricing, big.NewInt(testRefreshRate))
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr := swarm.MustParseHexAddress("00112233")
	debt := uint64(1000)
	debitAction := acc.PrepareDebit(peer1Addr, debt)
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()
	actualDebt, err := acc.PeerDebt(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if actualDebt.Cmp(new(big.Int).SetUint64(debt)) != 0 {
		t.Fatalf("wrong actual debt. got %d wanted %d", actualDebt, debt)
	}

	peer2Addr := swarm.MustParseHexAddress("11112233")
	err = acc.Credit(peer2Addr, 500, true)
	if err != nil {
		t.Fatal(err)
	}
	actualDebt, err = acc.PeerDebt(peer2Addr)
	if err != nil {
		t.Fatal(err)
	}
	if actualDebt.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("wrong actual debt. got %d wanted 0", actualDebt)
	}

	peer3Addr := swarm.MustParseHexAddress("22112233")
	actualDebt, err = acc.PeerDebt(peer3Addr)
	if err != nil {
		t.Fatal(err)
	}
	if actualDebt.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("wrong actual debt. got %d wanted 0", actualDebt)
	}

}
