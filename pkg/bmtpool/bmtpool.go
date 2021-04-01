// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bmtpool provides easy access to binary
// merkle tree hashers managed in as a resource pool.
package bmtpool

import (
	bmtlegacy "github.com/ethersphere/bee/pkg/bmt/legacy"
	"github.com/ethersphere/bee/pkg/bmt/pool"
	"github.com/ethersphere/bee/pkg/swarm"
)

var instance pool.Pooler

func init() {
	instance = pool.New(8, swarm.BmtBranches)
}

// Get a bmt Hasher instance.
// Instances are reset before being returned to the caller.
func Get() *bmtlegacy.Hasher {
	return instance.Get()
}

// Put a bmt Hasher back into the pool
func Put(h *bmtlegacy.Hasher) {
	instance.Put(h)
}
