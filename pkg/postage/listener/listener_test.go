package listener_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage/listener"
)

var hash common.Hash = common.HexToHash("ff6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")
var addr common.Address = common.HexToAddress("abcdef")

var postageStampAddress common.Address = common.HexToAddress("eeee")
var priceOracleAddress common.Address = common.HexToAddress("eeef")

var (
	postageStampABI    = parseABI(listener.PostageStampABI)
	priceOracleABI     = parseABI(listener.PriceOracleABI)
	createdTopic       = postageStampABI.Events["BatchCreated"].ID
	topupTopic         = postageStampABI.Events["BatchTopUp"].ID
	depthIncreaseTopic = postageStampABI.Events["BatchDepthIncrease"].ID
	priceUpdateTopic   = priceOracleABI.Events["PriceUpdate"].ID
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

		ev, evC := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				c.toLog(),
			),
		)
		listener := listener.New(logging.New(ioutil.Discard, 0), mf, postageStampAddress, priceOracleAddress)
		err := listener.Listen(0, ev)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case e := <-evC:
			e.(createArgs).compare(t, c) // event args should be equal
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("topup event", func(t *testing.T) {
		topup := topupArgs{
			id:                hash[:],
			amount:            big.NewInt(0),
			normalisedBalance: big.NewInt(1),
		}

		ev, evC := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				topup.toLog(),
			),
		)
		listener := listener.New(logging.New(ioutil.Discard, 0), mf, postageStampAddress, priceOracleAddress)
		err := listener.Listen(0, ev)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case e := <-evC:
			e.(topupArgs).compare(t, topup) // event args should be equal
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("depthIncrease event", func(t *testing.T) {
		depthIncrease := depthArgs{
			id:                hash[:],
			depth:             200,
			normalisedBalance: big.NewInt(2),
		}

		ev, evC := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				depthIncrease.toLog(),
			),
		)
		listener := listener.New(logging.New(ioutil.Discard, 0), mf, postageStampAddress, priceOracleAddress)
		err := listener.Listen(0, ev)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case e := <-evC:
			e.(depthArgs).compare(t, depthIncrease) // event args should be equal
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("priceUpdate event", func(t *testing.T) {
		priceUpdate := priceArgs{
			price: big.NewInt(500),
		}

		ev, evC := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				priceUpdate.toLog(),
			),
		)
		listener := listener.New(logging.New(ioutil.Discard, 0), mf, postageStampAddress, priceOracleAddress)
		err := listener.Listen(0, ev)
		if err != nil {
			t.Fatal(err)
		}

		select {
		case e := <-evC:
			e.(priceArgs).compare(t, priceUpdate) // event args should be equal
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("multiple events", func(t *testing.T) {
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

		priceUpdate := priceArgs{
			price: big.NewInt(500),
		}

		ev, evC := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				c.toLog(),
				topup.toLog(),
				depthIncrease.toLog(),
				priceUpdate.toLog(),
			),
		)
		listener := listener.New(logging.New(ioutil.Discard, 0), mf, postageStampAddress, priceOracleAddress)
		err := listener.Listen(0, ev)
		if err != nil {
			t.Fatal(err)
		}

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

		select {
		case e := <-evC:
			e.(priceArgs).compare(t, priceUpdate)
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

func (u *updater) Create(id, owner []byte, amount, normalisedAmount *big.Int, depth uint8) error {
	u.eventC <- createArgs{
		id:               id,
		owner:            owner,
		amount:           amount,
		normalisedAmount: normalisedAmount,
		depth:            depth,
	}
	return nil
}

func (u *updater) TopUp(id []byte, amount, normalisedBalance *big.Int) error {
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
	filterLogEvents    []types.Log
	subscriptionEvents []types.Log
	sub                *sub
}

func newMockFilterer(opts ...Option) *mockFilterer {
	mock := new(mockFilterer)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func WithFilterLogEvents(events ...types.Log) Option {
	return optionFunc(func(s *mockFilterer) {
		s.filterLogEvents = events
	})
}

func (m *mockFilterer) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return m.filterLogEvents, nil
}

func (m *mockFilterer) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	go func() {
		for _, ev := range m.subscriptionEvents {
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

func (c createArgs) toLog() types.Log {
	b, err := postageStampABI.Events["BatchCreated"].Inputs.NonIndexed().Pack(c.amount, c.normalisedAmount, common.BytesToAddress(c.owner), c.depth)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:   b,
		Topics: []common.Hash{createdTopic, common.BytesToHash(c.id)}, // 1st item is the function sig digest, 2nd is always the batch id
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

func (ta topupArgs) toLog() types.Log {
	b, err := postageStampABI.Events["BatchTopUp"].Inputs.NonIndexed().Pack(ta.amount, ta.normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:   b,
		Topics: []common.Hash{topupTopic, common.BytesToHash(ta.id)}, // 1st item is the function sig digest, 2nd is always the batch id
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

func (d depthArgs) toLog() types.Log {
	b, err := postageStampABI.Events["BatchDepthIncrease"].Inputs.NonIndexed().Pack(d.depth, d.normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:   b,
		Topics: []common.Hash{depthIncreaseTopic, common.BytesToHash(d.id)}, // 1st item is the function sig digest, 2nd is always the batch id
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

func (p priceArgs) toLog() types.Log {
	b, err := priceOracleABI.Events["PriceUpdate"].Inputs.NonIndexed().Pack(p.price)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:   b,
		Topics: []common.Hash{priceUpdateTopic},
	}
}

type Option interface {
	apply(*mockFilterer)
}

type optionFunc func(*mockFilterer)

func (f optionFunc) apply(r *mockFilterer) { f(r) }
