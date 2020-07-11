package accounting_test

import (
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"io/ioutil"
	"testing"
)

const (
	testDisconnectThreshold = 10000
	testPaymentThreshold    = 1000
	testPrice               = 10
)

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

	err = acc.Add(peer1Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	peer1Balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if peer1Balance != testPrice {
		t.Fatalf("peer1Balance not equal to price. got %d, wanted %d", peer1Balance, testPrice)
	}

	err = acc.Add(peer2Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	peer2Balance, err := acc.Balance(peer2Addr)
	if err != nil {
		t.Fatal(err)
	}
	if peer2Balance != testPrice {
		t.Fatalf("peer1Balance not equal to price. got %d, wanted %d", peer1Balance, testPrice)
	}

	err = acc.Add(peer1Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	peer1Balance, err = acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}
	if peer1Balance != 2*testPrice {
		t.Fatalf("peer1Balance not equal to price. got %d, wanted %d", peer1Balance, 2*testPrice)
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

	err = acc.Add(peer1Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Add(peer2Addr, 2*testPrice)
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

	if peer1Balance != testPrice {
		t.Fatalf("peer1Balance not loaded correctly. got %d, wanted %d", peer1Balance, testPrice)
	}

	peer2Balance, err := acc.Balance(peer2Addr)
	if err != nil {
		t.Fatal(err)
	}

	if peer2Balance != 2*testPrice {
		t.Fatalf("peer2Balance not loaded correctly. got %d, wanted %d", peer2Balance, 2*testPrice)
	}

}
