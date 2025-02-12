// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"errors"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	postagetest "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

var noopEvictFn = func([]byte) error { return nil }

const defaultCapacity = 2 ^ 22

func TestBatchStore_Get(t *testing.T) {
	t.Parallel()
	testBatch := postagetest.MustNewBatch()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, defaultCapacity, log.Noop)

	err := batchStore.Save(testBatch)
	if err != nil {
		t.Fatal(err)
	}

	got := batchStoreGetBatch(t, batchStore, testBatch.ID)
	postagetest.CompareBatches(t, testBatch, got)
}

func TestBatchStore_Iterate(t *testing.T) {
	t.Parallel()
	testBatch := postagetest.MustNewBatch()
	key := batchstore.BatchKey(testBatch.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, defaultCapacity, log.Noop)

	stateStorePut(t, stateStore, key, testBatch)

	var got *postage.Batch
	err := batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		got = b
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	postagetest.CompareBatches(t, testBatch, got)
}

func TestBatchStore_IterateStopsEarly(t *testing.T) {
	t.Parallel()
	testBatch1 := postagetest.MustNewBatch()
	key1 := batchstore.BatchKey(testBatch1.ID)

	testBatch2 := postagetest.MustNewBatch()
	key2 := batchstore.BatchKey(testBatch2.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, defaultCapacity, log.Noop)

	stateStorePut(t, stateStore, key1, testBatch1)
	stateStorePut(t, stateStore, key2, testBatch2)

	var iterations = 0
	err := batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		iterations++
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if iterations != 2 {
		t.Fatalf("wanted 2 iteration, got %d", iterations)
	}

	iterations = 0
	err = batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		iterations++
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if iterations > 2 {
		t.Fatalf("wanted 1 iteration, got %d", iterations)
	}

	iterations = 0
	err = batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		iterations++
		return false, errors.New("test error")
	})
	if err == nil {
		t.Fatalf("wanted error")
	}
	if iterations > 2 {
		t.Fatalf("wanted 1 iteration, got %d", iterations)
	}
}

func TestBatchStore_SaveAndUpdate(t *testing.T) {
	t.Parallel()
	testBatch := postagetest.MustNewBatch()
	key := batchstore.BatchKey(testBatch.ID)

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, defaultCapacity, log.Noop)

	if err := batchStore.Save(testBatch); err != nil {
		t.Fatalf("storer.Save(...): unexpected error: %v", err)
	}

	//get test batch after save call
	stateStoreGet(t, stateStore, key, testBatch)

	var have postage.Batch
	stateStoreGet(t, stateStore, key, &have)
	postagetest.CompareBatches(t, testBatch, &have)

	// Check for idempotency.
	if err := batchStore.Save(testBatch); err == nil {
		t.Fatalf("storer.Save(...): expected error")
	}

	cnt := 0
	if err := stateStore.Iterate(batchstore.ValueKey(testBatch.Value, testBatch.ID), func(k, v []byte) (stop bool, err error) {
		cnt++
		return false, nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cnt > 1 {
		t.Fatal("storer.Save(...): method is not idempotent")
	}

	// Check update.
	newValue := postagetest.NewBigInt()
	newDepth := uint8(rand.Intn(int(swarm.MaxPO)))
	if err := batchStore.Update(testBatch, newValue, newDepth); err != nil {
		t.Fatalf("storer.Update(...): unexpected error: %v", err)
	}
	stateStoreGet(t, stateStore, key, &have)
	postagetest.CompareBatches(t, testBatch, &have)
}

func TestBatchStore_GetChainState(t *testing.T) {
	t.Parallel()
	testChainState := postagetest.NewChainState()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, defaultCapacity, log.Noop)

	err := batchStore.PutChainState(testChainState)
	if err != nil {
		t.Fatal(err)
	}
	got := batchStore.GetChainState()
	postagetest.CompareChainState(t, testChainState, got)
}

func TestBatchStore_PutChainState(t *testing.T) {
	t.Parallel()
	testChainState := postagetest.NewChainState()

	stateStore := mock.NewStateStore()
	batchStore, _ := batchstore.New(stateStore, nil, defaultCapacity, log.Noop)

	batchStorePutChainState(t, batchStore, testChainState)
	var got postage.ChainState
	stateStoreGet(t, stateStore, batchstore.StateKey, &got)
	postagetest.CompareChainState(t, testChainState, &got)
}

func TestBatchStore_Reset(t *testing.T) {
	t.Parallel()
	testChainState := postagetest.NewChainState()
	testBatch := postagetest.MustNewBatch(
		postagetest.WithValue(15),
		postagetest.WithDepth(8),
	)

	path := t.TempDir()
	logger := log.Noop

	// we use the real statestore since the mock uses a mutex,
	// therefore deleting while iterating (in Reset() implementation)
	// leads to a deadlock.
	stateStore, err := leveldb.NewStateStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer stateStore.Close()

	batchStore, _ := batchstore.New(stateStore, noopEvictFn, defaultCapacity, log.Noop)
	err = batchStore.Save(testBatch)
	if err != nil {
		t.Fatal(err)
	}
	err = batchStore.PutChainState(testChainState)
	if err != nil {
		t.Fatal(err)
	}
	err = batchStore.Reset()
	if err != nil {
		t.Fatal(err)
	}
	c := 0
	_ = stateStore.Iterate("", func(k, _ []byte) (bool, error) {
		c++
		return false, nil
	})

	// we expect one key in the statestore since the schema name
	// will always be there.
	if c != 0 {
		t.Fatalf("expected only one key in statestore, got %d", c)
	}
}

type testBatch struct {
	depth         uint8
	value         int
	reserveRadius uint8 // expected radius of the reserve state after the batch is added/updated
}

// TestBatchSave adds batches to the batchstore, and after each batch, checks
// the reserve state radius.
func TestBatchSave(t *testing.T) {
	t.Parallel()
	totalCapacity := batchstore.Exp2(5)

	defaultDepth := uint8(8)
	defaultValue := 1

	type testCase struct {
		add  []testBatch
		name string
	}

	// Test cases define each batches's depth, value, and the new radius
	// of the reserve state after the batch is saved.
	// In some cases, batches with zero values are added to check that the radius is not altered.

	// To calculate radius, the formula totalCommitment/node_capacity = 2^R is used where
	// total_commitment is the sum of all batches.
	// For example: to compute the radius where the node has capacity 2^5 (32) and a batch with depth 8 (2^8 or 256 chunks),
	// using the formula, 256 / 32 = 2^R, R = log2(8) produces a radius of 4.
	// With two batches of the same depth, the calculation is as such: log2((256 + 256) / 32) = 5.
	// The ceiling function is used to round up results so the actual formula is ceil(log2(totalCommitment/node_capacity)) = R

	tcs := []testCase{
		{
			name: "first batch's depth is below capacity",
			add: []testBatch{
				// first batch's total chunks (2^3) is below capacity (2^5) so radius is 0
				{depth: 3, value: defaultValue, reserveRadius: 0},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 4},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 5},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 5},
			},
		},
		{
			name: "large last depth",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, reserveRadius: 3},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 4},
				{depth: defaultDepth + 2, value: defaultValue, reserveRadius: 6},
			},
		},
		{
			name: "large depths",
			add: []testBatch{
				{depth: defaultDepth + 2, value: defaultValue, reserveRadius: 5},
				{depth: defaultDepth + 2, value: defaultValue, reserveRadius: 6},
				{depth: defaultDepth + 2, value: defaultValue, reserveRadius: 7},
			},
		},
		{
			name: "zero valued batch",
			add: []testBatch{
				// batches with values <= cumulative payout get evicted, so
				// the radius remains 0 after the addition of the first batch
				{depth: defaultDepth, value: 0, reserveRadius: 0},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 3},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t, totalCapacity)

		for _, b := range tc.add {
			_ = addBatch(t, store, b.depth, b.value)
			checkState(t, tc.name, store, b.reserveRadius)
		}
	}
}

// TestBatchUpdate adds an initial group of batches to the batchstore and one by one
// updates their depth and value fields while checking the batchstore radius values.
func TestBatchUpdate(t *testing.T) {
	t.Parallel()
	totalCapacity := batchstore.Exp2(5)

	defaultDepth := uint8(8)
	defaultValue := 1

	type testCase struct {
		name string
		// the batches to add to the batchstore.
		add []testBatch
		// update contains the new depth and value values for added batches in the order that they were saved.
		update []testBatch
	}

	// Test cases define each batches's depth, value, and the new radius of the reserve
	// state after the batch is saved/updated. Unlike depth updates, value updates
	// that are above cumulative amount should NOT result in any radius changes.
	// Value updates that are less than or equal to the cumulative amount trigger
	// the eviction for the batch, as such, radius may be altered.

	tcs := []testCase{
		{
			name: "depth increase",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, reserveRadius: 3},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 4},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 5},
			},
			update: []testBatch{
				{depth: defaultDepth + 1, value: defaultValue, reserveRadius: 5},
				{depth: defaultDepth + 1, value: defaultValue, reserveRadius: 6},
				{depth: defaultDepth + 1, value: defaultValue, reserveRadius: 6},
			},
		},
		{
			name: "value updates",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, reserveRadius: 3},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 4},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 5},
			},
			// since not depths are altered, radius should remain the same
			update: []testBatch{
				{depth: defaultDepth, value: defaultValue + 1, reserveRadius: 5},
				{depth: defaultDepth, value: defaultValue + 1, reserveRadius: 5},
				{depth: defaultDepth, value: defaultValue + 1, reserveRadius: 5},
			},
		},
		{
			name: "zero value updates",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, reserveRadius: 3},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 4},
				{depth: defaultDepth, value: defaultValue, reserveRadius: 5},
			},
			update: []testBatch{
				// batches whose value is <= cumulative amount get evicted
				// so radius is affected after each update.
				{depth: defaultDepth + 1, value: 0, reserveRadius: 4},
				{depth: defaultDepth + 1, value: 0, reserveRadius: 3},
				{depth: defaultDepth + 1, value: 0, reserveRadius: 0},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t, totalCapacity)

		var batches []*postage.Batch

		// add initial group of batches
		for _, b := range tc.add {
			newBatch := addBatch(t, store, b.depth, b.value)
			batches = append(batches, newBatch)
			checkState(t, tc.name, store, b.reserveRadius)

		}

		for i, u := range tc.update {
			batch := batches[i]

			err := store.Update(batch, big.NewInt(int64(u.value)), u.depth)
			if err != nil {
				t.Fatalf("test case: %s, %v", tc.name, err)
			}

			checkState(t, tc.name, store, u.reserveRadius)

		}
	}
}

// TestPutChainState add a group of batches to the batchstore, and after updating the chainstate,
// checks the batchstore radius reflects the updates.
func TestPutChainState(t *testing.T) {
	t.Parallel()
	totalCapacity := batchstore.Exp2(5)

	defaultDepth := uint8(8)

	type chainUpdate struct {
		block  uint64
		amount *big.Int
		radius uint8
	}

	type testCase struct {
		name  string
		add   []testBatch
		chain []chainUpdate
	}

	tcs := []testCase{
		{
			name: "evict all at once",
			// initial group of batches to add
			add: []testBatch{
				{depth: defaultDepth, value: 3, reserveRadius: 3},
				{depth: defaultDepth, value: 3, reserveRadius: 4},
				{depth: defaultDepth, value: 3, reserveRadius: 5},
			},
			// after the chain state update, the new amount is 4,
			// which is greater than the values of the batches.
			// All the batches get evicted, and the new radius is zero.
			chain: []chainUpdate{
				{block: 1, amount: big.NewInt(4), radius: 0},
			},
		},
		{
			name: "evict all with two updates",
			add: []testBatch{
				{depth: defaultDepth, value: 3, reserveRadius: 3},
				{depth: defaultDepth, value: 4, reserveRadius: 4},
				{depth: defaultDepth, value: 5, reserveRadius: 5},
			},
			chain: []chainUpdate{
				{block: 1, amount: big.NewInt(4), radius: 3},
				{block: 2, amount: big.NewInt(5), radius: 0},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t, totalCapacity)

		// add the group of batches
		for _, b := range tc.add {
			_ = addBatch(t, store, b.depth, b.value)
			checkState(t, tc.name, store, b.reserveRadius)
		}

		for _, c := range tc.chain {

			// update chain state
			err := store.PutChainState(&postage.ChainState{
				Block:        c.block,
				TotalAmount:  c.amount,
				CurrentPrice: big.NewInt(1),
			})
			if err != nil {
				t.Fatalf("test case: %s, %v", tc.name, err)
			}

			checkState(t, tc.name, store, c.radius)
		}
	}
}

func TestBatchExpiry(t *testing.T) {
	t.Parallel()
	store := setupBatchStore(t, defaultCapacity)

	batch := postagetest.MustNewBatch(
		postagetest.WithValue(int64(4)),
		postagetest.WithDepth(0),
		postagetest.WithStart(111),
	)
	if err := store.Save(batch); err != nil {
		t.Fatal(err)
	}

	esi := postage.NewStampIssuer("", "", batch.ID, big.NewInt(3), 11, 10, 1000, true)
	emp := mockpost.New(mockpost.WithIssuer(esi))
	store.SetBatchExpiryHandler(emp)

	// update chain state
	err := store.PutChainState(&postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(10),
		CurrentPrice: big.NewInt(10),
	})
	if err != nil {
		t.Fatal(err)
	}

	if exists, err := store.Exists(esi.ID()); err != nil || exists {
		t.Fatalf("Want %v, got %v, error %v", false, exists, err)
	}
}

func TestUnexpiredBatch(t *testing.T) {
	t.Parallel()
	store := setupBatchStore(t, defaultCapacity)

	batch := postagetest.MustNewBatch(
		postagetest.WithValue(int64(14)),
		postagetest.WithDepth(0),
		postagetest.WithStart(111),
	)
	if err := store.Save(batch); err != nil {
		t.Fatal(err)
	}

	esi := postage.NewStampIssuer("", "", batch.ID, big.NewInt(15), 11, 10, 1000, true)
	emp := mockpost.New(mockpost.WithIssuer(esi))
	store.SetBatchExpiryHandler(emp)

	// update chain state
	err := store.PutChainState(&postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(10),
		CurrentPrice: big.NewInt(10),
	})
	if err != nil {
		t.Fatal(err)
	}

	if exists, err := store.Exists(esi.ID()); err != nil || !exists {
		t.Fatalf("Want %v, got %v, error %v", false, exists, err)
	}
}

func setupBatchStore(t *testing.T, capacity int) postage.Storer {
	t.Helper()
	dir := t.TempDir()

	logger := log.Noop
	stateStore, err := leveldb.NewStateStore(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, stateStore)

	evictFn := func(b []byte) error {
		return nil
	}

	bStore, _ := batchstore.New(stateStore, evictFn, capacity, log.Noop)

	err = bStore.PutChainState(&postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(0),
		CurrentPrice: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	return bStore
}

func checkState(t *testing.T, name string, store postage.Storer, radius uint8) {
	t.Helper()

	if radius != store.Radius() {
		t.Fatalf("got radius %v, want %v, test case: %s", store.Radius(), radius, name)
	}
}

func addBatch(t *testing.T, s postage.Storer, depth uint8, value int) *postage.Batch {
	t.Helper()

	batch := postagetest.MustNewBatch(
		postagetest.WithValue(int64(value)),
		postagetest.WithDepth(depth),
		postagetest.WithStart(111),
	)
	if err := s.Save(batch); err != nil {
		t.Fatal(err)
	}

	return batch
}

func stateStoreGet(t *testing.T, st storage.StateStorer, k string, v interface{}) {
	t.Helper()

	if err := st.Get(k, v); err != nil {
		t.Fatalf("store get batch: %v", err)
	}
}

func stateStorePut(t *testing.T, st storage.StateStorer, k string, v interface{}) {
	t.Helper()

	if err := st.Put(k, v); err != nil {
		t.Fatalf("store put batch: %v", err)
	}
}

func batchStoreGetBatch(t *testing.T, st postage.Storer, id []byte) *postage.Batch {
	t.Helper()

	b, err := st.Get(id)
	if err != nil {
		t.Fatalf("postage storer get: %v", err)
	}
	return b
}

func batchStorePutChainState(t *testing.T, st postage.Storer, cs *postage.ChainState) {
	t.Helper()

	if err := st.PutChainState(cs); err != nil {
		t.Fatalf("postage storer put chain state: %v", err)
	}
}
