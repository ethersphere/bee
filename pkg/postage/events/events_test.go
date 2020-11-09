package events_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
)

func TestSync(t *testing.T) {

}

type testEvents struct {
	in    <-chan string
	out   chan string
	err   chan error
	block uint64
}

func (te *testEvents) Create(id []byte, owner []byte, amount *big.Int, depth uint8) error {
	ev := fmt.Sprintf("%d:create(%x,%x,%s,%d)", te.block, id, owner, amount, depth)
	te.out <- ev
	return nil
}

func (te *testEvents) TopUp(id []byte, amount *big.Int) error {
	ev := fmt.Sprintf("%d:topup(%x,%s)", te.block, id, amount)
	te.out <- ev
	return nil
}

func (te *testEvents) UpdateDepth(id []byte, depth uint8) error {
	ev := fmt.Sprintf("%d:depth(%x,%d)", te.block, id, depth)
	te.out <- ev
	return nil
}

func (te *testEvents) UpdatePrice(price *big.Int) error {
	ev := fmt.Sprintf("%d:price(%s)", te.block, price)
	te.out <- ev
	return nil
}

//
func (te *testEvents) LastBlock() uint64 {
	return te.block
}
func (te *testEvents) Update(block uint64, ev postage.Event) error {
	te.block = block
	return ev.Update(te)
}
