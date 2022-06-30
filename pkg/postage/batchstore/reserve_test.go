// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"io"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
)

type testBatch struct {
	depth         uint8
	value         int
	reserveRadius uint8 // expected radius of the reserve state after the batch is added/updated
}

// TestBatchSave adds batches to the batchstore, and after each batch, checks
// the reserve state radius.
func TestBatchSave(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = totalCapacity

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

		store := setupBatchStore(t)

		for _, b := range tc.add {
			_ = addBatch(t, store, b.depth, b.value)
			checkState(t, tc.name, store, b.reserveRadius)
		}
	}
}

// TestBatchUpdate adds an initial group of batches to the batchstore and one by one
// updates their depth and value fields while checking the batchstore radius values.
func TestBatchUpdate(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = totalCapacity

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
	// the eviction for the the batch, as such, radius may be altered.

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

		store := setupBatchStore(t)

		var batches []*postage.Batch

		// add initial groupd of batches
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

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = totalCapacity

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

		store := setupBatchStore(t)

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

// TestUnreserve tests the Unreserve call increases the storage radius after each
// full iteration and that the storage radius is calculated correctly after a radius decrease.
func TestUnreserveAndLowerStorageRadius(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)
	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = totalCapacity

	const (
		initDepth                = 8 // batch depth
		expiredValue             = 1 // new chain state total amount that causes some batches to expire
		radiusAfterFirstBatches  = 5 // radius after three batches
		radiusAfterSecondBatches = 6 // radius after two more batches
		radiusAfterChainUpdate   = 5 // radius after expiring two batches with the chain update
		storageRadiusAfterUpdate = 4 // storage radius after chain update
	)

	store := setupBatchStore(t)

	// add some batches
	_ = addBatch(t, store, initDepth, expiredValue)
	_ = addBatch(t, store, initDepth, expiredValue)
	_ = addBatch(t, store, initDepth, expiredValue+1)

	state := store.GetReserveState()

	if state.Radius != radiusAfterFirstBatches {
		t.Fatalf("got radius %d, want %d", state.Radius, radiusAfterFirstBatches)
	}

	cb := func([]byte, uint8) (bool, error) { return false, nil }

	// Unreserve is called multiple times (radius + 1) to confirm that the
	// storage radius of the node never exceeds the radius.
	for i := uint8(0); i <= state.Radius+1; i++ {

		wantStorageRadius := i
		if wantStorageRadius > state.Radius {
			wantStorageRadius = state.Radius
		}

		state := store.GetReserveState()

		if state.StorageRadius != wantStorageRadius {
			t.Fatalf("got storage radius %d, want %d", state.StorageRadius, wantStorageRadius)
		}

		_ = store.Unreserve(cb)
	}

	// add some batches to increase the radius by one
	_ = addBatch(t, store, initDepth, expiredValue+1)
	_ = addBatch(t, store, initDepth, expiredValue+1)

	state = store.GetReserveState()

	if state.Radius != radiusAfterSecondBatches {
		t.Fatalf("got radius %d, want %d", state.Radius, radiusAfterSecondBatches)
	}

	// in the final step, the chain state is updated to evict two batches,
	// causing the radius to decrease, in return, a new storage radius is calculated,
	// and the storage radius of batches are lowered to the current storage radius.
	err := store.PutChainState(&postage.ChainState{
		Block:        1,
		TotalAmount:  big.NewInt(expiredValue),
		CurrentPrice: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}

	state = store.GetReserveState()

	// check that after the chain state update, radius has been lowered.
	if state.Radius != radiusAfterChainUpdate {
		t.Fatalf("got radius %d, want %d", state.Radius, radiusAfterChainUpdate)
	}

	// check that after the chain state update, storage radius has been lowered.
	if state.StorageRadius != storageRadiusAfterUpdate {
		t.Fatalf("got storage radius %d, want %d", state.StorageRadius, storageRadiusAfterUpdate)
	}

	// the radius of every batch must not exceed the current storage radius after a storage radius decrease.
	err = store.Iterate(func(b *postage.Batch) (bool, error) {
		if b.StorageRadius > state.StorageRadius {
			t.Fatalf("batch radius %d should not exceed storate radius %d", b.StorageRadius, state.StorageRadius)
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func setupBatchStore(t *testing.T) postage.Storer {
	t.Helper()
	dir := t.TempDir()

	logger := logging.New(io.Discard, 0)
	stateStore, err := leveldb.NewStateStore(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stateStore.Close(); err != nil {
			t.Fatal(err)
		}
	})

	evictFn := func(b []byte) error {
		return nil
	}

	bStore, _ := batchstore.New(stateStore, evictFn, logger)
	bStore.SetStorageRadiusSetter(noopRadiusSetter{})

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

	state := store.GetReserveState()

	if state.StorageRadius > state.Radius {
		t.Fatalf("storage radius %d must not exceed radius %d, test case: %s", state.StorageRadius, state.Radius, name)
	}

	if radius != state.Radius {
		t.Fatalf("got radius %v, want %v, test case: %s", state.Radius, radius, name)
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
