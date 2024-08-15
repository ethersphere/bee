// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting_test

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	p2pmock "github.com/ethersphere/bee/v2/pkg/p2p/mock"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	testPrice       = uint64(10)
	testRefreshRate = int64(1000)
)

var (
	testPaymentTolerance = int64(10)
	testPaymentEarly     = int64(10)
	testPaymentThreshold = big.NewInt(10000)
	testLightFactor      = int64(10)
)

type paymentCall struct {
	peer   swarm.Address
	amount *big.Int
}

// booking represents an accounting action and the expected result afterwards
type bookingT struct {
	peer              swarm.Address
	price             int64 // Credit if <0, Debit otherwise
	expectedBalance   int64
	originatedBalance int64
	originatedCredit  bool
	notifyPaymentSent bool
	overpay           uint64
}

func TestMutex(t *testing.T) {
	t.Parallel()

	t.Run("locked mutex can not be locked again", func(t *testing.T) {
		t.Parallel()

		var (
			m  = accounting.NewMutex()
			c  = make(chan struct{}, 1)
			wg sync.WaitGroup
		)

		m.Lock()

		wg.Add(1)
		go func() {
			wg.Done()
			m.Lock()
			c <- struct{}{}
		}()

		wg.Wait()

		select {
		case <-c:
			t.Error("not expected to acquire the lock")
		case <-time.After(time.Millisecond):
		}

		m.Unlock()
		<-c
	})

	t.Run("can lock after release", func(t *testing.T) {
		t.Parallel()

		m := accounting.NewMutex()
		m.Lock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()

		m.Unlock()
		if err := m.TryLock(ctx); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("locked mutex takes context into account", func(t *testing.T) {
		t.Parallel()

		m := accounting.NewMutex()
		m.Lock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		if err := m.TryLock(ctx); !errors.Is(err, accounting.ErrFailToLock) {
			t.Errorf("expected %v, got %v", accounting.ErrFailToLock, err)
		}
	})
}

// TestAccountingAddBalance does several accounting actions and verifies the balance after each steep
func TestAccountingAddBalance(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
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

	acc.Connect(peer1Addr, true)
	acc.Connect(peer2Addr, true)

	bookings := []bookingT{
		{peer: peer1Addr, price: 100, expectedBalance: 100},
		{peer: peer2Addr, price: 200, expectedBalance: 200},
		{peer: peer1Addr, price: 300, expectedBalance: 400},
		{peer: peer1Addr, price: -100, expectedBalance: 300},
		{peer: peer2Addr, price: -1000, expectedBalance: -800},
	}

	for i, booking := range bookings {
		if booking.price < 0 {
			creditAction, err := acc.PrepareCredit(context.Background(), booking.peer, uint64(-booking.price), true)
			if err != nil {
				t.Fatal(err)
			}
			err = creditAction.Apply()
			if err != nil {
				t.Fatal(err)
			}
			creditAction.Cleanup()
		} else {
			debitAction, err := acc.PrepareDebit(context.Background(), booking.peer, uint64(booking.price))
			if err != nil {
				t.Fatal(err)
			}
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
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(0), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, big.NewInt(0), big.NewInt(0), 1000, 0, nil)
	}

	acc.SetRefreshFunc(f)

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	bookings := []bookingT{
		// originated credit
		{peer: peer1Addr, price: -2000, expectedBalance: -2000, originatedBalance: -2000, originatedCredit: true},
		// forwarder credit
		{peer: peer1Addr, price: -2000, expectedBalance: -4000, originatedBalance: -2000, originatedCredit: false},
		// inconsequential debit not moving balance closer to 0 than originbalance is to 0
		{peer: peer1Addr, price: 1000, expectedBalance: -3000, originatedBalance: -2000},
		// consequential debit moving balance closer to 0 than originbalance, therefore also moving originated balance along
		{peer: peer1Addr, price: 2000, expectedBalance: -1000, originatedBalance: -1000},
		// forwarder credit happening to increase debt
		{peer: peer1Addr, price: -1000, expectedBalance: -2000, originatedBalance: -1000, originatedCredit: false},
		// expect notifypaymentsent triggered by reserve that moves originated balance into positive domain because of earlier debit triggering overpay
		{peer: peer1Addr, price: -7000, expectedBalance: 1000, originatedBalance: 1000, overpay: 9000, notifyPaymentSent: true},
		// inconsequential debit because originated balance is in the positive domain
		{peer: peer1Addr, price: 1000, expectedBalance: 2000, originatedBalance: 1000},
		// originated credit moving the originated balance back into the negative domain, should be limited to the expectedbalance
		{peer: peer1Addr, price: -3000, expectedBalance: -1000, originatedBalance: -1000, originatedCredit: true},
	}

	paychan := make(chan struct{})

	for i, booking := range bookings {

		makePayFunc := func(currentBooking bookingT) func(ctx context.Context, peer swarm.Address, amount *big.Int) {
			return func(ctx context.Context, peer swarm.Address, amount *big.Int) {
				if currentBooking.overpay != 0 {
					debitAction, err := acc.PrepareDebit(context.Background(), peer, currentBooking.overpay)
					if err != nil {
						t.Fatal(err)
					}
					_ = debitAction.Apply()
				}

				acc.NotifyPaymentSent(peer, amount, nil)
				paychan <- struct{}{}
			}
		}

		payFunc := makePayFunc(booking)

		acc.SetPayFunc(payFunc)

		if booking.price < 0 {
			creditAction, err := acc.PrepareCredit(context.Background(), booking.peer, uint64(-booking.price), booking.originatedCredit)
			if err != nil {
				t.Fatal(err)
			}

			if booking.notifyPaymentSent {
				select {
				case <-paychan:
				case <-time.After(1 * time.Second):
					t.Fatal("expected payment sent", booking)
				}
			}

			err = creditAction.Apply()
			if err != nil {
				t.Fatal(err)
			}

			creditAction.Cleanup()
		} else {
			debitAction, err := acc.PrepareDebit(context.Background(), booking.peer, uint64(booking.price))
			if err != nil {
				t.Fatal(err)
			}
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
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
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

	acc.Connect(peer1Addr, true)
	acc.Connect(peer2Addr, true)

	peer1DebitAmount := testPrice
	debitAction, err := acc.PrepareDebit(context.Background(), peer1Addr, peer1DebitAmount)
	if err != nil {
		t.Fatal(err)
	}
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	peer2CreditAmount := 2 * testPrice
	creditAction, err := acc.PrepareCredit(context.Background(), peer2Addr, peer2CreditAmount, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = creditAction.Apply(); err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()

	acc, err = accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
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
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	_, err = acc.PrepareCredit(context.Background(), peer1Addr, testPaymentThreshold.Uint64()+2*uint64(testRefreshRate)+1, true)

	if err == nil {
		t.Fatal("expected error from reserve")
	}

	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatalf("expected overdraft error from reserve, got %v", err)
	}
}

// TestAccountingReserve tests that reserve returns an error if the payment threshold would be exceeded,
// but extends this limit by 'n' (max 2) seconds worth of refresh rate if last refreshment was 'n' seconds ago.
func TestAccountingReserveAheadOfTime(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}
	ts := int64(0)

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, amount, amount, ts*1000, 0, nil)
		refreshchan <- paymentCall{peer: peer, amount: amount}
	}

	pay := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
	}

	acc.SetRefreshFunc(f)
	acc.SetPayFunc(pay)

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	acc.SetTime(ts)

	// reserve until limit

	_, err = acc.PrepareCredit(context.Background(), peer1Addr, testPaymentThreshold.Uint64(), false)
	if err != nil {
		t.Fatal(err)
	}

	// attempt further reserve, expect overdraft

	_, err = acc.PrepareCredit(context.Background(), peer1Addr, 1, false)
	if err == nil {
		t.Fatal("expected error from reserve")
	}
	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatalf("expected overdraft error from reserve, got %v", err)
	}

	// pass time to increase reserve limit

	ts = int64(1)
	acc.SetTime(ts)

	// reserve until limit extended by 1 second passed

	credit1, err := acc.PrepareCredit(context.Background(), peer1Addr, uint64(testRefreshRate)-1, false)
	if err != nil {
		t.Fatal(err)
	}

	credit2, err := acc.PrepareCredit(context.Background(), peer1Addr, 1, false)
	if err != nil {
		t.Fatal(err)
	}

	// attempt further reserve, expect overdraft

	_, err = acc.PrepareCredit(context.Background(), peer1Addr, 1, false)
	if err == nil {
		t.Fatal("expected error from reserve")
	}
	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatalf("expected overdraft error from reserve, got %v", err)
	}

	// apply less than minimal refreshment amount, expect no refreshment

	err = credit1.Apply()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-refreshchan:
		t.Fatal("unexpected refreshment below minimum debt")
	case <-time.After(1 * time.Second):
	}

	// pass time to see it doesn't mean a further increase to reserve limit beyond 2 seconds

	ts = int64(2)
	acc.SetTime(ts)

	// attempt further reserve, expect overdraft

	_, err = acc.PrepareCredit(context.Background(), peer1Addr, 1, false)
	if err == nil {
		t.Fatal("expected error from reserve")
	}
	if !errors.Is(err, accounting.ErrOverdraft) {
		t.Fatalf("expected overdraft error from reserve, got %v", err)
	}

	ts = int64(3)
	acc.SetTime(ts)

	// credit until minimal refreshment sent, expect refreshment made

	err = credit2.Apply()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(testRefreshRate)) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, testRefreshRate)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}

	// pass time

	ts = int64(4)
	acc.SetTime(ts)

	// see that refresh made further reserve possible

	_, err = acc.PrepareCredit(context.Background(), peer1Addr, uint64(testRefreshRate), false)
	if err != nil {
		t.Fatal(err)
	}
}

// TestAccountingDisconnect tests that exceeding the disconnect threshold with Debit returns a p2p.DisconnectError
func TestAccountingDisconnect(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	// put the peer 1 unit away from disconnect
	debitAction, err := acc.PrepareDebit(context.Background(), peer1Addr, uint64(testRefreshRate)+(testPaymentThreshold.Uint64()*(100+uint64(testPaymentTolerance))/100)-1)
	if err != nil {
		t.Fatal(err)
	}
	err = debitAction.Apply()
	if err != nil {
		t.Fatal("expected no error while still within tolerance")
	}
	debitAction.Cleanup()

	// put the peer over thee threshold
	debitAction, err = acc.PrepareDebit(context.Background(), peer1Addr, 1)
	if err != nil {
		t.Fatal(err)
	}
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
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, amount, amount, 0, 0, nil)
		refreshchan <- paymentCall{peer: peer, amount: amount}
	}

	pay := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
	}

	acc.SetRefreshFunc(f)
	acc.SetPayFunc(pay)

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	requestPrice := testPaymentThreshold.Uint64() - 1000

	creditAction, err := acc.PrepareCredit(context.Background(), peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment threshold
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}

	creditAction.Cleanup()

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

	balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Int64() != 0 {
		t.Fatalf("expected balance to be reset. got %d", balance)
	}

	// Assume 100 is reserved by some other request
	creditActionLong, err := acc.PrepareCredit(context.Background(), peer1Addr, 100, true)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until the expected debt exceeds payment threshold
	expectedAmount := testPaymentThreshold.Uint64() - 101
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, expectedAmount, true)
	if err != nil {
		t.Fatal(err)
	}

	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}

	creditAction.Cleanup()

	// try another request to trigger settlement
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 1, true)
	if err != nil {
		t.Fatal(err)
	}

	creditAction.Cleanup()

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
	creditActionLong.Cleanup()
}

func TestAccountingCallSettlementMonetary(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)
	paychan := make(chan paymentCall, 1)

	ts := int64(1000)

	acc.SetTime(ts)

	acc.SetRefreshFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, amount, amount, ts*1000, 0, nil)
		refreshchan <- paymentCall{peer: peer, amount: amount}
	})

	acc.SetPayFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyPaymentSent(peer, amount, nil)
		paychan <- paymentCall{peer: peer, amount: amount}
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	requestPrice := testPaymentThreshold.Uint64()

	creditAction, err := acc.PrepareCredit(context.Background(), peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}
	// Credit until payment threshold
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()

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

	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}

	// Credit until payment threshold
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()

	select {
	case call := <-paychan:
		if call.amount.Cmp(testPaymentThreshold) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
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
	if balance.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("expected balance to be adjusted. got %d", balance)
	}

	ts++
	acc.SetTime(ts)

	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(testPaymentThreshold) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for refreshment")
	}

	notTimeSettleableAmount := new(big.Int).Sub(testPaymentThreshold, big.NewInt(testRefreshRate))

	select {
	case call := <-paychan:
		if call.amount.Cmp(notTimeSettleableAmount) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPrice)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for payment")
	}

}

func TestAccountingCallSettlementTooSoon(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}
	refreshchan := make(chan paymentCall, 1)
	paychan := make(chan paymentCall, 1)
	ts := int64(1000)

	acc.SetRefreshFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, amount, amount, ts*1000, 2, nil)
		refreshchan <- paymentCall{peer: peer, amount: amount}
	})

	acc.SetPayFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		paychan <- paymentCall{peer: peer, amount: amount}
	})
	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	requestPrice := testPaymentThreshold.Uint64() - 1000

	creditAction, err := acc.PrepareCredit(context.Background(), peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}
	// Credit until payment threshold
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()
	// try another request
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 1, true)
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
	creditAction.Cleanup()
	balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("expected balance to be adjusted. got %d", balance)
	}
	acc.SetTime(ts)
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}
	// Credit until payment threshold
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()
	// try another request
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 1, true)
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
	creditAction.Cleanup()
	acc.NotifyPaymentSent(peer1Addr, big.NewInt(int64(requestPrice)), errors.New("error"))
	acc.SetTime(ts + 1)
	// try another request
	_, err = acc.PrepareCredit(context.Background(), peer1Addr, 1, true)
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
		t.Fatal("no refreshment")
	}
}

// TestAccountingCallSettlementEarly tests that settlement is called correctly if the payment threshold minus early payment is hit
func TestAccountingCallSettlementEarly(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	debt := uint64(500)
	earlyPayment := int64(10)

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, earlyPayment, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, amount, amount, 0, 0, nil)
		refreshchan <- paymentCall{peer: peer, amount: amount}
	}

	acc.SetRefreshFunc(f)

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	creditAction, err := acc.PrepareCredit(context.Background(), peer1Addr, debt, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = creditAction.Apply(); err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()

	debt2 := testPaymentThreshold.Uint64()*(100-uint64(earlyPayment))/100 - debt
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, debt2, true)
	if err != nil {
		t.Fatal(err)
	}

	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}

	creditAction.Cleanup()

	select {
	case call := <-refreshchan:
		if call.amount.Cmp(big.NewInt(int64(debt+debt2))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, debt+debt2)
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
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, 0, 0, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}
	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	acc.Connect(peer1Addr, true)

	// Try Debiting a large amount to peer so balance is large positive
	debitAction, err := acc.PrepareDebit(context.Background(), peer1Addr, testPaymentThreshold.Uint64()-1)
	if err != nil {
		t.Fatal(err)
	}
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
	debitAction, err = acc.PrepareDebit(context.Background(), peer1Addr, testPaymentThreshold.Uint64())
	if err != nil {
		t.Fatal(err)
	}
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
	debitAction, err = acc.PrepareDebit(context.Background(), peer1Addr, testPaymentThreshold.Uint64())
	if err != nil {
		t.Fatal(err)
	}
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

// TestAccountingNotifyPayment tests that payments adjust the balance
func TestAccountingNotifyPaymentReceived(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	debtAmount := uint64(100)
	debitAction, err := acc.PrepareDebit(context.Background(), peer1Addr, debtAmount)
	if err != nil {
		t.Fatal(err)
	}
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	err = acc.NotifyPaymentReceived(peer1Addr, new(big.Int).SetUint64(debtAmount))
	if err != nil {
		t.Fatal(err)
	}

	debitAction, err = acc.PrepareDebit(context.Background(), peer1Addr, debtAmount)
	if err != nil {
		t.Fatal(err)
	}
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	err = acc.NotifyPaymentReceived(peer1Addr, new(big.Int).SetUint64(debtAmount))
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

func TestAccountingConnected(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	_, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
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

	if pricing.paymentThreshold.Cmp(testPaymentThreshold) != 0 {
		t.Fatalf("paid wrong amount. got %d wanted %d", pricing.paymentThreshold, testPaymentThreshold)
	}
}

func TestAccountingNotifyPaymentThreshold(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, 0, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)

	f := func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, amount, amount, 0, 0, nil)
		refreshchan <- paymentCall{peer: peer, amount: amount}
	}

	acc.SetRefreshFunc(f)

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, true)

	debt := 2 * uint64(testRefreshRate)
	lowerThreshold := debt

	err = acc.NotifyPaymentThreshold(peer1Addr, new(big.Int).SetUint64(lowerThreshold))
	if err != nil {
		t.Fatal(err)
	}

	creditAction, err := acc.PrepareCredit(context.Background(), peer1Addr, debt, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = creditAction.Apply(); err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()

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
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, 0, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr := swarm.MustParseHexAddress("00112233")
	acc.Connect(peer1Addr, true)

	debt := uint64(1000)
	debitAction, err := acc.PrepareDebit(context.Background(), peer1Addr, debt)
	if err != nil {
		t.Fatal(err)
	}
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
	acc.Connect(peer2Addr, true)
	creditAction, err := acc.PrepareCredit(context.Background(), peer2Addr, 500, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = creditAction.Apply(); err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()
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

func TestAccountingCallPaymentErrorRetries(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(1), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}

	refreshchan := make(chan paymentCall, 1)
	paychan := make(chan paymentCall, 1)

	ts := int64(100)
	acc.SetTime(ts)

	acc.SetRefreshFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		acc.NotifyRefreshmentSent(peer, amount, big.NewInt(2), ts*1000, 0, nil)
		refreshchan <- paymentCall{peer: peer, amount: amount}
	})

	acc.SetPayFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		paychan <- paymentCall{peer: peer, amount: amount}
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	acc.Connect(peer1Addr, true)

	requestPrice := testPaymentThreshold.Uint64() - 100

	// Credit until near payment threshold
	creditAction, err := acc.PrepareCredit(context.Background(), peer1Addr, requestPrice, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = creditAction.Apply(); err != nil {
		t.Fatal(err)
	}
	creditAction.Cleanup()

	select {
	case <-refreshchan:
	case <-time.After(1 * time.Second):
		t.Fatalf("expected refreshment")
	}

	// Credit until near payment threshold
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = creditAction.Apply(); err != nil {
		t.Fatal(err)
	}

	var sentAmount *big.Int
	select {
	case call := <-paychan:
		sentAmount = call.amount
	case <-time.After(1 * time.Second):
		t.Fatal("payment expected to be sent")
	}

	creditAction.Cleanup()

	acc.NotifyPaymentSent(peer1Addr, sentAmount, errors.New("error"))

	// try another n requests 1 per second
	for i := 0; i < 10; i++ {
		ts++
		acc.SetTime(ts)

		creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 2, true)
		if err != nil {
			t.Fatal(err)
		}

		err = creditAction.Apply()
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-refreshchan:
		case <-time.After(1 * time.Second):
			t.Fatal("expected refreshment")
		}

		creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 2, true)
		if err != nil {
			t.Fatal(err)
		}

		err = creditAction.Apply()
		if err != nil {
			t.Fatal(err)
		}

		if acc.IsPaymentOngoing(peer1Addr) {
			t.Fatal("unexpected ongoing payment")
		}

		creditAction.Cleanup()
	}

	ts++
	acc.SetTime(ts)

	// try another request that uses refreshment
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-refreshchan:
	case <-time.After(1 * time.Second):
		t.Fatalf("expected refreshment")

	}

	// try another request that uses payment as it happens in the second of last refreshment
	creditAction, err = acc.PrepareCredit(context.Background(), peer1Addr, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	err = creditAction.Apply()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-paychan:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("payment expected to be sent")
	}

	creditAction.Cleanup()
}

var errInvalidReason = errors.New("invalid blocklist reason")

func TestAccountingGhostOverdraft(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	var blocklistTime int64

	paymentThresholdInRefreshmentSeconds := new(big.Int).Div(testPaymentThreshold, big.NewInt(testRefreshRate)).Uint64()

	f := func(s swarm.Address, t time.Duration, reason string) error {
		if reason != "ghost overdraw" {
			return errInvalidReason
		}
		blocklistTime = int64(t.Seconds())
		return nil
	}

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New(p2pmock.WithBlocklistFunc(f)))
	if err != nil {
		t.Fatal(err)
	}

	ts := int64(1000)
	acc.SetTime(ts)

	peer, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	acc.Connect(peer, true)

	requestPrice := testPaymentThreshold.Uint64()

	debitActionNormal, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	err = debitActionNormal.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitActionNormal.Cleanup()

	// debit ghost balance
	debitActionGhost, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	debitActionGhost.Cleanup()

	// increase shadow reserve
	debitActionShadow, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	_ = debitActionShadow

	if blocklistTime != 0 {
		t.Fatal("unexpected blocklist")
	}

	// ghost overdraft triggering blocklist
	debitAction4, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	debitAction4.Cleanup()

	if blocklistTime != int64(5*paymentThresholdInRefreshmentSeconds) {
		t.Fatalf("unexpected blocklisting time, got %v expected %v", blocklistTime, 5*paymentThresholdInRefreshmentSeconds)
	}
}

func TestAccountingReconnectBeforeAllowed(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	var blocklistTime int64

	paymentThresholdInRefreshmentSeconds := new(big.Int).Div(testPaymentThreshold, big.NewInt(testRefreshRate)).Uint64()

	f := func(s swarm.Address, t time.Duration, reason string) error {
		if reason != "accounting disconnect" {
			return errInvalidReason
		}
		blocklistTime = int64(t.Seconds())
		return nil
	}

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New(p2pmock.WithBlocklistFunc(f)))
	if err != nil {
		t.Fatal(err)
	}

	ts := int64(1000)
	acc.SetTime(ts)

	peer, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	acc.Connect(peer, true)

	requestPrice := testPaymentThreshold.Uint64()

	debitActionNormal, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	err = debitActionNormal.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitActionNormal.Cleanup()

	// debit ghost balance
	debitActionGhost, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	debitActionGhost.Cleanup()

	// increase shadow reserve
	debitActionShadow, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	_ = debitActionShadow

	if blocklistTime != 0 {
		t.Fatal("unexpected blocklist")
	}

	acc.Disconnect(peer)

	if blocklistTime != int64(4*paymentThresholdInRefreshmentSeconds) {
		t.Fatalf("unexpected blocklisting time, got %v expected %v", blocklistTime, 4*paymentThresholdInRefreshmentSeconds)
	}

}

func TestAccountingResetBalanceAfterReconnect(t *testing.T) {
	t.Parallel()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	var blocklistTime int64

	paymentThresholdInRefreshmentSeconds := new(big.Int).Div(testPaymentThreshold, big.NewInt(testRefreshRate)).Uint64()

	f := func(s swarm.Address, t time.Duration, reason string) error {
		if reason != "accounting disconnect" {
			return errInvalidReason
		}
		blocklistTime = int64(t.Seconds())
		return nil
	}

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, testPaymentTolerance, testPaymentEarly, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New(p2pmock.WithBlocklistFunc(f)))
	if err != nil {
		t.Fatal(err)
	}

	ts := int64(1000)
	acc.SetTime(ts)

	peer, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer, true)

	requestPrice := testPaymentThreshold.Uint64()

	debitActionNormal, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	err = debitActionNormal.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitActionNormal.Cleanup()

	// debit ghost balance
	debitActionGhost, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	debitActionGhost.Cleanup()

	// increase shadow reserve
	debitActionShadow, err := acc.PrepareDebit(context.Background(), peer, requestPrice)
	if err != nil {
		t.Fatal(err)
	}
	_ = debitActionShadow

	if blocklistTime != 0 {
		t.Fatal("unexpected blocklist")
	}

	acc.Disconnect(peer)

	if blocklistTime != int64(4*paymentThresholdInRefreshmentSeconds) {
		t.Fatalf("unexpected blocklisting time, got %v expected %v", blocklistTime, 4*paymentThresholdInRefreshmentSeconds)
	}

	acc.Connect(peer, true)

	balance, err := acc.Balance(peer)
	if err != nil {
		t.Fatal(err)
	}

	if balance.Int64() != 0 {
		t.Fatalf("balance for peer %v not as expected got %d, wanted 0", peer.String(), balance)
	}

	surplusBalance, err := acc.SurplusBalance(peer)
	if err != nil {
		t.Fatal(err)
	}

	if surplusBalance.Int64() != 0 {
		t.Fatalf("surplus balance for peer %v not as expected got %d, wanted 0", peer.String(), balance)
	}

}

func testAccountingSettlementGrowingThresholds(t *testing.T, settleFunc func(t *testing.T, acc *accounting.Accounting, peer1Addr swarm.Address, debitRefresh int64), fullNode bool, testPayThreshold *big.Int, testGrowth int64) {
	t.Helper()

	logger := log.Noop

	store := mock.NewStateStore()
	defer store.Close()

	pricing := &pricingMock{}

	acc, err := accounting.NewAccounting(testPaymentThreshold, 0, 0, logger, store, pricing, big.NewInt(testRefreshRate), testLightFactor, p2pmock.New())
	if err != nil {
		t.Fatal(err)
	}
	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = pricing.AnnouncePaymentThreshold(context.Background(), peer1Addr, testPayThreshold)
	if err != nil {
		t.Fatal(err)
	}

	acc.Connect(peer1Addr, fullNode)

	checkPaymentThreshold := new(big.Int).Set(testPayThreshold)

	// Simulate first 18 threshold upgrades
	for j := 0; j < 18; j++ {
		for i := 0; i < 100; i++ {

			// expect no change in threshold while less than 100 seconds worth of refreshment rate was settled
			settleFunc(t, acc, peer1Addr, testGrowth-1)

			if pricing.paymentThreshold.Cmp(checkPaymentThreshold) != 0 {
				t.Fatalf("expected threshold %v got %v", checkPaymentThreshold, pricing.paymentThreshold)
			}

		}

		// Cumulative settled debt is now "checkpoint - 100 + j" ( j < 18 )

		// Expect increase after 100 seconds of refreshment is crossed

		checkPaymentThreshold = new(big.Int).Add(checkPaymentThreshold, big.NewInt(testGrowth))

		// Cross
		settleFunc(t, acc, peer1Addr, 101)

		// Cumulative settled debt is now "checkpoint + j + 1" ( j < 18 )

		// Check increase happened (meaning pricing AnnounceThreshold was called by accounting)
		if pricing.paymentThreshold.Cmp(checkPaymentThreshold) != 0 {
			t.Fatalf("expected threshold %v got %v", checkPaymentThreshold, pricing.paymentThreshold)
		}

	}

	// Simulate first exponential checkpoint

	// Expect no increase for the next 179 seconds of refreshment

	for k := 0; k < 1799; k++ {

		settleFunc(t, acc, peer1Addr, testGrowth)

		// Check threshold have not been updated
		if pricing.paymentThreshold.Cmp(checkPaymentThreshold) != 0 {
			t.Fatalf("expected threshold %v got %v", checkPaymentThreshold, pricing.paymentThreshold)
		}

	}

	// Expect increase after the 1800th second worth of refreshment settled

	checkPaymentThreshold = new(big.Int).Add(checkPaymentThreshold, big.NewInt(testGrowth))

	settleFunc(t, acc, peer1Addr, testGrowth)

	// Check increase happened (meaning pricing AnnounceThreshold was called by accounting)
	if pricing.paymentThreshold.Cmp(checkPaymentThreshold) != 0 {
		t.Fatalf("expected threshold %v got %v", checkPaymentThreshold, pricing.paymentThreshold)
	}

	// Simulate second exponential checkpoint

	// Expect no increase for another 3599 seconds of refreshments

	for k := 0; k < 3599; k++ {

		settleFunc(t, acc, peer1Addr, testGrowth)

		// Check threshold have not been updated
		if pricing.paymentThreshold.Cmp(checkPaymentThreshold) != 0 {
			t.Fatalf("expected threshold %v got %v", checkPaymentThreshold, pricing.paymentThreshold)
		}

	}

	// Expect increase after the 360th second worth of refreshment settled

	checkPaymentThreshold = new(big.Int).Add(checkPaymentThreshold, big.NewInt(testGrowth))

	settleFunc(t, acc, peer1Addr, testGrowth)

	// Check increase happened (meaning pricing AnnounceThreshold was called by accounting)
	if pricing.paymentThreshold.Cmp(checkPaymentThreshold) != 0 {
		t.Fatalf("expected threshold %v got %v", checkPaymentThreshold, pricing.paymentThreshold)
	}

}

func TestAccountingRefreshGrowingThresholds(t *testing.T) {
	t.Parallel()

	testAccountingSettlementGrowingThresholds(t, debitAndRefresh, true, testPaymentThreshold, testRefreshRate)

}

func TestAccountingRefreshGrowingThresholdsLight(t *testing.T) {
	t.Parallel()

	lightPaymentThresholdDefault := new(big.Int).Div(testPaymentThreshold, big.NewInt(testLightFactor))
	lightRefreshRate := testRefreshRate / testLightFactor

	testAccountingSettlementGrowingThresholds(t, debitAndRefresh, false, lightPaymentThresholdDefault, lightRefreshRate)

}

func TestAccountingSwapGrowingThresholds(t *testing.T) {
	t.Parallel()

	testAccountingSettlementGrowingThresholds(t, debitAndReceivePayment, true, testPaymentThreshold, testRefreshRate)

}

func TestAccountingSwapGrowingThresholdsLight(t *testing.T) {
	t.Parallel()

	lightPaymentThresholdDefault := new(big.Int).Div(testPaymentThreshold, big.NewInt(testLightFactor))
	lightRefreshRate := testRefreshRate / testLightFactor

	testAccountingSettlementGrowingThresholds(t, debitAndReceivePayment, false, lightPaymentThresholdDefault, lightRefreshRate)

}

func debitAndRefresh(t *testing.T, acc *accounting.Accounting, peer1Addr swarm.Address, debitRefresh int64) {
	t.Helper()

	// Create debt
	debitAction, err := acc.PrepareDebit(context.Background(), peer1Addr, uint64(debitRefresh))
	if err != nil {
		t.Fatal(err)
	}
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	// Refresh
	err = acc.NotifyRefreshmentReceived(peer1Addr, big.NewInt(debitRefresh), time.Now().Unix())
	if err != nil {
		t.Fatalf("unexpected error from NotifyRefreshmentReceived: %v", err)
	}

}

func debitAndReceivePayment(t *testing.T, acc *accounting.Accounting, peer1Addr swarm.Address, debitRefresh int64) {
	t.Helper()

	// Create debt
	debitAction, err := acc.PrepareDebit(context.Background(), peer1Addr, uint64(debitRefresh))
	if err != nil {
		t.Fatal(err)
	}
	err = debitAction.Apply()
	if err != nil {
		t.Fatal(err)
	}
	debitAction.Cleanup()

	// Refresh
	err = acc.NotifyPaymentReceived(peer1Addr, big.NewInt(debitRefresh))
	if err != nil {
		t.Fatalf("unexpected error from NotifyRefreshmentReceived: %v", err)
	}

}
