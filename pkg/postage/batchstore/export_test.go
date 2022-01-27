// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"fmt"

	"github.com/ethersphere/bee/pkg/postage"
)

// ChainStateKey is the statestore key for the chain state.
const StateKey = chainStateKey

var (
	BatchKey = batchKey
	ValueKey = valueKey
)

// power of 2 function
var Exp2 = exp2

func (s *store) String() string {
	return fmt.Sprintf("inner=%d,outer=%d", s.rs.Inner.Uint64(), s.rs.Outer.Uint64())
}

func SetUnreserveFunc(s postage.Storer, fn func([]byte, uint8) error) {
	st := s.(*store)
	st.unreserveFn = fn
}
