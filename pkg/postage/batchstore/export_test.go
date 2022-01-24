// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"github.com/ethersphere/bee/pkg/postage"
)

// ChainStateKey is the statestore key for the chain state.
const StateKey = chainStateKey

// BatchKey returns the index key for the batch ID used in the by-ID batch index.
var BatchKey = batchKey

// power of 2 function
var Exp2 = exp2

// func (s *store) String() string {
// 	return fmt.Sprintf("inner=%d,outer=%d", s.rs.Inner.Uint64(), s.rs.Outer.Uint64())
// }

func SetUnreserveFunc(s postage.Storer, fn func([]byte, uint8) error) {
	st := s.(*store)

	unreserveFn := st.unreserve
	st.unreserveFn = func(batchID []byte, radius uint8) error {

		fn(batchID, radius)
		return unreserveFn(batchID, radius)

	}
}
