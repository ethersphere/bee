// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"encoding/hex"
	"io"
	"math"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/storage"
)

type testBatch struct {
	depth uint8
	value int
}

// TestCommitments adds batches to the batchstore and after each batch, checks
// the allocated commitment with respect to batch depth and value. Also, checked are
// the reserve state radius and available values.
func TestCommitments(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	type testcase struct {
		add        []testBatch
		name       string
		commitment [][]int64 // keeps track of how much commitment is allocated for the batch at each step.
	}

	// Becase totalCapacity is 2^5, the node Capacity is 32 chunks.
	// Batches are added one by one, using different depth and value values,
	// and after each addition,
	// 1)	the commitment values inside the commitment slice
	// 		is checked against the commitment value allocated in the batchstore,
	// 2) 	the the sum of left over reservate state available and allocated
	// 		commitments should never exceed 32 chunks.
	// 3)	reserve state available remains a positive value

	// The first commitment is always 32 because the entire Capacity is allocated
	// to the first batch. As more batches are added, the Capacity is divided with respect
	// to depth and value.

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
			commitment: [][]int64{{32}, {16, 16}, {8, 8, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 1, value: 5},
			},
			commitment: [][]int64{{32}, {16, 16}, {8, 8, 16}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 2, value: 5},
			},
			commitment: [][]int64{{32}, {16, 16}, {4, 4, 16}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 2},
			},
			commitment: [][]int64{{32}, {16, 16}, {8, 16, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 2},
				{depth: initDepth, value: 4},
			},
			commitment: [][]int64{{32}, {16, 16}, {8, 8, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 5},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 3},
			},
			commitment: [][]int64{{32}, {16, 16}, {16, 8, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 5, value: 5},
			},
			commitment: [][]int64{{32}, {16, 16}, {0, 0, 16}},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		var (
			batches    []*postage.Batch
			commitment map[string]int64
		)

		for i, b := range tc.add {

			batches, commitment = saveNewBatch(t, tc.add[:i+1], batches, totalCapacity, store, b.depth, b.value)

			for i, c := range tc.commitment[i] {
				com := commitment[hex.EncodeToString(batches[i].ID)]
				if com != c {
					t.Fatalf("test %s: got commitment %v, want %v", tc.name, com, c)
				}
			}
		}
	}
}

// TestBatchUpdate adds an initial group of batches to the batchstore and one by one
// updates their depth and value to fields while checking their new commitment allocation
// value in the batchstore.
func TestBatchUpdate(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	type update struct {
		value      int
		depth      uint8
		index      int
		commitment []int64
	}

	type testcase struct {
		// the initial batches to add to the batchstore.
		add []testBatch
		// contains the new depth and value values for the added batches
		// and the new commitment values that should be allocated for the batches
		// after each update call.
		update []update
		name   string
	}

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
			},
			update: []update{
				// after updating the first batch (index 0), the batch commitment values should reflect
				// the commitment slice below. Notice, the updated batch's depth and value are higher so
				// more commitment is allocated than the rest.
				{index: 0, depth: initDepth + 1, value: 6, commitment: []int64{16, 8, 8}},
				{index: 1, depth: initDepth + 1, value: 6, commitment: []int64{16, 8, 8}},
				{index: 2, depth: initDepth + 2, value: 6, commitment: []int64{8, 8, 16}},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 1, value: 5},
			},
			update: []update{
				{index: 0, depth: initDepth + 1, value: 6, commitment: []int64{8, 8, 16}},
				{index: 1, depth: initDepth + 1, value: 6, commitment: []int64{8, 8, 16}},
				{index: 2, depth: initDepth + 1, value: 6, commitment: []int64{8, 8, 8}},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		var batches []*postage.Batch

		// add initial groupd of batches
		for i, b := range tc.add {
			batches, _ = saveNewBatch(t, tc.add[:i+1], batches, totalCapacity, store, b.depth, b.value)
		}

		// update batches one by one with new depth and values and check, for each batch,
		// the allocated commitment matches the commitment values in the test case.
		for _, u := range tc.update {
			batch := batches[u.index]

			err := store.Update(batch, big.NewInt(int64(u.value)), u.depth)
			if err != nil {
				t.Fatal(err)
			}

			state := store.GetReserveState()
			_, capacities := getCommitments(t, store, batches, state.Radius)

			if state.Available < 0 {
				t.Fatal("negative available")
			}

			for i, c := range u.commitment {
				cap := capacities[hex.EncodeToString(batches[i].ID)]
				if cap != c {
					t.Fatalf("test update %s: got commitment %v, want %v", tc.name, cap, c)
				}
			}
		}
	}
}

// TODO: add comments
func TestPutChainState(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	type chain struct {
		block      uint64
		amount     *big.Int
		commitment []int64
	}

	type testcase struct {
		add   []testBatch
		chain []chain
		// name  string
	}

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
			},
			chain: []chain{
				{block: 1, amount: big.NewInt(4), commitment: []int64{0, 0, 0}},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
			chain: []chain{
				{block: 1, amount: big.NewInt(4), commitment: []int64{0, 0, 32}},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		var batches []*postage.Batch

		for i, b := range tc.add {
			batches, _ = saveNewBatch(t, tc.add[:i+1], batches, totalCapacity, store, b.depth, b.value)
		}

		for _, c := range tc.chain {

			err := store.PutChainState(&postage.ChainState{
				Block:        c.block,
				TotalAmount:  c.amount,
				CurrentPrice: big.NewInt(1),
			})
			if err != nil {
				t.Fatal(err)
			}

			state := store.GetReserveState()

			for i, b := range batches {
				new, change, err := batchstore.BatchCommitment(store, b, state.Radius)

				if c.commitment[i] == 0 && err != storage.ErrNotFound {
					t.Fatal("batch should have been fully evicted")
				} else if c.commitment[i] != new+change {
					t.Fatalf("got commitment %d, want %d", new+change, c.commitment[i])
				}
			}
		}
	}
}

// TODO: add comments
func TestUnreserve(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	store := setupBatchStore(t)

	values := []int{3, 4, 5}

	_ = addBatch(t, store, initDepth, values[0])
	_ = addBatch(t, store, initDepth, values[1])
	_ = addBatch(t, store, initDepth, values[2])

	state := store.GetReserveState()

	cb := func([]byte, uint8) (bool, error) { return false, nil }

	for i := uint8(0); i <= state.Radius; i++ {
		if store.GetReserveState().StorageRadius != i {
			t.Fatalf("got storage radius %d, want %d", store.GetReserveState().StorageRadius, i)
		}
		_ = store.Unreserve(cb)
	}

	_ = store.Unreserve(cb)

	state = store.GetReserveState()
	if state.Radius != state.StorageRadius {
		t.Fatalf("radius %d does not match %d", state.Radius, state.StorageRadius)
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

func saveNewBatch(t *testing.T, testBatches []testBatch, batches []*postage.Batch, totalCapacity int64, store postage.Storer, depth uint8, value int) ([]*postage.Batch, map[string]int64) {

	t.Helper()

	newBatch := addBatch(t, store, depth, value)
	batches = append(batches, newBatch)

	radius := calcRadius(testBatches, batchstore.DefaultDepth)
	state := store.GetReserveState()

	if radius != state.Radius {
		t.Fatalf("got radius %v, want %v", state.Radius, radius)
	}
	if state.Available < 0 {
		t.Fatal("negative available")
	}

	sum, capacities := getCommitments(t, store, batches, radius)

	if sum+state.Available != totalCapacity {
		t.Fatalf("want %d, got %d", totalCapacity, sum+state.Available)
	}

	return batches, capacities
}

func calcRadius(batches []testBatch, depth uint8) uint8 {

	var total int64
	for _, b := range batches {
		total += batchstore.Exp2(uint(b.depth))
	}

	return uint8(math.Ceil(math.Log2(float64(total) / float64(batchstore.Exp2(uint(depth))))))
}

func getCommitments(t *testing.T, st postage.Storer, batches []*postage.Batch, radius uint8) (int64, map[string]int64) {

	t.Helper()

	m := make(map[string]int64)

	var sum int64

	for _, b := range batches {
		newCommitment, change, err := batchstore.BatchCommitment(st, b, radius)
		if err != nil {
			t.Fatal(err)
		}
		sum += change + newCommitment
		m[hex.EncodeToString(b.ID)] = change + newCommitment
	}

	return sum, m
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
