// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

var (
	hash = common.HexToHash("ff6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")
	addr = common.HexToAddress("abcdef")
)

var (
	postageStampContractAddress = common.HexToAddress("eeee")
	postageStampContractABI     = abiutil.MustParseABI(chaincfg.Testnet.PostageStampABI)
)

const (
	stallingTimeout = 5 * time.Second
	backoffTime     = 5 * time.Second
)

func toBatchBlock(block uint64) uint64 {
	return (block / listener.BatchFactor) * listener.BatchFactor
}

func TestListener(t *testing.T) {
	t.Parallel()

	const (
		blockNumber = uint64(500)
		timeout     = 5 * time.Second
	)

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

		ev := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				c.toLog(496),
			),
		)

		l := listener.New(
			nil,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			stallingTimeout,
			backoffTime,
		)
		testutil.CleanupCloser(t, l)
		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-ev.eventC:
			e.(createArgs).compareF(t, c) // event args should be equal
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

		ev := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				topup.toLog(496),
			),
		)
		l := listener.New(
			nil,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			stallingTimeout,
			backoffTime,
		)
		testutil.CleanupCloser(t, l)
		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-ev.eventC:
			e.(topupArgs).compareF(t, topup) // event args should be equal
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

		ev := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				depthIncrease.toLog(496),
			),
		)
		l := listener.New(
			nil,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			stallingTimeout,
			backoffTime,
		)
		testutil.CleanupCloser(t, l)

		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-ev.eventC:
			e.(depthArgs).compareF(t, depthIncrease) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("priceUpdate event", func(t *testing.T) {
		priceUpdate := priceArgs{
			price: big.NewInt(500),
		}

		ev := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				priceUpdate.toLog(496),
			),
		)
		l := listener.New(
			nil,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			stallingTimeout,
			backoffTime,
		)
		testutil.CleanupCloser(t, l)
		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, blockNumber-uint64(listener.TailSize)) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-ev.eventC:
			e.(priceArgs).compareF(t, priceUpdate) // event args should be equal
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

		ev := newEventUpdaterMock()
		mf := newMockFilterer(
			WithFilterLogEvents(
				c.toLog(495),
				topup.toLog(496),
				depthIncrease.toLog(497),
				priceUpdate.toLog(498),
			),
			WithBlockNumber(blockNumber),
		)
		l := listener.New(
			nil,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			stallingTimeout,
			backoffTime,
		)
		testutil.CleanupCloser(t, l)
		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, 495) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-ev.eventC:
			e.(createArgs).compareF(t, c) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}
		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, 496) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}
		select {
		case e := <-ev.eventC:
			e.(topupArgs).compareF(t, topup) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}
		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, 497) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-ev.eventC:
			e.(depthArgs).compareF(t, depthIncrease) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}
		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, 498) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case e := <-ev.eventC:
			e.(priceArgs).compareF(t, priceUpdate)
		case <-time.After(timeout):
			t.Fatal("timed out waiting for event")
		}

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, toBatchBlock(blockNumber-uint64(listener.TailSize))) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}
	})

	t.Run("do not shutdown on error event", func(t *testing.T) {
		blockNumber := uint64(500)
		ev := newEventUpdaterMock()
		mf := newMockFilterer(
			WithBlockNumberErrorOnce(errors.New("dummy error"), blockNumber),
		)

		l := listener.New(
			nil,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			stallingTimeout,
			0,
		)
		testutil.CleanupCloser(t, l)
		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, toBatchBlock(blockNumber-uint64(listener.TailSize))) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}
	})

	t.Run("shutdown on stalling", func(t *testing.T) {
		ev := newEventUpdaterMock()
		mf := newMockFilterer(
			WithBlockNumberError(errors.New("dummy error")),
		)
		c := syncutil.NewSignaler()

		l := listener.New(
			c,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			50*time.Millisecond,
			0,
		)
		testutil.CleanupCloser(t, l)
		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case <-c.C:
		case <-time.After(5 * time.Second):
			t.Fatal("expected shutdown call by now")
		}
	})

	t.Run("shutdown on processing error", func(t *testing.T) {
		blockNumber := uint64(500)
		ev := newEventUpdaterMockWithBlockNumberUpdateError(errors.New("err"))
		mf := newMockFilterer(
			WithBlockNumber(blockNumber),
		)
		c := syncutil.NewSignaler()

		l := listener.New(c,
			log.Noop,
			mf,
			postageStampContractAddress,
			postageStampContractABI,
			1,
			stallingTimeout,
			backoffTime,
		)
		testutil.CleanupCloser(t, l)
		<-l.Listen(context.Background(), 0, ev, nil)

		select {
		case e := <-ev.eventC:
			e.(blockNumberCall).compareF(t, toBatchBlock(blockNumber-uint64(listener.TailSize))) // event args should be equal
		case <-time.After(timeout):
			t.Fatal("timed out waiting for block number update")
		}

		select {
		case <-c.C:
		case <-time.After(time.Second * 5):
			t.Fatal("expected shutdown call by now")
		}
	})
}

func TestListenerBatchState(t *testing.T) {
	t.Parallel()

	ev := newEventUpdaterMock()
	mf := newMockFilterer()

	create := createArgs{
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

	snapshot := &postage.ChainSnapshot{
		Events: []types.Log{
			create.toLog(496),
			topup.toLog(497),
			depthIncrease.toLog(498),
			priceUpdate.toLog(499),
		},
		FirstBlockNumber: 496,
		LastBlockNumber:  499,
		Timestamp:        time.Now().Unix(),
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	errs := make(chan error)
	noOfEvents := 0

	go func() {
		for {
			select {
			case <-stop:
				return
			case e := <-ev.eventC:
				noOfEvents++
				switch ev := e.(type) {
				case blockNumberCall:
					if ev.blockNumber < 497 && ev.blockNumber > 500 {
						errs <- fmt.Errorf("invalid blocknumber call %d", ev.blockNumber)
						return
					}
					if ev.blockNumber == 500 {
						close(done)
						return
					}
				case createArgs:
					if err := ev.compare(create); err != nil {
						errs <- err
						return
					}
				case topupArgs:
					if err := ev.compare(topup); err != nil {
						errs <- err
						return
					}
				case depthArgs:
					if err := ev.compare(depthIncrease); err != nil {
						errs <- err
						return
					}
				case priceArgs:
					if err := ev.compare(priceUpdate); err != nil {
						errs <- err
						return
					}
				}
			}
		}
	}()

	l := listener.New(
		nil,
		log.Noop,
		mf,
		postageStampContractAddress,
		postageStampContractABI,
		1,
		stallingTimeout,
		backoffTime,
	)
	testutil.CleanupCloser(t, l)
	l.Listen(context.Background(), snapshot.LastBlockNumber+1, ev, snapshot)

	defer close(stop)

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timedout waiting for events to be processed", noOfEvents)
	case err := <-errs:
		t.Fatal(err)
	case <-done:
		if noOfEvents != 9 {
			t.Fatal("invalid count of events on completion", noOfEvents)
		}
	}
}

func newEventUpdaterMock() *updater {
	return &updater{
		eventC: make(chan interface{}, 1),
	}
}

func newEventUpdaterMockWithBlockNumberUpdateError(err error) *updater {
	return &updater{
		eventC:                 make(chan interface{}, 1),
		blockNumberUpdateError: err,
	}
}

type updater struct {
	eventC                 chan interface{}
	blockNumberUpdateError error
}

func (u *updater) Create(id, owner []byte, amount, normalisedAmount *big.Int, depth, bucketDepth uint8, immutable bool, _ common.Hash) error {
	u.eventC <- createArgs{
		id:               id,
		owner:            owner,
		amount:           amount,
		normalisedAmount: normalisedAmount,
		bucketDepth:      bucketDepth,
		depth:            depth,
		immutable:        immutable,
	}
	return nil
}

func (u *updater) GetSyncStatus() (bool, error) { return true, nil }

func (u *updater) TopUp(id []byte, amount, normalisedBalance *big.Int, _ common.Hash) error {
	u.eventC <- topupArgs{
		id:                id,
		normalisedBalance: normalisedBalance,
		amount:            amount,
	}
	return nil
}

func (u *updater) UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int, _ common.Hash) error {
	u.eventC <- depthArgs{
		id:                id,
		depth:             depth,
		normalisedBalance: normalisedBalance,
	}
	return nil
}

func (u *updater) UpdatePrice(price *big.Int, _ common.Hash) error {
	u.eventC <- priceArgs{price}
	return nil
}

func (u *updater) UpdateBlockNumber(blockNumber uint64) error {
	u.eventC <- blockNumberCall{blockNumber: blockNumber}
	return u.blockNumberUpdateError
}

func (u *updater) Start(ctx context.Context, bno uint64, cs *postage.ChainSnapshot) error {
	return nil
}

func (u *updater) TransactionStart() error { return nil }
func (u *updater) TransactionEnd() error   { return nil }

type mockFilterer struct {
	filterLogEvents      []types.Log
	subscriptionEvents   []types.Log
	sub                  *sub
	blockNumber          uint64
	blockNumberErrorOnce error
	blockNumberError     error
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

func WithBlockNumberError(err error) Option {
	return optionFunc(func(s *mockFilterer) {
		s.blockNumberError = err
	})
}

func WithBlockNumberErrorOnce(err error, blockNumber uint64) Option {
	return optionFunc(func(s *mockFilterer) {
		s.blockNumber = blockNumber
		s.blockNumberErrorOnce = err
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
	if m.blockNumberError != nil {
		return 0, m.blockNumberError
	}
	if m.blockNumberErrorOnce != nil {
		err := m.blockNumberErrorOnce
		m.blockNumberErrorOnce = nil
		return 0, err
	}
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
	bucketDepth      uint8
	depth            uint8
	immutable        bool
}

func (c createArgs) compare(want createArgs) error {
	if !bytes.Equal(c.id, want.id) {
		return fmt.Errorf("id mismatch. got %v want %v", c.id, want.id)
	}
	if !bytes.Equal(c.owner, want.owner) {
		return fmt.Errorf("owner mismatch. got %v want %v", c.owner, want.owner)
	}
	if c.normalisedAmount.Cmp(want.normalisedAmount) != 0 {
		return fmt.Errorf("normalised amount mismatch. got %v want %v", c.normalisedAmount.String(), want.normalisedAmount.String())
	}
	return nil
}

func (c createArgs) compareF(t *testing.T, want createArgs) {
	t.Helper()
	err := c.compare(want)
	if err != nil {
		t.Fatal(err)
	}
}

func (c createArgs) toLog(blockNumber uint64) types.Log {
	event := postageStampContractABI.Events["BatchCreated"]
	b, err := event.Inputs.NonIndexed().Pack(c.amount, c.normalisedAmount, common.BytesToAddress(c.owner), c.bucketDepth, c.depth, c.immutable)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{event.ID, common.BytesToHash(c.id)}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

type topupArgs struct {
	id                []byte
	amount            *big.Int
	normalisedBalance *big.Int
}

func (ta topupArgs) compare(want topupArgs) error {
	if !bytes.Equal(ta.id, want.id) {
		return fmt.Errorf("id mismatch. got %v want %v", ta.id, want.id)
	}
	if ta.normalisedBalance.Cmp(want.normalisedBalance) != 0 {
		return fmt.Errorf("normalised balance mismatch. got %v want %v", ta.normalisedBalance.String(), want.normalisedBalance.String())
	}
	return nil
}

func (ta topupArgs) compareF(t *testing.T, want topupArgs) {
	t.Helper()
	err := ta.compare(want)
	if err != nil {
		t.Fatal(err)
	}
}

func (ta topupArgs) toLog(blockNumber uint64) types.Log {
	event := postageStampContractABI.Events["BatchTopUp"]
	b, err := event.Inputs.NonIndexed().Pack(ta.amount, ta.normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{event.ID, common.BytesToHash(ta.id)}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

type depthArgs struct {
	id                []byte
	depth             uint8
	normalisedBalance *big.Int
}

func (d depthArgs) compare(want depthArgs) error {
	if !bytes.Equal(d.id, want.id) {
		return fmt.Errorf("id mismatch. got %v want %v", d.id, want.id)
	}
	if d.depth != want.depth {
		return fmt.Errorf("depth mismatch. got %d want %d", d.depth, want.depth)
	}
	if d.normalisedBalance.Cmp(want.normalisedBalance) != 0 {
		return fmt.Errorf("normalised balance mismatch. got %v want %v", d.normalisedBalance.String(), want.normalisedBalance.String())
	}
	return nil
}

func (d depthArgs) compareF(t *testing.T, want depthArgs) {
	t.Helper()
	err := d.compare(want)
	if err != nil {
		t.Fatal(err)
	}
}

func (d depthArgs) toLog(blockNumber uint64) types.Log {
	event := postageStampContractABI.Events["BatchDepthIncrease"]
	b, err := event.Inputs.NonIndexed().Pack(d.depth, d.normalisedBalance)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{event.ID, common.BytesToHash(d.id)}, // 1st item is the function sig digest, 2nd is always the batch id
	}
}

type priceArgs struct {
	price *big.Int
}

func (p priceArgs) compare(want priceArgs) error {
	if p.price.Cmp(want.price) != 0 {
		return fmt.Errorf("price mismatch. got %s want %s", p.price.String(), want.price.String())
	}
	return nil
}

func (p priceArgs) compareF(t *testing.T, want priceArgs) {
	t.Helper()
	err := p.compare(want)
	if err != nil {
		t.Fatal(err)
	}
}

func (p priceArgs) toLog(blockNumber uint64) types.Log {
	event := postageStampContractABI.Events["PriceUpdate"]
	b, err := event.Inputs.NonIndexed().Pack(p.price)
	if err != nil {
		panic(err)
	}
	return types.Log{
		Data:        b,
		BlockNumber: blockNumber,
		Topics:      []common.Hash{event.ID},
	}
}

type blockNumberCall struct {
	blockNumber uint64
}

func (b blockNumberCall) compare(want uint64) error {
	if b.blockNumber != want {
		return fmt.Errorf("blockNumber mismatch. got %d want %d", b.blockNumber, want)
	}
	return nil
}

func (b blockNumberCall) compareF(t *testing.T, want uint64) {
	t.Helper()
	err := b.compare(want)
	if err != nil {
		t.Fatal(err)
	}
}

type Option interface {
	apply(*mockFilterer)
}

type optionFunc func(*mockFilterer)

func (f optionFunc) apply(r *mockFilterer) { f(r) }
