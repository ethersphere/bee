// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package batchstore

import (
	"github.com/ethersphere/bee/pkg/postage"
)

// ChainStateKey is the statestore key for the chain state.
const StateKey = chainStateKey
const BatchstoreVersion = batchstoreVersion

var (
	BatchKey = batchKey
	ValueKey = valueKey
)

var Exp2 = exp2

func BatchCommitment(s postage.Storer, b *postage.Batch, radius uint8) (int64, int64, error) {

	st := s.(*store)
	item, err := st.getValueItem(b)

	if err != nil {
		return 0, 0, err
	}

	newCommitment := exp2(uint(b.Depth - radius))
	change := st.commitment(b.Depth, item.Radius, radius)
	return newCommitment, change, err
}
