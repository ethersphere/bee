// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting_test

import (
	"errors"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"io/ioutil"
	"testing"
)

const (
	testDisconnectThreshold = 10000
	testPrice               = uint64(10)
)

// Booking represents an accounting action and the expected result afterwards
type Booking struct {
	peer            swarm.Address
	price           int64 // Credit if <0, Debit otherwise
	expectedBalance int64
}

// TestAccountingAddBalance does several accounting actions and verifies the balance after each steep
func TestAccountingAddBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		Logger:              logger,
		Store:               store,
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	peer2Addr, err := swarm.ParseHexAddress("00112244")
	if err != nil {
		t.Fatal(err)
	}

	bookings := []Booking{
		{peer: peer1Addr, price: 100, expectedBalance: 100},
		{peer: peer2Addr, price: 200, expectedBalance: 200},
		{peer: peer1Addr, price: 300, expectedBalance: 400},
		{peer: peer1Addr, price: -100, expectedBalance: 300},
		{peer: peer2Addr, price: -1000, expectedBalance: -800},
	}

	for i, booking := range bookings {
		if booking.price < 0 {
			err = acc.Reserve(booking.peer, uint64(booking.price))
			if err != nil {
				t.Fatal(err)
			}
			err = acc.Credit(booking.peer, uint64(-booking.price))
			if err != nil {
				t.Fatal(err)
			}
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

		if booking.price < 0 {
			acc.Release(booking.peer, uint64(booking.price))
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

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		Logger:              logger,
		Store:               store,
	})

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

	acc = accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		Logger:              logger,
		Store:               store,
	})

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

// TestAccountingReserve tests that reserve returns an error if the disconnect threshold would be exceeded
func TestAccountingReserve(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		Logger:              logger,
		Store:               store,
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Reserve(peer1Addr, testDisconnectThreshold-100)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Reserve(peer1Addr, 101)
	if err == nil {
		t.Fatal("expected error from reserve")
	}
}

// TestAccountingDisconnect tests that exceeding the disconnect threshold with Debit returns a p2p.DisconnectError
func TestAccountingDisconnect(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		Logger:              logger,
		Store:               store,
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Debit(peer1Addr, testDisconnectThreshold)
	if err == nil {
		t.Fatal("expected Add to return error")
	}

	var e *p2p.DisconnectError
	if !errors.As(err, &e) {
		t.Fatalf("expected DisconnectError, got %v", err)
	}
}
