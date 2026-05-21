// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bmtpool provides easy access to binary
// merkle tree hashers managed in as a resource pool.
package bmtpool

import (
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const Capacity = 32

// instance holds the active bmt.Pool. Using an atomic pointer so Rebuild can
// swap it in after cmd/bee calls bmt.SetSIMDOptIn during startup, without
// introducing lock contention on the hot Get/Put path.
var instance atomic.Pointer[bmt.Pool]

// proverInstance holds the Prover pool. Goroutine-backed by construction
// (see bmt.NewProverPool) and intentionally not affected by Rebuild or the
// SIMD opt-in flag: proofs run on a rare, redistribution-only code path where
// the well-tested goroutine implementation is preferred.
var proverInstance bmt.ProverPool

// nolint:gochecknoinits
func init() {
	// Eager init: construct the pool at package load time so its internal
	// channel is created outside any testing/synctest bubble. Tests that use
	// synctest would otherwise trip "receive on synctest channel from outside
	// bubble" if the pool were first created inside a bubble.
	//
	// bmt.SIMDOptIn() is still false here (flag hasn't been parsed). cmd/bee
	// calls Rebuild after flag parsing if the user opted in.
	p := bmt.NewPool(bmt.NewConf(swarm.BmtBranches, Capacity))
	instance.Store(&p)
	proverInstance = bmt.NewProverPool(bmt.NewConf(swarm.BmtBranches, Capacity))
}

// Rebuild discards the current pool and constructs a new one reading the
// latest bmt.SIMDOptIn() value. Must be called exactly once during startup
// after CLI flag parsing and before the first external Get, so that
// --use-simd-hashing takes effect and no in-flight hashers are referencing the
// old pool's tree. The Prover pool is intentionally not rebuilt.
func Rebuild() {
	p := bmt.NewPool(bmt.NewConf(swarm.BmtBranches, Capacity))
	instance.Store(&p)
}

// Get a bmt Hasher instance. Instances are reset before being returned to the caller.
func Get() bmt.Hasher {
	return (*instance.Load()).Get()
}

// Put a bmt Hasher back into the pool.
func Put(h bmt.Hasher) {
	(*instance.Load()).Put(h)
}

// GetProver returns a goroutine-backed Prover from the global prover pool.
func GetProver() *bmt.Prover {
	return proverInstance.GetProver()
}

// PutProver returns a Prover to the global prover pool for reuse.
func PutProver(p *bmt.Prover) {
	proverInstance.PutProver(p)
}
