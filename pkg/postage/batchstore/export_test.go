// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"github.com/ethersphere/bee/pkg/postage"
)

// ChainStateKey is the statestore key for the chain state.
const StateKey = chainStateKey

var (
	BatchKey = batchKey
	ValueKey = valueKey
)

var Exp2 = exp2

func BatchCapacity(s postage.Storer, b *postage.Batch, evictionRadius uint8) (int64, int64, error) {

	st := s.(*store)
	item, err := st.getUnreserveItem(b.ID)

	if err != nil {
		return 0, 0, err
	}

	newCapacity, change := st.capacity(b, item, evictionRadius)
	return newCapacity, change, err
}
