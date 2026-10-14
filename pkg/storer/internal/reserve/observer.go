// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import "github.com/ethersphere/bee/v2/pkg/swarm"

// ChunkID identifies one (chunk, stamp) pair in the reserve: one element of
// the set kept by the address index (ChunkAddrItem). Its identity is
// (Address, StampHash); BatchID is carried along so a consumer can resolve
// the chunk without a lookup. It is exported by the storer as
// storer.ChunkID.
type ChunkID struct {
	Address   swarm.Address
	StampHash []byte
	BatchID   []byte
}

// Observer is the reserve's view of storer.ReserveObserver: the method sets
// are identical and the storer checks at compile time that the two
// interfaces are interchangeable. The full contract is documented on
// storer.ReserveObserver. The reserve's side of it:
//   - Commit wraps each set-changing transaction exactly once and is never
//     called from inside another Commit's run. EvictBatchBin and Reset wrap
//     each per-item transaction, never the whole loop.
//   - Commit may be called with r.multx locks held, concurrently from
//     several goroutines.
//   - run returns an exact delta, and an empty one when the transaction
//     failed.
//   - RadiusChanged is called from SetRadius only, Reset at the end of a
//     successful Reset. Neither is called with r.multx locks held.
type Observer interface {
	Commit(run func() (adds, removes []ChunkID, err error)) error
	RadiusChanged(radius uint8)
	Reset()
}
