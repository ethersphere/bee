// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"encoding/hex"
	"io"
	"math"
	"math/big"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
)

func setupBatchStore(t *testing.T) postage.Storer {
	t.Helper()
	// we cannot  use the mock statestore here since the iterator is not giving the right order
	// must use the leveldb statestore
	dir, err := os.MkdirTemp("", "batchstore_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	})
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

type testBatch struct {
	depth uint8
	value int
}

func TestCapacityChange(t *testing.T) {

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
		capacities [][]int64
	}

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
			capacities: [][]int64{{32}, {16, 16}, {8, 8, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 1, value: 5},
			},
			capacities: [][]int64{{32}, {16, 16}, {8, 8, 16}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 2, value: 5},
			},
			capacities: [][]int64{{32}, {16, 16}, {4, 4, 16}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 2},
			},
			capacities: [][]int64{{32}, {16, 16}, {8, 16, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 2},
				{depth: initDepth, value: 4},
			},
			capacities: [][]int64{{32}, {16, 16}, {8, 8, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 5},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 3},
			},
			capacities: [][]int64{{32}, {16, 16}, {16, 8, 8}},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 5, value: 5},
			},
			capacities: [][]int64{{32}, {16, 16}, {0, 0, 16}},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		store.SetRadiusSetter(noopRadiusSetter{})

		var batches []*postage.Batch

		for i, b := range tc.add {

			newBatch := addBatch(t, store, depthValue(b.depth, b.value))
			batches = append(batches, newBatch)
			radius := calcRadius(tc.add[:i+1], batchstore.DefaultDepth)
			state := store.GetReserveState()
			if radius != state.Radius {
				t.Fatalf("got radius %v, want %v", state.Radius, radius)
			}
			if state.Available < 0 {
				t.Fatal("negative available")
			}

			sum, capacities := getCapacities(t, store, batches, radius)
			t.Log(state)
			t.Log(capacities)

			if sum+state.Available != totalCapacity {
				t.Fatalf("want %d, got %d", totalCapacity, sum+state.Available)
			}

			for i, c := range tc.capacities[i] {
				cap := capacities[hex.EncodeToString((batches[i].ID))]
				if cap != c {
					t.Fatalf("test %s: got capacity %v, want %v", tc.name, cap, c)
				}
			}
		}
	}
}

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
		capacities []int64
	}

	type testcase struct {
		add    []testBatch
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
				{index: 0, depth: initDepth + 1, value: 6, capacities: []int64{16, 8, 8}},
				{index: 1, depth: initDepth + 1, value: 6, capacities: []int64{16, 8, 8}},
				{index: 2, depth: initDepth + 2, value: 6, capacities: []int64{8, 8, 16}},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 1, value: 5},
			},
			update: []update{
				{index: 0, depth: initDepth + 1, value: 6, capacities: []int64{8, 8, 16}},
				{index: 1, depth: initDepth + 1, value: 6, capacities: []int64{8, 8, 16}},
				{index: 2, depth: initDepth + 1, value: 6, capacities: []int64{8, 8, 8}},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		store.SetRadiusSetter(noopRadiusSetter{})

		var batches []*postage.Batch

		for i, b := range tc.add {

			newBatch := addBatch(t, store, depthValue(b.depth, b.value))
			batches = append(batches, newBatch)
			radius := calcRadius(tc.add[:i+1], batchstore.DefaultDepth)
			state := store.GetReserveState()
			if radius != state.Radius {
				t.Fatalf("got radius %v, want %v", state.Radius, radius)
			}
			if state.Available < 0 {
				t.Fatal("negative available")
			}

			sum, capacities := getCapacities(t, store, batches, radius)
			t.Log(state)
			t.Log(capacities)

			if sum+state.Available != totalCapacity {
				t.Fatalf("want %d, got %d", totalCapacity, sum+state.Available)
			}
		}

		for _, u := range tc.update {
			batch := batches[u.index]

			big.NewInt(int64(u.value))

			err := store.Update(batch, big.NewInt(int64(u.value)), u.depth)
			if err != nil {
				t.Fatal(err)
			}

			state := store.GetReserveState()
			_, capacities := getCapacities(t, store, batches, state.Radius)

			if state.Available < 0 {
				t.Fatal("negative available")
			}

			t.Log(state)
			t.Log(capacities)

			for i, c := range u.capacities {
				cap := capacities[hex.EncodeToString((batches[i].ID))]
				if cap != c {
					t.Fatalf("test update %s: got capacity %v, want %v", tc.name, cap, c)
				}
			}
		}
	}
}

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
		capacities []int64
	}

	type testcase struct {
		add   []testBatch
		chain []chain
		name  string
	}

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 3},
			},
			chain: []chain{
				{block: 1, amount: big.NewInt(4), capacities: []int64{0, 0, 0}},
			},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
			chain: []chain{
				{block: 1, amount: big.NewInt(4), capacities: []int64{0, 0, 32}},
			},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		store.SetRadiusSetter(noopRadiusSetter{})

		var batches []*postage.Batch

		for i, b := range tc.add {

			newBatch := addBatch(t, store, depthValue(b.depth, b.value))
			batches = append(batches, newBatch)
			radius := calcRadius(tc.add[:i+1], batchstore.DefaultDepth)
			state := store.GetReserveState()
			if radius != state.Radius {
				t.Fatalf("got radius %v, want %v", state.Radius, radius)
			}
			if state.Available < 0 {
				t.Fatal("negative available")
			}

			sum, capacities := getCapacities(t, store, batches, radius)
			t.Log(state)
			t.Log(capacities)

			if sum+state.Available != totalCapacity {
				t.Fatalf("want %d, got %d", totalCapacity, sum+state.Available)
			}
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
			_, capacities := getCapacities(t, store, batches, state.Radius)

			if state.Available < 0 {
				t.Fatal("negative available")
			}

			t.Log(state)
			t.Log(capacities)

			for i, c := range c.capacities {
				cap := capacities[hex.EncodeToString((batches[i].ID))]
				if cap != c {
					t.Fatalf("test update %s: got capacity %v, want %v", tc.name, cap, c)
				}
			}
		}
	}
}

func TestUnreserve(t *testing.T) {

	totalCapacity := batchstore.Exp2(5)

	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = totalCapacity

	initDepth := uint8(8)

	type testcase struct {
		add            []testBatch
		evictionRadius []uint8
	}

	tcs := []testcase{
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 5},
			},
			evictionRadius: []uint8{5, 5, 5},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 1, value: 5},
			},
			evictionRadius: []uint8{5, 5, 5},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth + 2, value: 5},
			},
			evictionRadius: []uint8{6, 6, 6},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 2},
			},
			evictionRadius: []uint8{5, 4, 5},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 3},
				{depth: initDepth, value: 2},
				{depth: initDepth, value: 4},
			},
			evictionRadius: []uint8{5, 5, 5},
		},
		{
			add: []testBatch{
				{depth: initDepth, value: 5},
				{depth: initDepth, value: 4},
				{depth: initDepth, value: 3},
			},
			evictionRadius: []uint8{4, 5, 5},
		},
	}

	for _, tc := range tcs {

		store := setupBatchStore(t)

		store.SetRadiusSetter(noopRadiusSetter{})

		var batches []*postage.Batch

		for _, b := range tc.add {
			newBatch := addBatch(t, store, depthValue(b.depth, b.value))
			batches = append(batches, newBatch)
		}

		evictionRadiuses := map[string]uint8{}
		err := store.Unreserve(func(id []byte, radius uint8) (bool, error) {
			evictionRadiuses[hex.EncodeToString(id)] = radius
			t.Log("~ Unreserve", hex.EncodeToString(id), radius)
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}

		for i, b := range batches {
			if evictionRadiuses[hex.EncodeToString(b.ID)] != tc.evictionRadius[i] {
				t.Fatalf("want %d, got %d", tc.evictionRadius[i], evictionRadiuses[hex.EncodeToString(b.ID)])
			}
		}
	}
}

func calcRadius(batches []testBatch, depth uint8) uint8 {

	var total int64
	for _, b := range batches {
		total += batchstore.Exp2(uint(b.depth))
	}

	return uint8(math.Ceil(math.Log2(float64(total) / float64(batchstore.Exp2(uint(depth))))))
}

func getCapacities(t *testing.T, st postage.Storer, batches []*postage.Batch, radius uint8) (int64, map[string]int64) {
	m := make(map[string]int64)

	var sum int64

	for _, b := range batches {
		newCapacity, change, err := batchstore.BatchCapacity(st, b, radius)
		if err != nil {
			t.Fatal(err)
		}
		sum += change + newCapacity
		m[hex.EncodeToString(b.ID)] = change + newCapacity
	}

	return sum, m
}

type depthValueTuple struct {
	depth uint8
	value int
}

func depthValue(d uint8, v int) depthValueTuple {
	return depthValueTuple{depth: d, value: v}
}

func addBatch(t *testing.T, s postage.Storer, dvp depthValueTuple) *postage.Batch {
	batch := postagetest.MustNewBatch(
		postagetest.WithValue(int64(dvp.value)),
		postagetest.WithDepth(dvp.depth),
		postagetest.WithStart(111),
	)
	if err := s.Save(batch); err != nil {
		t.Fatal(err)
	}

	return batch
}
