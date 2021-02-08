package cac_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestCac(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)
	foo := "foo"

	c, err := cac.New([]byte(foo))
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}
}
