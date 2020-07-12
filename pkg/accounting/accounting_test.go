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
	testPaymentThreshold    = 1000
	testPrice               = uint64(10)
)

type Booking struct {
	peer            swarm.Address
	price           int64
	expectedBalance int64
}

func TestAccountingAddBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		PaymentThreshold:    testPaymentThreshold,
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

func TestAccountingAdd_persistentBalances(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		PaymentThreshold:    testPaymentThreshold,
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
		PaymentThreshold:    testPaymentThreshold,
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

func TestAccountingReserve(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		PaymentThreshold:    testPaymentThreshold,
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

func TestAccountingDisconnect(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		PaymentThreshold:    testPaymentThreshold,
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
