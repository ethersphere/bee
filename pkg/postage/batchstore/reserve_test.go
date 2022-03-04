// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"io"
	"math"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
)

type testBatch struct {
	depth uint8
	value int
}

// TestBatchSave adds batches to the batchstore, and after each batch, checks
// the reserve state radius and available values.
func TestBatchSave(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	type testcase struct {
		add []testBatch
	}

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 1, value: 5},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 2, value: 5},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 2},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 2},
				{depth: initDepth, value: 4},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 5},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 3},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 3, value: 5},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		for _, b := range tc.add {
			_ = addBatch(t, store, b.depth, b.value)
			checkState(t, store)
		}
	}
}

// TestBatchUpdate adds an initial group of batches to the batchstore and one by one
// updates their depth and value to fields while checking the batchstore available and radius values.
func TestBatchUpdate(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	type update struct {
		value int
		depth uint8
		index int
	}

	type testcase struct {
		// the initial batches to add to the batchstore.
		add []testBatch
		// update contains the new depth and value values for the added batches
		update []update
	}

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
			update: []update{
				{index: 0, depth: initDepth + 1, value: 6},
				{index: 1, depth: initDepth + 1, value: 6},
				{index: 2, depth: initDepth + 1, value: 6},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 2, value: 5},
			},
			update: []update{
				{index: 0, depth: initDepth + 2, value: 6},
				{index: 1, depth: initDepth + 2, value: 6},
				{index: 2, depth: initDepth + 2, value: 6},
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
			checkState(t, store)
		}

		// update batches one by one with new depth and values and check, for each batch,
		// the allocated commitment matches the commitment values in the test case.
		for _, u := range tc.update {
			batch := batches[u.index]

			err := store.Update(batch, big.NewInt(int64(u.value)), u.depth)
			if err != nil {
				t.Fatal(err)
			}

			checkState(t, store)
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

	initDepth := uint8(8)

	type chain struct {
		block  uint64
		amount *big.Int
	}

	type testcase struct {
		add   []testBatch
		chain []chain
	}

	tcs := []testcase{
		{
			// initial group of batches to add
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
			},
			// chain state update, notice that after the update, the amount is 4,
			// which is greater than the values of the batches, the new commitment
			// amounts are zero.
			chain: []chain{
				{block: 1, amount: big.NewInt(4)},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
			chain: []chain{
				{block: 1, amount: big.NewInt(4)},
				{block: 2, amount: big.NewInt(5)},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		// add the group of batches
		for _, b := range tc.add {
			_ = addBatch(t, store, b.depth, b.value)
			checkState(t, store)
		}

		for _, c := range tc.chain {

			// update chain state
			err := store.PutChainState(&postage.ChainState{
				Block:        c.block,
				TotalAmount:  c.amount,
				CurrentPrice: big.NewInt(1),
			})
			if err != nil {
				t.Fatal(err)
			}

			checkState(t, store)
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
	_ = addBatch(t, store, initDepth, 3)

	state := store.GetReserveState()

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

func checkState(t *testing.T, store postage.Storer) {

	t.Helper()

	totalCommitment := calcCommitment(store)
	radius := calcRadius(totalCommitment, batchstore.Capacity)

	state := store.GetReserveState()

	if state.StorageRadius > state.Radius {
		t.Fatalf("storage radius %d must not exceed radius %d", state.StorageRadius, state.Radius)
	}

	if radius != state.Radius {
		t.Fatalf("got radius %v, want %v", state.Radius, radius)
	}
}

func calcCommitment(store postage.Storer) int64 {
	var total int64
	_ = store.Iterate(func(b *postage.Batch) (bool, error) {
		total += batchstore.Exp2(uint(b.Depth))
		return false, nil
	})
	return total
}

func calcRadius(totalCommitment int64, capacity int64) uint8 {
	return uint8(math.Ceil(math.Log2(float64(totalCommitment) / float64(capacity))))
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
