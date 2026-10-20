// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
)

var _ Local = (*local)(nil)

// local is the Local of the sessions of a Service: summaries from the
// fingerprint index, and IDs from the storer's address index. Summaries of
// ranges below the index leaf depth are computed by scanning the address
// index; a session asks for them only for opening ranges at most
// LeafDepthSlack bits below the session leaf depth, so a scan reads at most
// one bucket.
type local struct {
	index *fingerprint.Index
	store Store
}

// Every error of local wraps ErrLocal, except the error of an IterateIDs
// callback: its errors come from this node's index and reserve.

// Summary implements Summarizer. Errors of the index other than
// fingerprint.ErrBelowLeaf (ErrBusy while the range is being rebuilt,
// ErrOutsideRoot after the root moved) are returned wrapped.
func (l *local) Summary(r fingerprint.Range) (fingerprint.Summary, error) {
	s, err := l.index.Summary(r)
	if errors.Is(err, fingerprint.ErrBelowLeaf) {
		return l.scan(r)
	}
	if err != nil {
		return fingerprint.Summary{}, fmt.Errorf("%w: index summary: %w", ErrLocal, err)
	}
	return s, nil
}

// scan returns the summary of r from the address index.
func (l *local) scan(r fingerprint.Range) (fingerprint.Summary, error) {
	var (
		h fingerprint.LtHash
		n uint64
	)
	err := l.store.ReserveIterateAddr(r.Prefix, r.Bits, nil, func(c storer.ChunkID) (bool, error) {
		h.Add(fingerprint.NewID(c.Address, c.StampHash))
		n++
		return false, nil
	})
	if err != nil {
		return fingerprint.Summary{}, fmt.Errorf("%w: scan: %w", ErrLocal, err)
	}
	if n == 0 {
		return fingerprint.Summary{}, nil
	}
	return fingerprint.Summary{Count: n, Digest: h.Digest()}, nil
}

// IterateIDs implements Local with one ReserveIterateAddr call. fn's error is
// returned as the reserve returns it; another error of the reserve wraps
// ErrLocal.
func (l *local) IterateIDs(r fingerprint.Range, after *fingerprint.ID, fn func(fingerprint.ID) (bool, error)) error {
	var a []byte
	if after != nil {
		a = after[:]
	}
	var fnErr error
	err := l.store.ReserveIterateAddr(r.Prefix, r.Bits, a, func(c storer.ChunkID) (bool, error) {
		stop, err := fn(fingerprint.NewID(c.Address, c.StampHash))
		fnErr = err
		return stop, err
	})
	if err != nil && (fnErr == nil || !errors.Is(err, fnErr)) {
		return fmt.Errorf("%w: iterate ids: %w", ErrLocal, err)
	}
	return err
}

// Symbols implements Local over fingerprint.Index.Symbols, whose scan path
// is one ReserveIterateAddr call over the range. Errors are returned wrapped.
func (l *local) Symbols(r fingerprint.Range, start, end uint64) ([]fingerprint.CodedSymbol, bool, error) {
	syms, exhausted, err := l.index.Symbols(r, start, end, l.scanIDs)
	if err != nil {
		return nil, false, fmt.Errorf("%w: index symbols: %w", ErrLocal, err)
	}
	return syms, exhausted, nil
}

// maxScanIDs bounds a Symbols scan. The index checks count(r) ≤ ScanLimit
// before the scan, which runs unlocked, so concurrent puts can grow r; the
// bound keeps the cost of a scan within twice that.
const maxScanIDs = 2 * fingerprint.ScanLimit

// errScanLimit is returned by a Symbols scan of more than maxScanIDs IDs.
var errScanLimit = errors.New("rangesync: symbols scan over limit")

// scanIDs calls fn with the IDs of the address index in r until fn returns
// an error, which scanIDs returns, or until it yielded maxScanIDs IDs, when
// it returns errScanLimit.
func (l *local) scanIDs(r fingerprint.Range, fn func(fingerprint.ID) error) error {
	n := 0
	return l.store.ReserveIterateAddr(r.Prefix, r.Bits, nil, func(c storer.ChunkID) (bool, error) {
		if n++; n > maxScanIDs {
			return true, errScanLimit
		}
		return false, fn(fingerprint.NewID(c.Address, c.StampHash))
	})
}

// HasID implements Local.
func (l *local) HasID(id fingerprint.ID) (bool, error) {
	has, err := l.store.ReserveHasID(id.Address(), id.StampHash())
	if err != nil {
		return false, fmt.Errorf("%w: reserve has id: %w", ErrLocal, err)
	}
	return has, nil
}
