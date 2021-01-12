package listener_test

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/postage/listener"
)

var hash common.Hash = common.HexToHash("ff6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")
var addr common.Address = common.HexToAddress("abcdef")
var (
	postageStampABI    = parseABI(listener.Abi)
	createdTopic       = postageStampABI.Events["BatchCreated"].ID
	topupTopic         = postageStampABI.Events["BatchTopUp"].ID
	depthIncreaseTopic = postageStampABI.Events["BatchDepthIncrease"].ID
)

func TestListener(t *testing.T) {
	// test that when the listener gets a certain event
	// then we would like to assert the appropriate EventUpdater method was called
	t.Run("create event", func(t *testing.T) {
		c := createArgs{
			id:               hash[:],
			owner:            addr[:],
			amount:           big.NewInt(42),
			normalisedAmount: big.NewInt(43),
			depth:            100,
		}

		topup := topupArgs{
			id:                hash[:],
			amount:            big.NewInt(0),
			normalisedBalance: big.NewInt(1),
		}

		depthIncrease := depthArgs{
			id:                hash[:],
			depth:             200,
			normalisedBalance: big.NewInt(2),
		}

		ev, evC := newEventUpdaterMock()
		mf := newMockFilterer(
			newCreateEvent(common.BytesToHash(c.id), c.amount, c.normalisedAmount, c.depth),
			newTopupEvent(common.BytesToHash(topup.id), topup.amount, topup.normalisedBalance),
			newDepthIncreaseEvent(common.BytesToHash(depthIncrease.id), depthIncrease.depth, depthIncrease.normalisedBalance),
		)
		listener := listener.New(mf)
		listener.Listen(0, ev)

		select {
		case e := <-evC:
			e.(createArgs).compare(t, c) // event args should be equal
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for event")
		}

		select {
		case e := <-evC:
			e.(topupArgs).compare(t, topup) // event args should be equal
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for event")
		}

		select {
		case e := <-evC:
			e.(depthArgs).compare(t, depthIncrease) // event args should be equal
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

}

func newEventUpdaterMock() (*updater, chan interface{}) {
	c := make(chan interface{})
	return &updater{
		eventC: c,
	}, c
}

type updater struct {
	eventC chan interface{}
}

func (u *updater) Create(id []byte, owner []byte, amount *big.Int, normalisedAmount *big.Int, depth uint8) error {
	u.eventC <- createArgs{
		id:               id,
		owner:            owner,
		amount:           amount,
		normalisedAmount: normalisedAmount,
		depth:            depth,
	}
	return nil
}

func (u *updater) TopUp(id []byte, amount *big.Int, normalisedBalance *big.Int) error {
	u.eventC <- topupArgs{
		id:                id,
		amount:            amount,
		normalisedBalance: normalisedBalance,
	}
	return nil
}

func (u *updater) UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int) error {
	u.eventC <- depthArgs{
		id:                id,
		depth:             depth,
		normalisedBalance: normalisedBalance,
	}
	return nil
}

func (u *updater) UpdatePrice(price *big.Int) error {
	u.eventC <- priceArgs{price}
	return nil
}

type mockFilterer struct {
	events []types.Log
	sub    *sub
}

func newMockFilterer(logs ...types.Log) *mockFilterer {
	return &mockFilterer{
		events: logs,
	}
}

func (m *mockFilterer) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return m.events, nil
}

func (m *mockFilterer) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	go func() {
		for _, ev := range m.events {
			ch <- ev
		}
	}()
	s := newSub()
	return s, nil
}

func (m *mockFilterer) Close() {
	close(m.sub.c)
}

func (m *mockFilterer) BlockHeight(context.Context) (uint64, error) {
	return 0, nil
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for postage contract: %v", err))
	}
	return cabi
}

func newCreateEvent(batchID common.Hash, totalAmount *big.Int, normalisedBalance *big.Int, depth uint8) types.Log {
	b, err := postageStampABI.Events["BatchCreated"].Inputs.NonIndexed().Pack(totalAmount, normalisedBalance, addr, depth)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:   b,
		Topics: []common.Hash{createdTopic, batchID}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

func newTopupEvent(batchID common.Hash, topupAmount *big.Int, normalisedBalance *big.Int) types.Log {
	b, err := postageStampABI.Events["BatchTopUp"].Inputs.NonIndexed().Pack(topupAmount, normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:   b,
		Topics: []common.Hash{topupTopic, batchID}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

func newDepthIncreaseEvent(batchID common.Hash, newDepth uint8, normalisedBalance *big.Int) types.Log {
	b, err := postageStampABI.Events["BatchDepthIncrease"].Inputs.NonIndexed().Pack(newDepth, normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:   b,
		Topics: []common.Hash{depthIncreaseTopic, batchID}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

type sub struct {
	c chan error
}

func newSub() *sub {
	return &sub{
		c: make(chan error),
	}
}

func (s *sub) Unsubscribe() {}
func (s *sub) Err() <-chan error {
	return s.c
}

type createArgs struct {
	id               []byte
	owner            []byte
	amount           *big.Int
	normalisedAmount *big.Int
	depth            uint8
}

func (c createArgs) compare(t *testing.T, want createArgs) {
	if !bytes.Equal(c.id, want.id) {
		t.Fatalf("id mismatch. got %v want %v", c.id, want.id)
	}
	if !bytes.Equal(c.owner, want.owner) {
		t.Fatalf("owner mismatch. got %v want %v", c.owner, want.owner)
	}
	if c.amount.Cmp(want.amount) != 0 {
		t.Fatalf("amount mismatch. got %v want %v", c.amount.String(), want.amount.String())
	}
	if c.normalisedAmount.Cmp(want.normalisedAmount) != 0 {
		t.Fatalf("normalised amount mismatch. got %v want %v", c.normalisedAmount.String(), want.normalisedAmount.String())
	}
}

type topupArgs struct {
	id                []byte
	amount            *big.Int
	normalisedBalance *big.Int
}

func (ta topupArgs) compare(t *testing.T, want topupArgs) {
	if !bytes.Equal(ta.id, want.id) {
		t.Fatalf("id mismatch. got %v want %v", ta.id, want.id)
	}
	if ta.amount.Cmp(want.amount) != 0 {
		t.Fatalf("amount mismatch. got %s want %s", ta.amount.String(), want.amount.String())
	}
	if ta.normalisedBalance.Cmp(want.normalisedBalance) != 0 {
		t.Fatalf("normalised balance mismatch. got %v want %v", ta.normalisedBalance.String(), want.normalisedBalance.String())
	}
}

type depthArgs struct {
	id                []byte
	depth             uint8
	normalisedBalance *big.Int
}

func (d depthArgs) compare(t *testing.T, want depthArgs) {
	if !bytes.Equal(d.id, want.id) {
		t.Fatalf("id mismatch. got %v want %v", d.id, want.id)
	}
	if d.depth != want.depth {
		t.Fatalf("depth mismatch. got %d want %d", d.depth, want.depth)
	}
	if d.normalisedBalance.Cmp(want.normalisedBalance) != 0 {
		t.Fatalf("normalised balance mismatch. got %v want %v", d.normalisedBalance.String(), want.normalisedBalance.String())
	}
}

type priceArgs struct {
	price *big.Int
}

func (p priceArgs) compare(t *testing.T, want priceArgs) {
	if p.price.Cmp(want.price) != 0 {
		t.Fatalf("price mismatch. got %s want %s", p.price.String(), want.price.String())
	}
}
