// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchservice_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"math/rand"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/statestore/leveldb"
	mocks "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

var (
	testLog    = log.Noop
	errTest    = errors.New("fails")
	testTxHash = common.BytesToHash(make([]byte, 32))
)

type mockListener struct{}

func (*mockListener) Listen(ctx context.Context, from uint64, updater postage.EventUpdater) <-chan error {
	c := make(chan error, 1)
	c <- nil
	return c
}
func (*mockListener) Close() error { return nil }

func newMockListener() *mockListener {
	return &mockListener{}
}

type mockBatchListener struct {
	createCount int
	topupCount  int
	diluteCount int
}

func (m *mockBatchListener) HandleCreate(b *postage.Batch, _ *big.Int) error {
	m.createCount++
	return nil
}

func (m *mockBatchListener) HandleTopUp(_ []byte, _ *big.Int) {
	m.topupCount++
}

func (m *mockBatchListener) HandleDepthIncrease(_ []byte, _ uint8) {
	m.diluteCount++
}

var _ postage.BatchEventListener = (*mockBatchListener)(nil)

func TestBatchServiceCreate(t *testing.T) {
	t.Parallel()

	testChainState := postagetesting.NewChainState()

	validateNoBatch := func(t *testing.T, testBatch *postage.Batch, st *mock.BatchStore) {
		t.Helper()

		got, err := st.Exists(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}
		if got {
			t.Fatalf("expected batch not to exist")
		}
	}

	validateBatch := func(t *testing.T, testBatch *postage.Batch, st *mock.BatchStore) {
		t.Helper()

		got, err := st.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if !bytes.Equal(got.ID, testBatch.ID) {
			t.Fatalf("batch id: want %v, got %v", testBatch.ID, got.ID)
		}
		if !bytes.Equal(got.Owner, testBatch.Owner) {
			t.Fatalf("batch owner: want %v, got %v", testBatch.Owner, got.Owner)
		}
		if got.Value.Cmp(testBatch.Value) != 0 {
			t.Fatalf("batch value: want %v, got %v", testBatch.Value.String(), got.Value.String())
		}
		if got.BucketDepth != testBatch.BucketDepth {
			t.Fatalf("bucket depth: want %v, got %v", got.BucketDepth, testBatch.BucketDepth)
		}
		if got.Depth != testBatch.Depth {
			t.Fatalf("batch depth: want %v, got %v", got.Depth, testBatch.Depth)
		}
		if got.Immutable != testBatch.Immutable {
			t.Fatalf("immutable: want %v, got %v", got.Immutable, testBatch.Immutable)
		}
		if got.Start != testChainState.Block {
			t.Fatalf("batch start block different form chain state: want %v, got %v", got.Start, testChainState.Block)
		}
	}

	t.Run("expect put create put error", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		testAmount := postagetesting.NewBigInt()

		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
			mock.WithSaveError(errTest),
		)

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			testAmount,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err == nil {
			t.Fatalf("expected error")
		}
		if testBatchListener.createCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.createCount)
		}
	})

	t.Run("passes", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		testAmount := postagetesting.NewBigInt()

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)
		testBatch.Value.Add(testChainState.TotalAmount, big.NewInt(0).Mul(testChainState.CurrentPrice, big.NewInt(2)))
		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testAmount,
			testBatch.Value,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err != nil {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.createCount != 1 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.createCount)
		}

		validateBatch(t, testBatch, batchStore)
	})

	t.Run("passes without recovery", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		testAmount := postagetesting.NewBigInt()

		// create a owner different from the batch owner
		owner := testutil.RandBytes(t, 32)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)

		testBatch.Value.Add(testChainState.TotalAmount, big.NewInt(0).Mul(testChainState.CurrentPrice, big.NewInt(2)))

		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testAmount,
			testBatch.Value,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); err != nil {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.createCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.createCount)
		}

		validateBatch(t, testBatch, batchStore)
	})

	t.Run("batch with near-zero val rejected", func(t *testing.T) {
		testBatch := postagetesting.MustNewBatch()
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithChainState(testChainState),
		)

		vv := big.NewInt(0).Add(testChainState.CurrentPrice, testChainState.TotalAmount)
		if err := svc.Create(
			testBatch.ID,
			testBatch.Owner,
			testBatch.Value,
			vv,
			testBatch.Depth,
			testBatch.BucketDepth,
			testBatch.Immutable,
			testTxHash,
		); !errors.Is(err, batchservice.ErrZeroValueBatch) {
			t.Fatalf("got error %v", err)
		}
		if testBatchListener.createCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.createCount)
		}

		validateNoBatch(t, testBatch, batchStore)
	})
}

func TestBatchServiceTopUp(t *testing.T) {
	t.Parallel()

	testBatch := postagetesting.MustNewBatch()
	testNormalisedBalance := big.NewInt(2000000000000)
	testTopUpAmount := big.NewInt(1000)
	t.Run("expect get error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithGetErr(errTest, 0),
		)

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected error")
		}
		if testBatchListener.topupCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.topupCount)
		}
	})

	t.Run("expect update error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithUpdateErr(errTest, 0),
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected error")
		}
		if testBatchListener.topupCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.topupCount)
		}
	})

	t.Run("passes", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		want := testNormalisedBalance

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if got.Value.Cmp(want) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, want)
		}
		if testBatchListener.topupCount != 1 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.topupCount)
		}
	})

	// if a batch with a different owner is topped up we should not see any event fired in the
	// batch service
	t.Run("passes without BatchEventListener update", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		// create an owner different from the batch owner
		owner := testutil.RandBytes(t, 32)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		want := testNormalisedBalance

		if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("top up: %v", err)
		}

		got, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if got.Value.Cmp(want) != 0 {
			t.Fatalf("topped up amount: got %v, want %v", got.Value, want)
		}
		if testBatchListener.topupCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.topupCount)
		}
	})
}

func TestBatchServiceUpdateDepth(t *testing.T) {
	t.Parallel()

	const testNewDepth = 30
	testNormalisedBalance := big.NewInt(2000000000000)
	testBatch := postagetesting.MustNewBatch()

	t.Run("expect get error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, _, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithGetErr(errTest, 0),
		)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected get error")
		}

		if testBatchListener.diluteCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.diluteCount)
		}
	})

	t.Run("expect put error", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
			mock.WithUpdateErr(errTest, 0),
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err == nil {
			t.Fatal("expected put error")
		}

		if testBatchListener.diluteCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.diluteCount)
		}
	})

	t.Run("passes", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			testBatch.Owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if val.Depth != testNewDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", testNewDepth, val.Depth)
		}

		if testBatchListener.diluteCount != 1 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 1, testBatchListener.diluteCount)
		}
	})

	// if a batch with a different owner is diluted we should not see any event fired in the
	// batch service
	t.Run("passes without BatchEventListener update", func(t *testing.T) {
		testBatchListener := &mockBatchListener{}
		// create an owner different from the batch owner
		owner := testutil.RandBytes(t, 32)

		svc, batchStore, _ := newTestStoreAndServiceWithListener(
			t,
			owner,
			testBatchListener,
		)
		createBatch(t, batchStore, testBatch)

		if err := svc.UpdateDepth(testBatch.ID, testNewDepth, testNormalisedBalance, testTxHash); err != nil {
			t.Fatalf("update depth: %v", err)
		}

		val, err := batchStore.Get(testBatch.ID)
		if err != nil {
			t.Fatalf("batch store get: %v", err)
		}

		if val.Depth != testNewDepth {
			t.Fatalf("wrong batch depth set: want %v, got %v", testNewDepth, val.Depth)
		}

		if testBatchListener.diluteCount != 0 {
			t.Fatalf("unexpected batch listener count, exp %d found %d", 0, testBatchListener.diluteCount)
		}
	})
}

func TestBatchServiceUpdatePrice(t *testing.T) {
	t.Parallel()

	testChainState := postagetesting.NewChainState()
	testChainState.CurrentPrice = big.NewInt(100000)
	testNewPrice := big.NewInt(20000000)

	t.Run("expect put error", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
			mock.WithUpdateErr(errTest, 1),
		)
		putChainState(t, batchStore, testChainState)

		if err := svc.UpdatePrice(testNewPrice, testTxHash); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("passes", func(t *testing.T) {
		svc, batchStore, _ := newTestStoreAndService(
			t,
			mock.WithChainState(testChainState),
		)

		if err := svc.UpdatePrice(testNewPrice, testTxHash); err != nil {
			t.Fatalf("update price: %v", err)
		}

		cs := batchStore.GetChainState()
		if cs.CurrentPrice.Cmp(testNewPrice) != 0 {
			t.Fatalf("bad price: want %v, got %v", cs.CurrentPrice, testNewPrice)
		}
	})
}

func TestBatchServiceUpdateBlockNumber(t *testing.T) {
	t.Parallel()

	testChainState := &postage.ChainState{
		Block:        1,
		CurrentPrice: big.NewInt(100),
		TotalAmount:  big.NewInt(100),
	}
	svc, batchStore, _ := newTestStoreAndService(
		t,
		mock.WithChainState(testChainState),
	)

	// advance the block number and expect total cumulative payout to update
	nextBlock := uint64(4)

	if err := svc.UpdateBlockNumber(nextBlock); err != nil {
		t.Fatalf("update price: %v", err)
	}
	nn := big.NewInt(400)
	cs := batchStore.GetChainState()
	if cs.TotalAmount.Cmp(nn) != 0 {
		t.Fatalf("bad price: want %v, got %v", nn, cs.TotalAmount)
	}
}

// The eq* types and helpers below back the #5343 chain-state buffering
// regression tests (TestChainStateBufferingEquivalence and its randomized
// sibling). #5343 stopped calling storer.PutChainState (which runs cleanup +
// radius recompute) on every UpdatePrice/UpdateBlockNumber and instead buffers
// the chain state in pendingChainState, flushing once per page at
// TransactionEnd. The concern is whether deferring that flush, while the price
// fluctuates up and down within a window, can leave a different set of evicted
// batches or a different radius than the old per-event behaviour.
//
// It cannot: the cumulative outpayment (TotalAmount) is monotonically
// non-decreasing, so a single cleanup at the end of a window evicts the exact
// same superset, and computeRadius is a pure function of the final batch set
// and depths (Create/TopUp/UpdateDepth recompute it immediately, never
// deferred). The helpers prove this by replaying one identical event stream
// three ways against three independent real batchstores and asserting their
// final batch set, radius and chain state are identical:
//
//   - per-event: no transaction, so every UpdateBlockNumber/UpdatePrice
//     persists immediately (pre-#5343 behaviour).
//   - single-tx: the whole stream buffered, a single flush at the end.
//   - per-page:  one flush per page at TransactionEnd (#5343 behaviour).

type eqBatch struct {
	id, owner []byte
}

type eqKind int

const (
	eqCreate eqKind = iota
	eqTopUp
	eqDepth
	eqPrice
)

type eqEvent struct {
	block uint64
	kind  eqKind
	batch *eqBatch // create/topup/depth target
	value int64    // create/topup/depth normalised balance
	depth uint8    // create/depth depth
	price int64    // price
}

type eqPage struct {
	events []eqEvent
	end    uint64
}

func applyEqEvent(t *testing.T, svc postage.EventUpdater, mode string, e eqEvent) {
	t.Helper()
	if err := svc.UpdateBlockNumber(e.block); err != nil {
		t.Fatalf("%s: update block %d: %v", mode, e.block, err)
	}
	switch e.kind {
	case eqCreate:
		// A near-zero-value batch is rejected identically in all modes (the
		// chain state at a Create is the same regardless of flush timing), so
		// mirror the listener and treat it as a skip rather than a failure.
		err := svc.Create(e.batch.id, e.batch.owner, big.NewInt(0), big.NewInt(e.value), e.depth, 16, false, testTxHash)
		if err != nil && !errors.Is(err, batchservice.ErrZeroValueBatch) {
			t.Fatalf("%s: create: %v", mode, err)
		}
	case eqTopUp:
		if err := svc.TopUp(e.batch.id, big.NewInt(0), big.NewInt(e.value), testTxHash); err != nil {
			t.Fatalf("%s: topup: %v", mode, err)
		}
	case eqDepth:
		if err := svc.UpdateDepth(e.batch.id, e.depth, big.NewInt(e.value), testTxHash); err != nil {
			t.Fatalf("%s: depth: %v", mode, err)
		}
	case eqPrice:
		if err := svc.UpdatePrice(big.NewInt(e.price), testTxHash); err != nil {
			t.Fatalf("%s: price: %v", mode, err)
		}
	}
}

// replayEq drives the script through svc in one of three flushing modes. The
// per-event mode omits transactions entirely, reproducing the pre-#5343
// per-event PutChainState behaviour, while single-tx and per-page wrap the
// stream the way the listener does for a single large page and for many pages.
func replayEq(t *testing.T, svc postage.EventUpdater, mode string, script []eqPage) {
	t.Helper()

	pageEnd := func(p eqPage) {
		if err := svc.UpdateBlockNumber(p.end); err != nil {
			t.Fatalf("%s: page end block %d: %v", mode, p.end, err)
		}
	}
	begin := func() {
		if err := svc.TransactionStart(); err != nil {
			t.Fatalf("%s: tx start: %v", mode, err)
		}
	}
	end := func() {
		if err := svc.TransactionEnd(); err != nil {
			t.Fatalf("%s: tx end: %v", mode, err)
		}
	}

	switch mode {
	case "per-event":
		for _, p := range script {
			for _, e := range p.events {
				applyEqEvent(t, svc, mode, e)
			}
			pageEnd(p)
		}
	case "single-tx":
		begin()
		for _, p := range script {
			for _, e := range p.events {
				applyEqEvent(t, svc, mode, e)
			}
			pageEnd(p)
		}
		end()
	case "per-page":
		for _, p := range script {
			begin()
			for _, e := range p.events {
				applyEqEvent(t, svc, mode, e)
			}
			pageEnd(p)
			end()
		}
	}
}

type eqSnapshot struct {
	block   uint64
	total   string
	price   string
	radius  uint8
	batches map[string]bool
}

func takeEqSnapshot(t *testing.T, store postage.Storer) eqSnapshot {
	t.Helper()
	cs := store.GetChainState()
	s := eqSnapshot{
		block:   cs.Block,
		total:   cs.TotalAmount.String(),
		price:   cs.CurrentPrice.String(),
		radius:  store.Radius(),
		batches: map[string]bool{},
	}
	if err := store.Iterate(func(b *postage.Batch) (bool, error) {
		s.batches[hex.EncodeToString(b.ID)] = true
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

// assertBufferingEquivalence replays the script against three independent real
// batchstores (per-event reference, one buffered transaction, one transaction
// per page) and fails if any buffered mode's final state differs from the
// per-event reference. It returns the reference snapshot.
func assertBufferingEquivalence(t *testing.T, capacity int, script []eqPage) eqSnapshot {
	t.Helper()
	run := func(mode string) eqSnapshot {
		svc, store := newRealStoreService(t, capacity)
		replayEq(t, svc, mode, script)
		return takeEqSnapshot(t, store)
	}
	ref := run("per-event")
	for _, mode := range []string{"single-tx", "per-page"} {
		if got := run(mode); !reflect.DeepEqual(ref, got) {
			t.Fatalf("mode %q diverged from per-event reference:\n  per-event: %+v\n  %-9s: %+v", mode, ref, mode, got)
		}
	}
	return ref
}

func TestChainStateBufferingEquivalence(t *testing.T) {
	t.Parallel()

	// Small capacity so total commitment exceeds it and radius is non-zero.
	const capacity = 32

	mk := func() *eqBatch {
		return &eqBatch{id: testutil.RandBytes(t, 32), owner: testutil.RandBytes(t, 32)}
	}
	a, b, c := mk(), mk(), mk()

	// Models the postage contract event stream the listener replays, covering
	// all four EventUpdater mutations:
	//   - A, B, C created at depth 8 in the first blocks,
	//   - C topped up (8000) and then diluted to depth 10, so it stays alive and
	//     the final radius is driven by its new depth,
	//   - per-block price rises 1 -> 10 then falls 10 -> 2.
	// Final TotalAmount is 153, which evicts A (50) and B (150); C survives.
	script := []eqPage{
		{
			events: []eqEvent{
				{block: 1, kind: eqCreate, batch: a, value: 50, depth: 8},
				{block: 2, kind: eqCreate, batch: b, value: 150, depth: 8},
				{block: 3, kind: eqCreate, batch: c, value: 5000, depth: 8},
				{block: 13, kind: eqPrice, price: 10},
			},
			end: 13,
		},
		{
			events: []eqEvent{
				{block: 18, kind: eqTopUp, batch: c, value: 8000},
				{block: 23, kind: eqDepth, batch: c, value: 8000, depth: 10},
				{block: 23, kind: eqPrice, price: 2},
			},
			end: 23,
		},
		{
			events: []eqEvent{},
			end:    43,
		},
	}

	ref := assertBufferingEquivalence(t, capacity, script)

	// Guard against future edits making the script vacuous: it must actually
	// evict A and B, keep C, and move the radius off zero.
	if len(ref.batches) != 1 || !ref.batches[hex.EncodeToString(c.id)] {
		t.Fatalf("script no longer exercises eviction: surviving batches %v", ref.batches)
	}
	if ref.radius == 0 {
		t.Fatal("script no longer exercises radius change: radius is 0")
	}
}

// TestChainStateBufferingEquivalenceRandomized fuzzes the equivalence property:
// for many randomly generated event streams (random prices up and down, random
// top-ups and depth changes, random page boundaries), the buffered modes must
// reach the same final state as the per-event reference. TopUp/UpdateDepth only
// ever target "permanent" batches whose value exceeds any reachable TotalAmount,
// so the targeted batch is guaranteed to exist in every mode regardless of how
// flush timing shifts eviction of the ephemeral batches.
func TestChainStateBufferingEquivalenceRandomized(t *testing.T) {
	t.Parallel()

	const (
		iterations = 40
		capacity   = 16
	)

	const seed = int64(20260217)
	t.Logf("randomized equivalence seed=%d (override by editing the test)", seed)
	rng := rand.New(rand.NewSource(seed))

	for i := 0; i < iterations; i++ {
		script := randomEqScript(rng)
		t.Run(fmt.Sprintf("iter_%02d", i), func(t *testing.T) {
			assertBufferingEquivalence(t, capacity, script)
		})
	}
}

// randomEqScript builds a reproducible random event stream. All batches are
// created in the first page (so later top-ups/depth changes always reference an
// existing batch); subsequent pages carry random price moves, top-ups and depth
// changes split across random page boundaries.
func randomEqScript(rng *rand.Rand) []eqPage {
	// permanentValue is far above any TotalAmount the script can reach
	// (bounded by ~maxBlocks * maxPrice), so permanent batches never expire.
	const permanentValue = int64(1) << 40

	randBytes := func(n int) []byte {
		b := make([]byte, n)
		_, _ = rng.Read(b)
		return b
	}
	randDepth := func() uint8 { return uint8(2 + rng.Intn(9)) } // 2..10

	var block uint64
	nextBlock := func(maxGap int) uint64 {
		block += uint64(1 + rng.Intn(maxGap))
		return block
	}

	numPermanent := 1 + rng.Intn(2) // 1..2
	numEphemeral := 2 + rng.Intn(4) // 2..5

	var (
		creates    []eqEvent
		permanents []*eqBatch
	)
	for p := 0; p < numPermanent; p++ {
		batch := &eqBatch{id: randBytes(32), owner: randBytes(32)}
		permanents = append(permanents, batch)
		creates = append(creates, eqEvent{block: nextBlock(3), kind: eqCreate, batch: batch, value: permanentValue, depth: randDepth()})
	}
	for e := 0; e < numEphemeral; e++ {
		batch := &eqBatch{id: randBytes(32), owner: randBytes(32)}
		// Values that, given the price/block accrual below, leave a random mix
		// of survivors and evictions.
		value := int64(50 + rng.Intn(4951)) // 50..5000
		creates = append(creates, eqEvent{block: nextBlock(3), kind: eqCreate, batch: batch, value: value, depth: randDepth()})
	}

	script := []eqPage{{events: creates, end: block}}

	numPages := 1 + rng.Intn(6)
	for p := 0; p < numPages; p++ {
		var events []eqEvent
		for n := rng.Intn(5); n > 0; n-- { // 0..4 events per page
			blk := nextBlock(10)
			switch rng.Intn(3) {
			case 0:
				events = append(events, eqEvent{block: blk, kind: eqPrice, price: int64(1 + rng.Intn(40))})
			case 1:
				events = append(events, eqEvent{block: blk, kind: eqTopUp, batch: permanents[rng.Intn(len(permanents))], value: permanentValue})
			case 2:
				events = append(events, eqEvent{block: blk, kind: eqDepth, batch: permanents[rng.Intn(len(permanents))], value: permanentValue, depth: randDepth()})
			}
		}
		end := block
		if len(events) == 0 {
			end = nextBlock(10) // ensure blocks advance so eviction can progress
		}
		script = append(script, eqPage{events: events, end: end})
	}

	return script
}

func TestTransactionOk(t *testing.T) {
	t.Parallel()

	svc, store, s := newTestStoreAndService(t)
	if err := svc.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionEnd(); err != nil {
		t.Fatal(err)
	}

	svc2, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 0 {
		t.Fatalf("expect %d reset calls got %d", 0, c)
	}
}

func TestTransactionError(t *testing.T) {
	t.Parallel()

	svc, store, s := newTestStoreAndService(t)
	if err := svc.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransactionStart(); err != nil {
		t.Fatal(err)
	}

	svc2, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc2.Start(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if c := store.ResetCalls(); c != 1 {
		t.Fatalf("expect %d reset calls got %d", 1, c)
	}
}

func TestChecksum(t *testing.T) {
	t.Parallel()

	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, false)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testTopUpAmount := big.NewInt(1000)
	testBatch := postagetesting.MustNewBatch()
	createBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}
}

func TestChecksumResync(t *testing.T) {
	t.Parallel()

	s := mocks.NewStateStore()
	store := mock.New()
	mockHash := &hs{}
	svc, err := batchservice.New(s, store, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash }, true)
	if err != nil {
		t.Fatal(err)
	}
	testNormalisedBalance := big.NewInt(2000000000000)
	testTopUpAmount := big.NewInt(1000)
	testBatch := postagetesting.MustNewBatch()
	createBatch(t, store, testBatch)

	if err := svc.TopUp(testBatch.ID, testTopUpAmount, testNormalisedBalance, testTxHash); err != nil {
		t.Fatalf("top up: %v", err)
	}
	if m := mockHash.ctr; m != 2 {
		t.Fatalf("expected %d calls got %d", 2, m)
	}

	// now start a new instance and check that the value gets read from statestore
	store2 := mock.New()
	mockHash2 := &hs{}
	_, err = batchservice.New(s, store2, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash2 }, false)
	if err != nil {
		t.Fatal(err)
	}
	if m := mockHash2.ctr; m != 1 {
		t.Fatalf("expected %d calls got %d", 1, m)
	}

	// now start a new instance and check that the value does not get written into the hasher
	// when resyncing
	store3 := mock.New()
	mockHash3 := &hs{}
	_, err = batchservice.New(s, store3, testLog, newMockListener(), nil, nil, func() hash.Hash { return mockHash3 }, true)
	if err != nil {
		t.Fatal(err)
	}
	if m := mockHash3.ctr; m != 0 {
		t.Fatalf("expected %d calls got %d", 0, m)
	}
}

func newTestStoreAndServiceWithListener(
	t *testing.T,
	owner []byte,
	batchListener postage.BatchEventListener,
	opts ...mock.Option,
) (postage.EventUpdater, *mock.BatchStore, storage.StateStorer) {
	t.Helper()
	s := mocks.NewStateStore()
	store := mock.New(opts...)
	svc, err := batchservice.New(s, store, testLog, newMockListener(), owner, batchListener, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	return svc, store, s
}

func newTestStoreAndService(t *testing.T, opts ...mock.Option) (postage.EventUpdater, *mock.BatchStore, storage.StateStorer) {
	t.Helper()
	return newTestStoreAndServiceWithListener(t, nil, nil, opts...)
}

// newRealStoreService wires a batchservice to a real batchstore backed by a
// leveldb state store, so that PutChainState exercises the actual cleanup and
// computeRadius logic. The in-memory mock state store cannot be used here: it
// iterates its backing map in random order, which breaks the sorted early-stop
// in batchstore cleanup and yields non-deterministic evictions.
func newRealStoreService(t *testing.T, capacity int) (postage.EventUpdater, postage.Storer) {
	t.Helper()

	// In-memory leveldb: sorted iteration (unlike the map-backed mock state
	// store) is required for batchstore cleanup's value-ordered early-stop.
	stateStore, err := leveldb.NewInMemoryStateStore(log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, stateStore)

	store, err := batchstore.New(stateStore, func([]byte) error { return nil }, capacity, log.Noop)
	if err != nil {
		t.Fatal(err)
	}

	// Initialise chain state as the node bootstrap does.
	if err := store.PutChainState(&postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(0),
		CurrentPrice: big.NewInt(1),
	}); err != nil {
		t.Fatal(err)
	}

	svc, err := batchservice.New(mocks.NewStateStore(), store, testLog, newMockListener(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	return svc, store
}

func createBatch(t *testing.T, store postage.Storer, b *postage.Batch) {
	t.Helper()

	if err := store.Save(b); err != nil {
		t.Fatalf("store create batch: %v", err)
	}
}

func putChainState(t *testing.T, store postage.Storer, cs *postage.ChainState) {
	t.Helper()

	if err := store.PutChainState(cs); err != nil {
		t.Fatalf("store put chain state: %v", err)
	}
}

type hs struct{ ctr uint8 }

func (h *hs) Write(p []byte) (n int, err error) { h.ctr++; return len(p), nil }
func (h *hs) Sum(b []byte) []byte               { return []byte{h.ctr} }
func (h *hs) Reset()                            {}
func (h *hs) Size() int                         { panic("not implemented") }
func (h *hs) BlockSize() int                    { panic("not implemented") }
