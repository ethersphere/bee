// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	postagetest "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// random advance on the blockchain
func newBlockAdvance() uint64 {
	return uint64(rand.Intn(3) + 1)
}

// initial depth of a new batch
func newBatchDepth(depth uint8) uint8 {
	return depth + uint8(rand.Intn(10)) + 4
}

// the factor to increase the batch depth with
func newDilutionFactor() int {
	return rand.Intn(3) + 1
}

// new value on top of value based on random period and price
func newValue(price, value *big.Int) *big.Int {
	period := rand.Intn(100) + 1000
	v := new(big.Int).Mul(price, big.NewInt(int64(period)))
	return v.Add(v, value)
}

// TestBatchStoreUnreserve is testing the correct behaviour of the reserve.
// the following assumptions are tested on each modification of the batches (top up, depth increase, price change)
// - reserve exceeds capacity
// - value-consistency of unreserved POs
func TestBatchStoreUnreserveEvents(t *testing.T) {
	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = batchstore.Exp2(16)

	bStore, unreserved := setupBatchStore(t)
	bStore.SetRadiusSetter(noopRadiusSetter{})
	batches := make(map[string]*postage.Batch)

	t.Run("new batches only", func(t *testing.T) {
		// iterate starting from batchstore.DefaultDepth to maxPO
		_, radius := batchstore.GetReserve(bStore)
		for step := 0; radius < swarm.MaxPO; step++ {
			cs, err := nextChainState(bStore)
			if err != nil {
				t.Fatal(err)
			}
			var b *postage.Batch
			if b, err = createBatch(bStore, cs, radius); err != nil {
				t.Fatal(err)
			}
			batches[string(b.ID)] = b
			if radius, err = checkReserve(bStore, unreserved); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("top up batches", func(t *testing.T) {
		n := 0
		for id := range batches {
			b, err := bStore.Get([]byte(id))
			if err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					continue
				}
				t.Fatal(err)
			}
			cs, err := nextChainState(bStore)
			if err != nil {
				t.Fatal(err)
			}
			if err = topUp(bStore, cs, b); err != nil {
				t.Fatal(err)
			}
			if _, err = checkReserve(bStore, unreserved); err != nil {
				t.Fatal(err)
			}
			n++
			if n > len(batches)/5 {
				break
			}
		}
	})
	t.Run("dilute batches", func(t *testing.T) {
		n := 0
		for id := range batches {
			b, err := bStore.Get([]byte(id))
			if err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					continue
				}
				t.Fatal(err)
			}
			cs, err := nextChainState(bStore)
			if err != nil {
				t.Fatal(err)
			}
			if err = increaseDepth(bStore, cs, b); err != nil {
				t.Fatal(err)
			}
			if _, err = checkReserve(bStore, unreserved); err != nil {
				t.Fatal(err)
			}
			n++
			if n > len(batches)/5 {
				break
			}
		}
	})
}

func TestBatchStoreUnreserveAll(t *testing.T) {
	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = batchstore.Exp2(16)

	bStore, unreserved := setupBatchStore(t)
	bStore.SetRadiusSetter(noopRadiusSetter{})
	var batches [][]byte
	// iterate starting from batchstore.DefaultDepth to maxPO
	_, depth := batchstore.GetReserve(bStore)
	for step := 0; depth < swarm.MaxPO; step++ {
		cs, err := nextChainState(bStore)
		if err != nil {
			t.Fatal(err)
		}
		event := rand.Intn(6)
		//  0:  dilute, 1: topup, 2,3,4,5: create
		var b *postage.Batch
		if event < 2 && len(batches) > 10 {
			for {
				n := rand.Intn(len(batches))
				b, err = bStore.Get(batches[n])
				if err != nil {
					if errors.Is(err, storage.ErrNotFound) {
						continue
					}
					t.Fatal(err)
				}
				break
			}
			if event == 0 {
				if err = increaseDepth(bStore, cs, b); err != nil {
					t.Fatal(err)
				}
			} else if err = topUp(bStore, cs, b); err != nil {
				t.Fatal(err)
			}
		} else if b, err = createBatch(bStore, cs, depth); err != nil {
			t.Fatal(err)
		} else {
			batches = append(batches, b.ID)
		}
		if depth, err = checkReserve(bStore, unreserved); err != nil {
			t.Fatal(err)
		}
	}
}

func setupBatchStore(t *testing.T) (postage.Storer, map[string]uint8) {
	t.Helper()
	// we cannot  use the mock statestore here since the iterator is not giving the right order
	// must use the leveldb statestore
	dir, err := ioutil.TempDir("", "batchstore_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	})
	logger := logging.New(ioutil.Discard, 0)
	stateStore, err := leveldb.NewStateStore(dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stateStore.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// set mock unreserve call
	unreserved := make(map[string]uint8)
	unreserveFunc := func(batchID []byte, radius uint8) error {
		unreserved[hex.EncodeToString(batchID)] = radius
		return nil
	}
	bStore, _ := batchstore.New(stateStore, unreserveFunc)
	bStore.SetRadiusSetter(noopRadiusSetter{})

	// initialise chainstate
	err = bStore.PutChainState(&postage.ChainState{
		Block:        0,
		TotalAmount:  big.NewInt(0),
		CurrentPrice: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	return bStore, unreserved
}

func nextChainState(bStore postage.Storer) (*postage.ChainState, error) {
	cs := bStore.GetChainState()
	// random advance on the blockchain
	advance := newBlockAdvance()
	cs = &postage.ChainState{
		Block:        advance + cs.Block,
		CurrentPrice: cs.CurrentPrice,
		// settle although no price change
		TotalAmount: cs.TotalAmount.Add(cs.TotalAmount, new(big.Int).Mul(cs.CurrentPrice, big.NewInt(int64(advance)))),
	}
	return cs, bStore.PutChainState(cs)
}

// creates a test batch with random value and depth and adds it to the batchstore
func createBatch(bStore postage.Storer, cs *postage.ChainState, depth uint8) (*postage.Batch, error) {
	b := postagetest.MustNewBatch()
	b.Depth = newBatchDepth(depth)
	value := newValue(cs.CurrentPrice, cs.TotalAmount)
	b.Value = big.NewInt(0)
	return b, bStore.Put(b, value, b.Depth)
}

// tops up a batch with random amount
func topUp(bStore postage.Storer, cs *postage.ChainState, b *postage.Batch) error {
	value := newValue(cs.CurrentPrice, b.Value)
	return bStore.Put(b, value, b.Depth)
}

// dilutes the batch with random factor
func increaseDepth(bStore postage.Storer, cs *postage.ChainState, b *postage.Batch) error {
	diff := newDilutionFactor()
	value := new(big.Int).Sub(b.Value, cs.TotalAmount)
	value.Div(value, big.NewInt(int64(1<<diff)))
	value.Add(value, cs.TotalAmount)
	return bStore.Put(b, value, b.Depth+uint8(diff))
}

// checkReserve is testing the correct behaviour of the reserve.
// the following assumptions are tested on each modification of the batches (top up, depth increase, price change)
// - reserve exceeds capacity
// - value-consistency of unreserved POs
func checkReserve(bStore postage.Storer, unreserved map[string]uint8) (uint8, error) {
	var size int64
	count := 0
	outer := big.NewInt(0)
	inner := big.NewInt(0)
	limit, depth := batchstore.GetReserve(bStore)
	// checking all batches
	err := batchstore.IterateAll(bStore, func(b *postage.Batch) (bool, error) {
		count++
		bDepth, found := unreserved[hex.EncodeToString(b.ID)]
		if !found {
			return true, fmt.Errorf("batch not unreserved")
		}
		if b.Value.Cmp(limit) >= 0 {
			if bDepth < depth-1 || bDepth > depth {
				return true, fmt.Errorf("incorrect reserve radius. expected %d or %d. got  %d", depth-1, depth, bDepth)
			}
			if bDepth == depth {
				if inner.Cmp(b.Value) < 0 {
					inner.Set(b.Value)
				}
			} else if outer.Cmp(b.Value) > 0 || outer.Cmp(big.NewInt(0)) == 0 {
				outer.Set(b.Value)
			}
			if outer.Cmp(big.NewInt(0)) != 0 && outer.Cmp(inner) <= 0 {
				return true, fmt.Errorf("inconsistent reserve radius: %d <= %d", outer.Uint64(), inner.Uint64())
			}
			size += batchstore.Exp2(b.Depth - bDepth - 1)
		} else if bDepth != swarm.MaxPO {
			return true, fmt.Errorf("batch below limit expected to be fully unreserved. got found=%v, radius=%d", found, bDepth)
		}
		return false, nil
	})
	if err != nil {
		return 0, err
	}
	if size > batchstore.Capacity {
		return 0, fmt.Errorf("reserve size beyond capacity. max %d, got %d", batchstore.Capacity, size)
	}
	return depth, nil
}

// TestBatchStore_Unreserve tests that the unreserve
// hook is called with the correct batch IDs and correct
// Radius as a result of batches coming in from chain events.
// All tests share the same initial state:
//		▲ bzz/chunk
//		│
//	6	├──┐
//	5	│  ├──┐
//	4	│  │  ├──┐
//	3	│  │  │  ├──┐---inner, outer
//		│  │  │  │  │
//		└──┴──┴──┴──┴───────> time
//
func TestBatchStore_Unreserve(t *testing.T) {
	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = batchstore.Exp2(5) // 32 chunks
	// 8 is the initial batch depth we add the initial state batches with.
	// the default radius is 5 (defined in reserve.go file), which means there
	// are 2^5 neighborhoods. now, since there are 2^8 chunks in a batch (256),
	// we can divide that by the number of neighborhoods (32) and get 8, which is
	// the number of chunks at most that can fall inside a neighborhood for a batch
	initBatchDepth := uint8(8)

	for _, tc := range []struct {
		desc string
		add  []depthValueTuple
		exp  []batchUnreserveTuple
	}{
		{
			// add one batch with value 2 and expect that it will be called in
			// evict with radius 5, which means that the outer half of chunks from
			// that batch will be deleted once chunks start entering the localstore.
			// inner 2, outer 4
			desc: "add one at inner",
			add:  []depthValueTuple{depthValue(8, 2)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 4),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5)},
		}, {
			// add one batch with value 3 and expect that it will be called in
			// evict with radius 5 alongside with the other value 3 batch
			// inner 3, outer 4
			desc: "add another at inner",
			add:  []depthValueTuple{depthValue(8, 3)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 4),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5)},
		}, {
			// add one batch with value 4 and expect that the batch with value
			// 3 gets called with radius 5, and BOTH batches with value 4 will
			// also be called with radius 5.
			// inner 3, outer 5
			desc: "add one at inner and evict half of self",
			add:  []depthValueTuple{depthValue(8, 4)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5)},
		}, {
			// this builds on the previous case:
			// since we over-evicted one batch before (since both 4's ended up in
			// inner, then we can add another one at 4, and expect it also to be
			// at inner (called with 5).
			// inner 3, outer 5 (stays the same)
			desc: "add one at inner and fill after over-eviction",
			add:  []depthValueTuple{depthValue(8, 4), depthValue(8, 4)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5),
				batchUnreserve(5, 5),
			},
		}, {
			// insert a batch of depth 6 (2 chunks fall under our radius)
			// value is 3, expect unreserve 5, expect other value 3 to be
			// at radius 5.
			// inner 3, outer 4
			desc: "insert smaller at inner",
			add:  []depthValueTuple{depthValue(6, 3)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 4),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5),
			},
		}, {
			// this case builds on the previous one:
			// because we over-evicted, we can insert another batch of depth 6
			// with value 3, expect unreserve 5
			// inner 3, outer 4
			desc: "insert smaller and fill over-eviction",
			add:  []depthValueTuple{depthValue(6, 3), depthValue(6, 3)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 4),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5),
				batchUnreserve(5, 5),
			},
		}, {
			// insert a batch of depth 6 (2 chunks fall under our radius)
			// value is 4, expect unreserve 5, expect other value 3 to be
			// at radius 5.
			// inner 3, outer 4
			desc: "insert smaller and evict cheaper",
			add:  []depthValueTuple{depthValue(6, 4)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 4),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// insert a batch of depth 6 (2 chunks fall under our radius)
			// value is 6, expect unreserve 4, expect other value 3 to be
			// at radius 5.
			// inner 3, outer 4
			desc: "insert at outer and evict inner",
			add:  []depthValueTuple{depthValue(6, 6)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 4),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// insert a batch of depth 9 (16 chunks in outer tier)
			// expect batches with value 3 and 4 to be unreserved with radius 5
			// including the one that was just added (evicted half of itself)
			// inner 3, outer 5
			desc: "insert at inner and evict self and sister batches",
			add:  []depthValueTuple{depthValue(9, 3)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5),
			},
		}, {
			// insert a batch of depth 9 (16 chunks in outer tier)
			// expect batches with value 3 and 4 to be unreserved with radius 5
			// state is same as the last case
			// inner 3, outer 5
			desc: "insert at inner and evict self and sister batches",
			add:  []depthValueTuple{depthValue(9, 4)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5),
			},
		}, {
			// insert a batch of depth 9 (16 chunks in outer tier), and 7 (8 chunks in premium)
			// expect batches with value 3 to 5 to be unreserved with radius 5
			// inner 3, outer 6
			desc: "insert at outer and evict inner",
			add:  []depthValueTuple{depthValue(9, 5), depthValue(7, 5)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5),
				batchUnreserve(5, 5),
			},
		}, {
			// insert a batch of depth 10 value 3 (32 chunks in outer tier)
			// expect all batches to be called with radius 5!
			// inner 3, outer 3
			desc: "insert at outer and evict everything to fit the batch",
			add:  []depthValueTuple{depthValue(10, 3)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 5),
				batchUnreserve(4, 5),
			},
		}, {
			// builds on the last case:
			// insert a batch of depth 10 value 3 (32 chunks in outer tier)
			// and of depth 7 value 3. expect value 3's to be called with radius 6
			// inner 3, outer 4
			desc: "insert another at outer and expect evict self",
			add:  []depthValueTuple{depthValue(10, 3), depthValue(7, 3)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 6),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 5),
				batchUnreserve(4, 6),
				batchUnreserve(5, 6),
			},
		}, {
			// insert a batch of depth 10 value 6 (32 chunks in outer tier)
			// expect all batches to be called with unreserved 5
			// inner 3, outer 3
			desc: "insert at outer and evict from all to fit the batch",
			add:  []depthValueTuple{depthValue(10, 6)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 5),
				batchUnreserve(4, 5),
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			store, unreserved := setupBatchStore(t)
			store.SetRadiusSetter(noopRadiusSetter{})
			batches := addBatch(t, store,
				depthValue(initBatchDepth, 3),
				depthValue(initBatchDepth, 4),
				depthValue(initBatchDepth, 5),
				depthValue(initBatchDepth, 6),
			)

			checkUnreserved(t, unreserved, batches, 4)

			b := addBatch(t, store, tc.add...)
			batches = append(batches, b...)

			for _, v := range tc.exp {
				b := []*postage.Batch{batches[v.batchIndex]}
				checkUnreserved(t, unreserved, b, v.expDepth)
			}
		})
	}
}

// TestBatchStore_Topup tests that the unreserve
// hook is called with the correct batch IDs and correct
// Radius as a result of batches being topped up.
// All tests share the same initial state:
//		▲ bzz/chunk
//		│
//	6	├──┐
//	5	│  ├──┐
//	4	│  │  ├──┐
//	3	│  │  │  ├──┐
//	2	│  │  │  │  ├──┐---inner, outer
//		└──┴──┴──┴──┴──┴─────> time
//
func TestBatchStore_Topup(t *testing.T) {
	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = batchstore.Exp2(5) // 32 chunks
	initBatchDepth := uint8(8)

	for _, tc := range []struct {
		desc  string
		topup []batchValueTuple
		exp   []batchUnreserveTuple
	}{
		{
			// initial state
			// inner 2, outer 4
			desc: "initial state",
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// builds on initial state:
			// topup of batch with value 2 to value 3 should result
			// in no state change.
			// inner 3, outer 4. before the topup: inner 2, outer 4
			desc:  "topup value 2->3, same state",
			topup: []batchValueTuple{batchValue(0, 3)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// topup of batch with value 2 to value 4 should result
			// in the other batches (3,4) in being downgraded to inner too, so all three batches are
			// at inner. there's excess capacity
			// inner 3, outer 5
			desc:  "topup value 2->4, same state",
			topup: []batchValueTuple{batchValue(0, 4)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// builds on the last case:
			// add another batch at value 2, and since we've over-evicted before,
			// we should be able to accommodate it.
			// inner 3, outer 5
			desc:  "topup value 2->4, add another one at 2, same state",
			topup: []batchValueTuple{batchValue(0, 4)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// builds on the last case:
			// add another batch at value 2, and since we've over-evicted before,
			// we should be able to accommodate it.
			// inner 3, outer 5
			desc:  "topup value 2->4, add another one at 2, same state",
			topup: []batchValueTuple{batchValue(0, 4)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			store, unreserved := setupBatchStore(t)
			store.SetRadiusSetter(noopRadiusSetter{})
			batches := addBatch(t, store,
				depthValue(initBatchDepth, 2),
				depthValue(initBatchDepth, 3),
				depthValue(initBatchDepth, 4),
				depthValue(initBatchDepth, 5),
				depthValue(initBatchDepth, 6),
			)

			topupBatch(t, store, batches, tc.topup...)

			for _, v := range tc.exp {
				b := []*postage.Batch{batches[v.batchIndex]}
				checkUnreserved(t, unreserved, b, v.expDepth)
			}
		})
	}
}

// TestBatchStore_Dilution tests that the unreserve
// hook is called with the correct batch IDs and correct
// Radius as a result of batches being diluted.
// All tests share the same initial state:
//		▲ bzz/chunk
//		│
//	6	├──┐
//	5	│  ├──┐
//	4	│  │  ├──┐
//	3	│  │  │  ├──┐
//	2	│  │  │  │  ├──┐---inner, outer
//		└──┴──┴──┴──┴──┴─────> time
//
func TestBatchStore_Dilution(t *testing.T) {
	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = batchstore.Exp2(5) // 32 chunks
	initBatchDepth := uint8(8)

	for _, tc := range []struct {
		desc   string
		dilute []batchDepthTuple
		topup  []batchValueTuple
		exp    []batchUnreserveTuple
	}{
		{
			// initial state
			// inner 2, outer 4
			desc: "initial state",
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// dilution halves the value, and doubles the size of the batch
			// recalculate the per chunk balance:
			// ((value - total) / 2) + total => new batch value

			// expect this batch to be called with unreserved 5.
			// the batch collected the outer half, so in fact when it was
			// diluted it got downgraded from inner to outer, so it preserves
			// the same amount of chunks. the rest stays the same

			// total is 0 at this point

			desc:   "dilute most expensive",
			dilute: []batchDepthTuple{batchDepth(4, 9)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 4),
				batchUnreserve(3, 4),
				batchUnreserve(4, 5),
			},
		}, {
			// expect this batch to be called with unreserved 5, but also the
			// the rest of the batches to be evicted with radius 5 to fit this batch in
			desc:   "dilute most expensive further, evict batch from outer",
			dilute: []batchDepthTuple{batchDepth(4, 10)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 5),
				batchUnreserve(4, 5),
			},
		}, {
			// dilute the batch at value 3, expect to evict out the
			// batch with value 4 to radius 5
			desc:   "dilute cheaper batch and evict batch from outer",
			dilute: []batchDepthTuple{batchDepth(1, 9)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 4),
				batchUnreserve(4, 4),
			},
		}, {
			// top up the highest value batch to be value 12, then dilute it
			// to be depth 9 (original 8), which causes it to be at value 6
			// expect batches with value 4 and 5 to evict outer, and the last
			// batch to be at outer tier (radius 4)
			// inner 2, outer 6
			desc:   "dilute cheaper batch and evict batch from outer",
			topup:  []batchValueTuple{batchValue(4, 12)},
			dilute: []batchDepthTuple{batchDepth(4, 9)},
			exp: []batchUnreserveTuple{
				batchUnreserve(0, 5),
				batchUnreserve(1, 5),
				batchUnreserve(2, 5),
				batchUnreserve(3, 5),
				batchUnreserve(4, 4),
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			store, unreserved := setupBatchStore(t)
			store.SetRadiusSetter(noopRadiusSetter{})
			batches := addBatch(t, store,
				depthValue(initBatchDepth, 2),
				depthValue(initBatchDepth, 3),
				depthValue(initBatchDepth, 4),
				depthValue(initBatchDepth, 5),
				depthValue(initBatchDepth, 6),
			)

			topupBatch(t, store, batches, tc.topup...)
			diluteBatch(t, store, batches, tc.dilute...)

			for _, v := range tc.exp {
				b := []*postage.Batch{batches[v.batchIndex]}
				checkUnreserved(t, unreserved, b, v.expDepth)
			}
		})
	}
}

func TestBatchStore_EvictExpired(t *testing.T) {
	defer func(i int64, d uint8) {
		batchstore.Capacity = i
		batchstore.DefaultDepth = d
	}(batchstore.Capacity, batchstore.DefaultDepth)
	batchstore.DefaultDepth = 5
	batchstore.Capacity = batchstore.Exp2(5) // 32 chunks
	initBatchDepth := uint8(8)

	store, unreserved := setupBatchStore(t)
	store.SetRadiusSetter(noopRadiusSetter{})
	batches := addBatch(t, store,
		depthValue(initBatchDepth, 2),
		depthValue(initBatchDepth, 3),
		depthValue(initBatchDepth, 4),
		depthValue(initBatchDepth, 5),
	)

	cs := store.GetChainState()
	cs.Block = 4
	cs.TotalAmount = big.NewInt(4)
	err := store.PutChainState(cs)
	if err != nil {
		t.Fatal(err)
	}

	// expect the 5 to be preserved and the rest to be unreserved
	checkUnreserved(t, unreserved, batches[:3], swarm.MaxPO+1)
	checkUnreserved(t, unreserved, batches[3:], 4)

	// check that the batches is actually deleted from
	// statestore, by trying to do a Get on the deleted
	// batches, and assert that they are not found
	for _, v := range batches[:3] {
		_, err := store.Get(v.ID)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected err not found but got %v", err)
		}
	}
}

type depthValueTuple struct {
	depth uint8
	value int
}

func depthValue(d uint8, v int) depthValueTuple {
	return depthValueTuple{depth: d, value: v}
}

type batchValueTuple struct {
	batchIndex int
	value      *big.Int
}

func batchValue(i, v int) batchValueTuple {
	return batchValueTuple{batchIndex: i, value: big.NewInt(int64(v))}
}

type batchUnreserveTuple struct {
	batchIndex int
	expDepth   uint8
}

func batchUnreserve(i int, d uint8) batchUnreserveTuple {
	return batchUnreserveTuple{batchIndex: i, expDepth: d}
}

type batchDepthTuple struct {
	batchIndex int
	depth      uint8
}

func batchDepth(i, d int) batchDepthTuple {
	return batchDepthTuple{batchIndex: i, depth: uint8(d)}
}

func topupBatch(t *testing.T, s postage.Storer, batches []*postage.Batch, bvp ...batchValueTuple) {
	t.Helper()
	for _, v := range bvp {
		batch := batches[v.batchIndex]
		err := s.Put(batch, v.value, batch.Depth)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func diluteBatch(t *testing.T, s postage.Storer, batches []*postage.Batch, bdp ...batchDepthTuple) {
	t.Helper()
	for _, v := range bdp {
		batch := batches[v.batchIndex]
		val := batch.Value
		// for every depth increase we half the batch value
		for i := batch.Depth; i < v.depth; i++ {
			val = big.NewInt(0).Div(val, big.NewInt(2))
		}
		err := s.Put(batch, val, v.depth)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func addBatch(t *testing.T, s postage.Storer, dvp ...depthValueTuple) []*postage.Batch {
	t.Helper()
	var batches []*postage.Batch
	for _, v := range dvp {
		b := postagetest.MustNewBatch()

		// this is needed since the initial batch state should be
		// always zero. should be rectified with less magical test
		// helpers
		b.Value = big.NewInt(0)
		b.Depth = uint8(0)
		b.Start = 111

		val := big.NewInt(int64(v.value))

		err := s.Put(b, val, v.depth)
		if err != nil {
			t.Fatal(err)
		}
		batches = append(batches, b)
	}

	return batches
}

func checkUnreserved(t *testing.T, unreserved map[string]uint8, batches []*postage.Batch, exp uint8) {
	t.Helper()
	for _, b := range batches {
		v, ok := unreserved[hex.EncodeToString(b.ID)]
		if !ok {
			t.Fatalf("batch %x not called with unreserve", b.ID)
		}
		if v != exp {
			t.Fatalf("batch %x expected unreserve radius %d but got %d", b.ID, exp, v)
		}
	}
}
