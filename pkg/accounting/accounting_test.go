package accounting_test

import (
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"io/ioutil"
	"testing"
)

const (
	testPrice = 10
)

func TestAccountingAddBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	accounting := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: 10000,
		PaymentThreshold:    1000,
		Logger:              logger,
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	peer2Addr, err := swarm.ParseHexAddress("00112244")
	if err != nil {
		t.Fatal(err)
	}

	err = accounting.Add(peer1Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	peer1Balance := accounting.GetPeerBalance(peer1Addr)
	if peer1Balance != testPrice {
		t.Fatalf("peer1Balance not equal to price. got %d, wanted %d", peer1Balance, testPrice)
	}

	err = accounting.Add(peer2Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	peer2Balance := accounting.GetPeerBalance(peer2Addr)
	if peer2Balance != testPrice {
		t.Fatalf("peer1Balance not equal to price. got %d, wanted %d", peer1Balance, testPrice)
	}

	err = accounting.Add(peer1Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	peer1Balance = accounting.GetPeerBalance(peer1Addr)
	if peer1Balance != 2*testPrice {
		t.Fatalf("peer1Balance not equal to price. got %d, wanted %d", peer1Balance, 2*testPrice)
	}
}
