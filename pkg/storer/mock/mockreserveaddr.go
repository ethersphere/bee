// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/ethersphere/bee/v2/pkg/storage"
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
//   - Putting a chunk whose (address, stamp hash) is already in the set
//     still goes through Commit, with an empty delta.
//   - There is no replacement by newer stamps and no radius filtering: the
//     mock holds whatever is put into it, and IsWithinStorageRadius always
//     reports true, so out-of-radius cases (T1.10) cannot be tested with
//     the mock.
//   - WithEvilChunk affects ReserveGetByID only for an address that is in
//     the set: the batch ID is resolved from the set first.

var _ storer.ReserveAddrStore = (*ReserveStore)(nil)

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
// Busy). The set is not changed. It is a test switch: the storer's readiness
// only ever goes from false to true, so tests that model the storer start
// with SetAddrIndexReady(false) and later set true, never back.
func (s *ReserveStore) SetAddrIndexReady(ready bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.addrIndexReady = ready
}

func elementID(addr swarm.Address, stampHash []byte) string {
	return addr.ByteString() + string(stampHash)
}

// hasBitPrefix reports whether the first bits bits of addr equal those of
// prefix; missing prefix bytes count as zero.
func hasBitPrefix(addr, prefix []byte, bits uint8) bool {
	for i := range int(bits) {
		var p byte
		if i/8 < len(prefix) {
			p = prefix[i/8]
		}
		if (addr[i/8]>>(7-i%8))&1 != (p>>(7-i%8))&1 {
			return false
		}
	}
	return true
}

// ReserveIterateAddr implements storer.ReserveAddrStore.
func (s *ReserveStore) ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(storer.ChunkID) (stop bool, err error)) error {
	s.mtx.Lock()
	if !s.addrIndex {
		s.mtx.Unlock()
		return storer.ErrAddrIndexDisabled
	}
	var ids []storer.ChunkID
	for id, key := range s.ids {
		addr := []byte(id[:swarm.HashSize])
		if !hasBitPrefix(addr, prefix.Bytes(), bits) {
			continue
		}
		if after != nil && bytes.Compare([]byte(id), after) <= 0 {
			continue
		}
		ids = append(ids, storer.ChunkID{
			Address:   swarm.NewAddress(addr),
			StampHash: []byte(id[swarm.HashSize:]),
			BatchID:   s.chunks[key].Stamp().BatchID(),
		})
	}
	s.mtx.Unlock()

	slices.SortFunc(ids, func(a, b storer.ChunkID) int {
		if c := bytes.Compare(a.Address.Bytes(), b.Address.Bytes()); c != 0 {
			return c
		}
		return bytes.Compare(a.StampHash, b.StampHash)
	})
	for _, id := range ids {
		stop, err := cb(id)
		if err != nil {
			return fmt.Errorf("mockstorer: iterate address index: %w", err)
		}
		if stop {
			return nil
		}
	}
	return nil
}

// ReserveGetByID implements storer.ReserveAddrStore. It resolves the batch ID
// from the address index and returns ReserveGet's result, so an evil chunk
// (WithEvilChunk) is returned for its address.
func (s *ReserveStore) ReserveGetByID(ctx context.Context, addr swarm.Address, stampHash []byte) (swarm.Chunk, error) {
	s.mtx.Lock()
	if !s.addrIndex {
		s.mtx.Unlock()
		return nil, storer.ErrAddrIndexDisabled
	}
	key, ok := s.ids[elementID(addr, stampHash)]
	var batchID []byte
	if ok {
		batchID = s.chunks[key].Stamp().BatchID()
	}
	s.mtx.Unlock()
	if !ok {
		return nil, fmt.Errorf("mockstorer: get by id: %w", storage.ErrNotFound)
	}
	return s.ReserveGet(ctx, addr, batchID, stampHash)
}

// ReserveHasID implements storer.ReserveAddrStore.
func (s *ReserveStore) ReserveHasID(addr swarm.Address, stampHash []byte) (bool, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if !s.addrIndex {
		return false, storer.ErrAddrIndexDisabled
	}
	_, ok := s.ids[elementID(addr, stampHash)]
	return ok, nil
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
	s.mtx.Lock()
	defer s.mtx.Unlock()
	switch {
	case !s.addrIndex:
		return storer.ErrAddrIndexDisabled
	case o == nil:
		return storer.ErrNilObserver
	case s.observer != nil:
		return storer.ErrObserverRegistered
	}
	s.observer = o
	return nil
}

// remove deletes the stamped chunk of the element and reports it, if present.
func (s *ReserveStore) remove(id string) []storer.ChunkID {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	key, ok := s.ids[id]
	if !ok {
		return nil
	}
	ch := s.chunks[key]
	delete(s.chunks, key)
	delete(s.ids, id)
	return []storer.ChunkID{{
		Address:   swarm.NewAddress([]byte(id[:swarm.HashSize])),
		StampHash: []byte(id[swarm.HashSize:]),
		BatchID:   ch.Stamp().BatchID(),
	}}
}

// removeReported removes the element, through the observer's Commit if one is
// registered.
func (s *ReserveStore) removeReported(id string) error {
	s.mtx.Lock()
	o := s.observer
	s.mtx.Unlock()
	if o == nil {
		s.remove(id)
		return nil
	}
	return o.Commit(func() (adds, removes []storer.ChunkID, err error) {
		return nil, s.remove(id), nil
	})
}

// RemoveID removes the stamped chunk (addr, stampHash) from the mock, as an
// eviction would, and reports the removal to the observer, if any. Removing a
// chunk that is not in the mock is a no-op (an empty delta). It works with
// the address index enabled or not and returns the observer's Commit error.
func (s *ReserveStore) RemoveID(addr swarm.Address, stampHash []byte) error {
	return s.removeReported(elementID(addr, stampHash))
}

// ResetReserve removes every chunk, as the storer's ResetReserve does: with an
// observer registered each removed stamped chunk goes through its Commit,
// then its Reset is called. It works with the address index enabled or not.
func (s *ReserveStore) ResetReserve(ctx context.Context) error {
	s.mtx.Lock()
	ids := make([]string, 0, len(s.ids))
	for id := range s.ids {
		ids = append(ids, id)
	}
	s.mtx.Unlock()

	for _, id := range ids {
		if err := s.removeReported(id); err != nil {
			return err
		}
	}

	s.mtx.Lock()
	clear(s.chunks)
	o := s.observer
	s.mtx.Unlock()
	if o != nil {
		o.Reset()
	}
	return nil
}
