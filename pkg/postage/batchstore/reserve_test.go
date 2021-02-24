// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"testing"

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
	// temporarily reset reserve Capacity
	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = batchstore.Exp2(16)

	bStore, unreserved := setupBatchStore(t)
	batches := make(map[string]*postage.Batch)

	t.Run("new batches only", func(t *testing.T) {
		// iterate starting from batchstore.DefaultDepth to maxPO
		_, depth := batchstore.GetReserve(bStore)
		for step := 0; depth < swarm.MaxPO; step++ {
			cs, err := nextChainState(bStore)
			if err != nil {
				t.Fatal(err)
			}
			var b *postage.Batch
			if b, err = createBatch(bStore, cs, depth); err != nil {
				t.Fatal(err)
			}
			batches[string(b.ID)] = b
			if depth, err = checkReserve(bStore, unreserved); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("top up batches", func(t *testing.T) {
		n := 0
		for id := range batches {
			b, err := bStore.Get([]byte(id))
			if err != nil {
				if errors.Is(storage.ErrNotFound, err) {
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
				if errors.Is(storage.ErrNotFound, err) {
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
	// temporarily reset reserve Capacity
	defer func(i int64) {
		batchstore.Capacity = i
	}(batchstore.Capacity)
	batchstore.Capacity = batchstore.Exp2(16)

	bStore, unreserved := setupBatchStore(t)
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
					if errors.Is(storage.ErrNotFound, err) {
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

	stateStore, err := leveldb.NewStateStore(dir)
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
		unreserved[string(batchID)] = radius
		return nil
	}
	bStore, _ := batchstore.New(stateStore, unreserveFunc)

	// initialise chainstate
	err = bStore.PutChainState(&postage.ChainState{
		Block: 666,
		Total: big.NewInt(0),
		Price: big.NewInt(1),
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
		Block: advance + cs.Block,
		Price: cs.Price,
		// settle although no price change
		Total: cs.Total.Add(cs.Total, new(big.Int).Mul(cs.Price, big.NewInt(int64(advance)))),
	}
	return cs, bStore.PutChainState(cs)
}

// creates a test batch with random value and depth and adds it to the batchstore
func createBatch(bStore postage.Storer, cs *postage.ChainState, depth uint8) (*postage.Batch, error) {
	b := postagetest.MustNewBatch()
	b.Depth = newBatchDepth(depth)
	value := newValue(cs.Price, cs.Total)
	b.Value = big.NewInt(0)
	return b, bStore.Put(b, value, b.Depth)
}

// tops up a batch with random amount
func topUp(bStore postage.Storer, cs *postage.ChainState, b *postage.Batch) error {
	value := newValue(cs.Price, b.Value)
	return bStore.Put(b, value, b.Depth)
}

// dilutes the batch with random factor
func increaseDepth(bStore postage.Storer, cs *postage.ChainState, b *postage.Batch) error {
	diff := newDilutionFactor()
	value := new(big.Int).Sub(b.Value, cs.Total)
	value.Div(value, big.NewInt(int64(1<<diff)))
	value.Add(value, cs.Total)
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
		bDepth, found := unreserved[string(b.ID)]
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
