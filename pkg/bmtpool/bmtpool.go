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

const Capacity = 32

var instance *bmt.Pool

func init() {
	instance = bmt.NewPool(bmt.NewConf(swarm.NewHasher, swarm.BmtBranches, Capacity))
}

// Get a bmt Hasher instance.
// Instances are reset before being returned to the caller.
func Get() *bmt.Hasher {
	return instance.Get()
}

// Put a bmt Hasher back into the pool
func Put(h *bmt.Hasher) {
	instance.Put(h)
}
