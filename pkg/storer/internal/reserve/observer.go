// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import "github.com/ethersphere/bee/v2/pkg/swarm"

// ChunkID identifies one (chunk, stamp) pair in the reserve: one element of
// the set kept by the address index (ChunkAddrItem). Its identity is
// (Address, StampHash) only; BatchID is always set and carried along so a
// consumer can resolve the chunk without a lookup. The byte slices may alias
// reserve or caller memory and are read-only. It is exported by the storer
// as storer.ChunkID.
type ChunkID struct {
	Address   swarm.Address
	StampHash []byte
	BatchID   []byte
}

// Observer is the reserve's view of storer.ReserveObserver: the method sets
// are identical and the storer checks at compile time that the two
// interfaces are interchangeable. The full contract is documented on
// storer.ReserveObserver. The reserve's side of it:
//   - The observer is registered once and is never nil. Every set-changing
//     path of Put, EvictBatchBin and Reset loads the observer and runs its
//     transaction under the read side of a reserve-level registration lock;
//     registration takes the write side, so it returns only after every
//     unwrapped transaction has committed. Lock order: r.multx,
//     registration lock (read), observer lock (read, inside Commit), chunk
//     locks.
//   - Commit wraps each transaction that may change the set exactly once
//     and is never called from inside another Commit's run. EvictBatchBin
//     and Reset wrap each per-item transaction, never the whole loop.
//   - Commit may be called with r.multx locks and the registration read
//     lock held, concurrently from several goroutines.
//   - run returns an exact delta, and an empty one when the transaction
//     failed.
//   - RadiusChanged is called at the end of every SetRadius with no reserve
//     lock held; Reset at the end of a successful Reset.
type Observer interface {
	Commit(run func() (adds, removes []ChunkID, err error)) error
	RadiusChanged(radius uint8)
	Reset()
}
