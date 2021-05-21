// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage/listener"
)

var hash common.Hash = common.HexToHash("ff6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")
var addr common.Address = common.HexToAddress("abcdef")

var postageStampAddress common.Address = common.HexToAddress("eeee")

func TestListener(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	blockNumber := uint64(500)
	timeout := 5 * time.Second
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
				c.toLog(496),
			),
		)
		l := listener.New(logger, mf, postageStampAddress, 1)
		l.Listen(0, ev)

		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-evC:
			e.(createArgs).compare(t, c) // event args should be equal
		case <-time.After(timeout):
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
				topup.toLog(496),
			),
		)
		l := listener.New(logger, mf, postageStampAddress, 1)
		l.Listen(0, ev)

		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-evC:
			e.(topupArgs).compare(t, topup) // event args should be equal
		case <-time.After(timeout):
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
				depthIncrease.toLog(496),
			),
		)
		l := listener.New(logger, mf, postageStampAddress, 1)
		l.Listen(0, ev)

		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-evC:
			e.(depthArgs).compare(t, depthIncrease) // event args should be equal
		case <-time.After(timeout):
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
				priceUpdate.toLog(496),
			),
		)
		l := listener.New(logger, mf, postageStampAddress, 1)
		l.Listen(0, ev)
		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-evC:
			e.(priceArgs).compare(t, priceUpdate) // event args should be equal
		case <-time.After(timeout):
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
				c.toLog(495),
				topup.toLog(496),
				depthIncrease.toLog(497),
				priceUpdate.toLog(498),
			),
			WithBlockNumber(blockNumber),
		)
		l := listener.New(logger, mf, postageStampAddress, 1)
		l.Listen(0, ev)

		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, 495) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-evC:
			e.(createArgs).compare(t, c) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}
		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, 496) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}
		select {
		case e := <-evC:
			e.(topupArgs).compare(t, topup) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}
		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, 497) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-evC:
			e.(depthArgs).compare(t, depthIncrease) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}
		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, 498) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-evC:
			e.(priceArgs).compare(t, priceUpdate)
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}

		select {
		case e := <-evC:
			e.(blockNumberCall).compare(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
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

func (u *updater) Create(id, owner []byte, normalisedAmount *big.Int, depth uint8) error {
	u.eventC <- createArgs{
		id:               id,
		owner:            owner,
		normalisedAmount: normalisedAmount,
		depth:            depth,
	}
	return nil
}

func (u *updater) TopUp(id []byte, normalisedBalance *big.Int) error {
	u.eventC <- topupArgs{
		id:                id,
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

func (u *updater) UpdateBlockNumber(blockNumber uint64) error {
	u.eventC <- blockNumberCall{blockNumber: blockNumber}
	return nil
}

func (u *updater) Start(_ uint64) (<-chan struct{}, error) { return nil, nil }
func (u *updater) TransactionStart() error                 { return nil }
func (u *updater) TransactionEnd() error                   { return nil }

type mockFilterer struct {
	filterLogEvents    []types.Log
	subscriptionEvents []types.Log
	sub                *sub
	blockNumber        uint64
}

func newMockFilterer(opts ...Option) *mockFilterer {
	mock := &mockFilterer{
		blockNumber: uint64(listener.TailSize), // use the tailSize as blockNumber by default to ensure at least block 0 is ready
	}
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

func WithBlockNumber(blockNumber uint64) Option {
	return optionFunc(func(s *mockFilterer) {
		s.blockNumber = blockNumber
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

func (m *mockFilterer) BlockNumber(context.Context) (uint64, error) {
	return m.blockNumber, nil
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
	if c.normalisedAmount.Cmp(want.normalisedAmount) != 0 {
		t.Fatalf("normalised amount mismatch. got %v want %v", c.normalisedAmount.String(), want.normalisedAmount.String())
	}
}

func (c createArgs) toLog(blockNumber uint64) types.Log {
	b, err := listener.PostageStampABI.Events["BatchCreated"].Inputs.NonIndexed().Pack(c.amount, c.normalisedAmount, common.BytesToAddress(c.owner), c.depth)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{listener.BatchCreatedTopic, common.BytesToHash(c.id)}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

type topupArgs struct {
	id                []byte
	amount            *big.Int
	normalisedBalance *big.Int
}

func (ta topupArgs) compare(t *testing.T, want topupArgs) {
	t.Helper()
	if !bytes.Equal(ta.id, want.id) {
		t.Fatalf("id mismatch. got %v want %v", ta.id, want.id)
	}
	if ta.normalisedBalance.Cmp(want.normalisedBalance) != 0 {
		t.Fatalf("normalised balance mismatch. got %v want %v", ta.normalisedBalance.String(), want.normalisedBalance.String())
	}
}

func (ta topupArgs) toLog(blockNumber uint64) types.Log {
	b, err := listener.PostageStampABI.Events["BatchTopUp"].Inputs.NonIndexed().Pack(ta.amount, ta.normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{listener.BatchTopupTopic, common.BytesToHash(ta.id)}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

type depthArgs struct {
	id                []byte
	depth             uint8
	normalisedBalance *big.Int
}

func (d depthArgs) compare(t *testing.T, want depthArgs) {
	t.Helper()
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

func (d depthArgs) toLog(blockNumber uint64) types.Log {
	b, err := listener.PostageStampABI.Events["BatchDepthIncrease"].Inputs.NonIndexed().Pack(d.depth, d.normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{listener.BatchDepthIncreaseTopic, common.BytesToHash(d.id)}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

type priceArgs struct {
	price *big.Int
}

func (p priceArgs) compare(t *testing.T, want priceArgs) {
	t.Helper()
	if p.price.Cmp(want.price) != 0 {
		t.Fatalf("price mismatch. got %s want %s", p.price.String(), want.price.String())
	}
}

func (p priceArgs) toLog(blockNumber uint64) types.Log {
	b, err := listener.PostageStampABI.Events["PriceUpdate"].Inputs.NonIndexed().Pack(p.price)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{listener.PriceUpdateTopic},
	}
}

type blockNumberCall struct {
	blockNumber uint64
}

func (b blockNumberCall) compare(t *testing.T, want uint64) {
	t.Helper()
	if b.blockNumber != want {
		t.Fatalf("blockNumber mismatch. got %d want %d", b.blockNumber, want)
	}
}

type Option interface {
	apply(*mockFilterer)
}

type optionFunc func(*mockFilterer)

func (f optionFunc) apply(r *mockFilterer) { f(r) }
