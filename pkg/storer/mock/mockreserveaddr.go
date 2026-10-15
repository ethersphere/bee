// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The address index of the mock (storer.ReserveAddrStore) is an in-memory
// model of the storer's for protocol tests:
//   - The set is the mock's stamped chunks, one element per
//     (address, stamp hash). Chunks given with WithChunks (with a stamp)
//     are in the set from the start and are not reported to the observer;
//     chunks put through ReservePutter enter it, and RemoveID and
//     ResetReserve remove elements.
//   - With an observer registered every change of the set goes through its
//     Commit, one call per Put, RemoveID and removed element of
//     ResetReserve, with the exact delta, and with no lock of the mock held
//     (run takes it); ResetReserve then calls its Reset, and
//     SetStorageRadius its RadiusChanged.
//   - ReserveIterateAddr iterates a copy of the matching elements taken
//     when it is called and holds no lock of the mock while cb runs, so cb
//     may call other methods of the mock. It does not model leveldb
//     snapshots, eviction locking or the registration guarantee: test those
//     against the storer on transaction.NewStorage over leveldbstore, never
//     against this mock.
//   - There is no replacement by newer stamps and no radius filtering: the
//     mock holds whatever is put into it.

var _ storer.ReserveAddrStore = (*ReserveStore)(nil)

// errNotImplemented is returned by the address index methods until the mock
// implements them.
var errNotImplemented = errors.New("mockstorer: address index not implemented")

// WithAddrIndex enables the mock's address index. It is ready
// (ReserveAddrIndexReady reports true) until SetAddrIndexReady(false). Without
// this option the storer.ReserveAddrStore methods that return an error return
// storer.ErrAddrIndexDisabled and ReserveAddrIndexReady reports false.
func WithAddrIndex() Option {
	return optionFunc(func(p *ReserveStore) {
		p.addrIndex = true
		p.addrIndexReady = true
	})
}

// SetAddrIndexReady sets what ReserveAddrIndexReady reports while the address
// index is enabled, to test callers that wait for a fill (rangesync answers
// Busy). The set is not changed.
func (s *ReserveStore) SetAddrIndexReady(ready bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.addrIndexReady = ready
}

// ReserveIterateAddr implements storer.ReserveAddrStore.
func (s *ReserveStore) ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(storer.ChunkID) (stop bool, err error)) error {
	return s.addrIndexErr()
}

// ReserveGetByID implements storer.ReserveAddrStore. It resolves the batch ID
// from the address index and returns ReserveGet's result, so an evil chunk
// (WithEvilChunk) is returned for its address.
func (s *ReserveStore) ReserveGetByID(ctx context.Context, addr swarm.Address, stampHash []byte) (swarm.Chunk, error) {
	return nil, s.addrIndexErr()
}

// ReserveHasID implements storer.ReserveAddrStore.
func (s *ReserveStore) ReserveHasID(addr swarm.Address, stampHash []byte) (bool, error) {
	return false, s.addrIndexErr()
}

// ReserveAddrIndexReady implements storer.ReserveAddrStore.
func (s *ReserveStore) ReserveAddrIndexReady() bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.addrIndex && s.addrIndexReady
}

// SetReserveObserver implements storer.ReserveAddrStore, with the storer's
// errors: storer.ErrAddrIndexDisabled, storer.ErrNilObserver and
// storer.ErrObserverRegistered.
func (s *ReserveStore) SetReserveObserver(o storer.ReserveObserver) error {
	return s.addrIndexErr()
}

// RemoveID removes the stamped chunk (addr, stampHash) from the mock, as an
// eviction would, and reports the removal to the observer, if any. Removing a
// chunk that is not in the mock is a no-op (an empty delta). It works with
// the address index enabled or not and returns the observer's Commit error.
func (s *ReserveStore) RemoveID(addr swarm.Address, stampHash []byte) error {
	return errNotImplemented
}

// ResetReserve removes every chunk, as the storer's ResetReserve does: with an
// observer registered each removed stamped chunk goes through its Commit,
// then its Reset is called. It works with the address index enabled or not.
func (s *ReserveStore) ResetReserve(ctx context.Context) error {
	return errNotImplemented
}

func (s *ReserveStore) addrIndexErr() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if !s.addrIndex {
		return storer.ErrAddrIndexDisabled
	}
	return errNotImplemented
}
