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
	depth  uint8
	value  int
	radius uint8
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
	// In some cases, batches with zero values are added to check that radius is not altered.

	tcs := []testCase{
		{
			name: "first batch's depth is below capacity",
			add: []testBatch{
				// first batch's total chunks (2^3) is below capacity (2^5) so radius is 0
				{depth: 3, value: defaultValue, radius: 0},
				{depth: defaultDepth, value: defaultValue, radius: 4},
				{depth: defaultDepth, value: defaultValue, radius: 5},
				{depth: defaultDepth, value: defaultValue, radius: 5},
			},
		},
		{
			name: "large last depth",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, radius: 3},
				{depth: defaultDepth, value: defaultValue, radius: 4},
				{depth: defaultDepth + 2, value: defaultValue, radius: 6},
			},
		},
		{
			name: "large depths",
			add: []testBatch{
				{depth: defaultDepth + 2, value: defaultValue, radius: 5},
				{depth: defaultDepth + 2, value: defaultValue, radius: 6},
				{depth: defaultDepth + 2, value: defaultValue, radius: 7},
			},
		},
		{
			name: "zero valued batch",
			add: []testBatch{
				// batches with values <= cumulative payout get evicted, so
				// the radius remains 0 after the addition of the first batch
				{depth: defaultDepth, value: 0, radius: 0},
				{depth: defaultDepth, value: defaultValue, radius: 3},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		for _, b := range tc.add {
			_ = addBatch(t, store, b.depth, b.value)
			checkState(t, tc.name, store, b.radius)
		}
	}
}

// TestBatchUpdate adds an initial group of batches to the batchstore and one by one
// updates their depth and value to fields while checking the batchstore radius values.
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

		// the initial batches to add to the batchstore.
		add []testBatch
		// update contains the new depth and value values for the added batches
		update []testBatch
	}

	// Test cases define each batches's depth, value, and the new radius
	// of the reserve state after the batch is saved/updated.
	// Unlike depth updates, value updates should NOT result in any radius changes.

	tcs := []testCase{
		{
			name: "depth increase",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, radius: 3},
				{depth: defaultDepth, value: defaultValue, radius: 4},
				{depth: defaultDepth, value: defaultValue, radius: 5},
			},
			update: []testBatch{
				{depth: defaultDepth + 1, value: defaultValue, radius: 5},
				{depth: defaultDepth + 1, value: defaultValue, radius: 6},
				{depth: defaultDepth + 1, value: defaultValue, radius: 6},
			},
		},
		{
			name: "value updates",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, radius: 3},
				{depth: defaultDepth, value: defaultValue, radius: 4},
				{depth: defaultDepth, value: defaultValue, radius: 5},
			},
			// since not depths are altered, radius should remain the same
			update: []testBatch{
				{depth: defaultDepth, value: defaultValue + 1, radius: 5},
				{depth: defaultDepth, value: defaultValue + 1, radius: 5},
				{depth: defaultDepth, value: defaultValue + 1, radius: 5},
			},
		},
		{
			name: "zero value updates",
			add: []testBatch{
				{depth: defaultDepth, value: defaultValue, radius: 3},
				{depth: defaultDepth, value: defaultValue, radius: 4},
				{depth: defaultDepth, value: defaultValue, radius: 5},
			},
			update: []testBatch{
				{depth: defaultDepth + 1, value: 0, radius: 4},
				{depth: defaultDepth + 1, value: 0, radius: 3},
				{depth: defaultDepth + 1, value: 0, radius: 0},
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
			checkState(t, tc.name, store, b.radius)

		}

		// update batches one by one with new depth and values and check, for each batch,
		// the allocated commitment matches the commitment values in the test case.
		for i, u := range tc.update {
			batch := batches[i]

			err := store.Update(batch, big.NewInt(int64(u.value)), u.depth)
			if err != nil {
				t.Fatalf("name: %s, %v", tc.name, err)
			}

			checkState(t, tc.name, store, u.radius)

		}
	}
}

// TestPutChainState add an initial group of batches to the batchstore, and after updating the chainstate,
// checks that the batchstore available and radius values.
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
				{depth: defaultDepth, value: 3, radius: 3},
				{depth: defaultDepth, value: 3, radius: 4},
				{depth: defaultDepth, value: 3, radius: 5},
			},
			// notice that after the chain state update, the new amount is 4,
			// which is greater than the values of the batches.
			// All the batches get evicted, and the new radius is zero.
			chain: []chainUpdate{
				{block: 1, amount: big.NewInt(4), radius: 0},
			},
		},
		{
			name: "evict all with two updates",
			add: []testBatch{
				{depth: defaultDepth, value: 3, radius: 3},
				{depth: defaultDepth, value: 4, radius: 4},
				{depth: defaultDepth, value: 5, radius: 5},
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
			checkState(t, tc.name, store, b.radius)
		}

		for _, c := range tc.chain {

			// update chain state
			err := store.PutChainState(&postage.ChainState{
				Block:        c.block,
				TotalAmount:  c.amount,
				CurrentPrice: big.NewInt(1),
			})
			if err != nil {
				t.Fatalf("name: %s, %v", tc.name, err)
			}

			checkState(t, tc.name, store, c.radius)
		}
	}
}

// TestUnreserve tests the Unreserve call increases the storage radius after each
// full iteration and that the storage radius never exceeds the global radius.
func TestUnreserve(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)
	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	store := setupBatchStore(t)

	_ = addBatch(t, store, initDepth, 1)
	_ = addBatch(t, store, initDepth, 2)
	_ = addBatch(t, store, initDepth, 2)

	state := store.GetReserveState()
	t.Log(state)

	cb := func([]byte, uint8) (bool, error) { return false, nil }

	// storage radius should equal storage radius and not exceed it
	for i := uint8(0); i <= state.Radius+1; i++ {

		wantStorageRadius := i
		if i > state.Radius {
			wantStorageRadius = state.Radius
		}

		if store.GetReserveState().StorageRadius != wantStorageRadius {
			t.Fatalf("got storage radius %d, want %d", store.GetReserveState().StorageRadius, wantStorageRadius)
		}

		_ = store.Unreserve(cb)
	}

	err := store.PutChainState(&postage.ChainState{
		Block:        1,
		TotalAmount:  big.NewInt(1),
		CurrentPrice: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}

	state = store.GetReserveState()
	t.Log(state)

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
	bStore.SetRadiusSetter(noopRadiusSetter{})

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
