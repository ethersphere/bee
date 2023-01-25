// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bmtpool provides easy access to binary
// merkle tree hashers managed in as a resource pool.
package bmtpool

import (
	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/swarm"
)

var trInstance *bmt.TrPool

// nolint:gochecknoinits
func trInit(key []byte) {
	trInstance = bmt.NewTrPool(bmt.NewTrConf(swarm.NewHasher, key, swarm.BmtBranches, Capacity))
}

// Get a bmt Hasher instance.
// Instances are reset before being returned to the caller.
func TrGet() *bmt.TrHasher {
	return trInstance.Get()
}

// Put a bmt Hasher back into the pool
func TrPut(h *bmt.TrHasher) {
	trInstance.Put(h)
}
