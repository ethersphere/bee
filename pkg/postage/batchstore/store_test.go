// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore_test

import (
	"math/big"
	"testing"

	"github.com/ethersphere/bee/pkg/postage/batchstore"
	"github.com/ethersphere/bee/pkg/statestore/mock"
)

func TestStore(t *testing.T) {

}

func TestStoreMarshalling(t *testing.T) {
	mockstore := mock.NewStateStore()
	store, err := batchstore.New(mockstore)
	if err != nil {
		t.Fatal(err)
	}

	newPrice := big.NewInt(99)
	store.UpdatePrice(newPrice)

	store.Settle(1100)

	store2, err := batchstore.New(mockstore)
	if err != nil {
		t.Fatal(err)
	}
	if store2.Price().Cmp(newPrice) != 0 {
		t.Fatal("price not persisted")
	}

	expTotal := big.NewInt(0 + 99*1100)
	if store2.Total().Cmp(expTotal) != 0 {
		t.Fatal("value mismatch")
	}
}
