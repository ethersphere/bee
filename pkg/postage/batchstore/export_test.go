// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"fmt"
	"math/big"

	"github.com/ethersphere/bee/pkg/postage"
)

// ChainStateKey is the localstore key for the chain state.
const StateKey = chainStateKey

// BatchKey returns the index key for the batch ID used in the by-ID batch index.
var BatchKey = batchKey

// power of 2 function
var Exp2 = exp2

// iterates through all batches
func IterateAll(bs postage.Storer, f func(b *postage.Batch) (bool, error)) error {
	s := bs.(*store)
	return s.store.Iterate(batchKeyPrefix, func(key []byte, _ []byte) (bool, error) {
		b, err := s.Get(key[len(key)-32:])
		if err != nil {
			return true, err
		}
		return f(b)
	})
}

// GetReserve extracts the inner limit and depth of reserve
func GetReserve(si postage.Storer) (*big.Int, uint8) {
	s, _ := si.(*store)
	return s.rs.Inner, s.rs.Radius
}

func (s *store) String() string {
	return fmt.Sprintf("inner=%d,outer=%d", s.rs.Inner.Uint64(), s.rs.Outer.Uint64())
}
