// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

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

// SetObserver registers the single observer of the reserve's set changes. It
// fails with ErrAddrIndexDisabled when the address index is disabled, with
// ErrNilObserver for a nil observer and with ErrObserverRegistered when one is
// already registered; the last check doesn't wait for in-flight transactions.
// It returns once every set-changing transaction that started without the
// observer has committed; every transaction started afterwards goes through
// o.Commit.
func (r *Reserve) SetObserver(o Observer) error {
	if !r.addrIndex {
		return ErrAddrIndexDisabled
	}
	if o == nil {
		return ErrNilObserver
	}
	if !r.obsRegister.CompareAndSwap(false, true) {
		return ErrObserverRegistered
	}
	r.obsMu.Lock()
	r.observer = o
	r.obsMu.Unlock()
	return nil
}

// loadObserver returns the registered observer, or nil.
func (r *Reserve) loadObserver() Observer {
	if !r.addrIndex {
		return nil
	}
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	return r.observer
}

// run executes one transaction that may change the reserve's set. f receives
// the transaction and the address index writer for it, which is nil when the
// address index is disabled. With an observer registered the transaction is
// wrapped by Commit and its exact delta is reported.
//
// run takes the registration read lock for the duration of the transaction,
// in the calling goroutine. Callers must not hold it already, nor wait for
// other run calls while inside f.
func (r *Reserve) run(ctx context.Context, f func(transaction.Store, *addrTx) error) error {
	if !r.addrIndex {
		return r.st.Run(ctx, func(s transaction.Store) error { return f(s, nil) })
	}

	r.obsMu.RLock()
	defer r.obsMu.RUnlock()

	if r.observer == nil {
		return r.st.Run(ctx, func(s transaction.Store) error { return f(s, addrIndexOnly) })
	}

	// An error reported after the batch was written (a sharky release) is
	// kept out of the observer, which must apply the committed delta, and
	// returned to the caller afterwards.
	var afterCommitErr error
	err := r.observer.Commit(func() (adds, removes []ChunkID, err error) {
		tx := &addrTx{record: true}
		err = r.st.Run(ctx, func(s transaction.Store) error { return f(s, tx) })
		switch {
		case errors.Is(err, transaction.ErrAfterCommit):
			afterCommitErr = err
		case err != nil:
			return nil, nil, err
		}
		return tx.adds, tx.removes, nil
	})
	if err != nil {
		return err
	}
	return afterCommitErr
}

// addrTx writes the ChunkAddrItems of one transaction and, when record is
// set, collects the exact delta of the set. A nil *addrTx means the address
// index is disabled and every method is a no-op.
type addrTx struct {
	record  bool
	adds    []ChunkID
	removes []ChunkID
}

// addrIndexOnly maintains the address index without recording a delta. It is
// shared and, since record is false, never written to.
var addrIndexOnly = &addrTx{}

func addrTxFor(addrIndex bool) *addrTx {
	if addrIndex {
		return addrIndexOnly
	}
	return nil
}

// put writes the ChunkAddrItem of (addr, stampHash) and records +id if no
// entry existed.
func (tx *addrTx) put(s storage.IndexStore, addr swarm.Address, stampHash, batchID []byte, chunkType swarm.ChunkType) error {
	if tx == nil {
		return nil
	}
	item := &ChunkAddrItem{Address: addr, StampHash: stampHash, BatchID: batchID, ChunkType: chunkType}
	var existed bool
	if tx.record {
		var err error
		if existed, err = s.Has(item); err != nil {
			return fmt.Errorf("check chunk address item: %w", err)
		}
	}
	if err := s.Put(item); err != nil {
		return fmt.Errorf("put chunk address item: %w", err)
	}
	if tx.record && !existed {
		tx.adds = append(tx.adds, ChunkID{Address: addr, StampHash: stampHash, BatchID: batchID})
	}
	return nil
}

// delete deletes the ChunkAddrItem of (addr, stampHash) if it exists and then
// records -id. An absent entry (an interrupted or running fill) records
// nothing.
func (tx *addrTx) delete(s storage.IndexStore, addr swarm.Address, stampHash, batchID []byte) error {
	if tx == nil {
		return nil
	}
	item := &ChunkAddrItem{Address: addr, StampHash: stampHash}
	has, err := s.Has(item)
	if err != nil {
		return fmt.Errorf("check chunk address item: %w", err)
	}
	if !has {
		return nil
	}
	if err := s.Delete(item); err != nil {
		return fmt.Errorf("delete chunk address item: %w", err)
	}
	if tx.record {
		tx.removes = append(tx.removes, ChunkID{Address: addr, StampHash: stampHash, BatchID: batchID})
	}
	return nil
}
