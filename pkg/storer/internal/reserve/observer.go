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
//   - The observer is registered once, is never nil, and only while the
//     address index is enabled. A second registration is rejected by an
//     atomic flag before the registration lock is taken. With the address
//     index disabled no observer can exist, so the registration lock is
//     skipped entirely.
//   - Every set-changing transaction of Put, EvictBatchBin and Reset loads
//     the observer and runs under the read side of a reserve-level
//     registration lock; registration takes the write side, so it returns
//     only after every unwrapped transaction has committed. The read lock
//     is taken per transaction, in the goroutine that runs it; it is never
//     held across eg.Wait and never taken twice by one goroutine. Lock
//     order: r.multx, registration lock (read), observer lock (read, inside
//     Commit), chunk locks.
//   - Commit wraps each transaction that may change the set exactly once
//     and is never called from inside another Commit's run. EvictBatchBin
//     and Reset wrap each per-item transaction, never the whole loop.
//   - Commit may be called with r.multx locks and the registration read
//     lock held, concurrently from several goroutines.
//   - run returns an exact delta. It returns an error and an empty delta
//     only when the leveldb batch was not written; an error reported after
//     the batch commit (sharky release) is kept out of run, which returns
//     the full delta, and is returned to the caller after Commit. This
//     needs post-commit errors of transaction.Commit to be identifiable.
//   - SetRadius stores the radius, then loads the observer through a
//     synchronized load and calls RadiusChanged with no reserve lock held;
//     Reset loads it the same way and calls Reset at the end of a
//     successful Reset.
type Observer interface {
	Commit(run func() (adds, removes []ChunkID, err error)) error
	RadiusChanged(radius uint8)
	Reset()
}
