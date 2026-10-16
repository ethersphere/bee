// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"fmt"
	"unsafe"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// bucketSymbols is the Group adapter of the bucket symbol Fenwick tree: the
// first BucketSymbols coded symbols of a bucket, added symbol by symbol.
type bucketSymbols [BucketSymbols]CodedSymbol

// Add sets b to b plus o.
func (b *bucketSymbols) Add(o *bucketSymbols) {
	for i := range b {
		b[i].Add(&o[i])
	}
}

// Sub sets b to b minus o.
func (b *bucketSymbols) Sub(o *bucketSymbols) {
	for i := range b {
		b[i].Sub(&o[i])
	}
}

// Zero sets b to the symbols of the empty set.
func (b *bucketSymbols) Zero() { *b = bucketSymbols{} }

// level1Symbols and rootSymbols are the symbol caches of the 1<<FanoutBits
// level-1 ranges and of the root.
type (
	level1Symbols [1 << FanoutBits][Level1Symbols]CodedSymbol
	rootSymbols   [RootSymbols]CodedSymbol
)

// Sizes of the symbol caches (proposal §5.3: 11.5 MB of bucket symbols at
// B = 4096, 0.7 MB level-1, 0.36 MB root).
const (
	bucketSymbolsSize = int(unsafe.Sizeof(bucketSymbols{}))
	level1SymbolsSize = int(unsafe.Sizeof(level1Symbols{}))
	rootSymbolsSize   = int(unsafe.Sizeof(rootSymbols{}))
)

// expand sets sm to the expansion of id with the given sign: its LtHash, its
// coded symbol {id, Checksum(id), sign} and its symbol indices below
// RootSymbols, drawn once from the Index's Mapper.
func (ix *Index) expand(sm *sums, id ID, sign int) {
	sm.hash = ltHashNode(Expand(nil, id))
	sm.sym = CodedSymbol{Sum: id, Check: Checksum(id), Count: int64(sign)}
	sm.idx = sm.idx[:0]
	it := ix.mapper.Indices(id)
	for i := it.Next(); i < RootSymbols; i = it.Next() {
		sm.idx = append(sm.idx, i)
	}
}

// addTo adds the expanded element to the symbols syms, which hold symbols
// [0, len(syms)) of a stream. len(syms) is at most RootSymbols.
func (sm *sums) addTo(syms []CodedSymbol) {
	for _, i := range sm.idx {
		if i >= uint64(len(syms)) {
			return
		}
		syms[i].Add(&sm.sym)
	}
}

// level1 returns the level-1 range of root that holds addr.
func level1(root Range, addr swarm.Address) int {
	return int(root.ChildIndex(addr, FanoutBits))
}

// Symbols returns the coded symbols [start, end) of the stream of the
// elements in r, from the best source (proposal §4.3.5):
//
//   - r is the root of the live slab and end ≤ RootSymbols: the root cache;
//   - r is one of its level-1 ranges and end ≤ Level1Symbols: the level-1
//     cache;
//   - r is at most LeafBits bits below the root and end ≤ BucketSymbols: the
//     bucket Fenwick tree;
//   - r holds at most ScanLimit elements and scan is not nil: the scan path,
//     which encodes every element scan yields in r through the Index's Mapper
//     (for a range below leaf depth the count of its bucket is the bound);
//   - otherwise the symbols of the largest cache of r below end, from start
//     (none if start is past it), with exhausted true.
//
// exhausted is true when no source can go past the returned symbols: the
// returned window ends at the largest cache of r and the scan path is not
// available. The root and level-1 caches serve only the index's own root:
// a narrower session root uses the bucket tree and the scan path.
//
// The returned symbols are fresh copies owned by the caller, never aliased
// to the caches, so later commits do not change them; a stream that must
// stay one fixed set across batches (see Decoder) is the session's to
// snapshot. The caller bounds end − start (the session symbol budget).
//
// The serving source can change between calls for the same r: a window
// inside a cache is read from it, a window past it is scanned, and a commit
// can move count(r) across ScanLimit. Cache and scan results may reflect
// different element sets (the scan reads the address index at its own time,
// not the caches' state). So a session must snapshot the whole stream it
// serves before its first batch, never combine batches of separate calls
// (T2.4).
//
// scan, called without any index lock held, must call fn for every element
// of r (elements outside r are ignored) and return the first error fn
// returns; its error is returned wrapped. It is the caller's iteration of the
// address index, so fingerprint does not depend on the storer, and it is not
// synchronized with the caches. Symbols returns ErrInvalidBatch if end is
// below start, and ErrBusy or ErrOutsideRoot as Summary does.
func (ix *Index) Symbols(r Range, start, end uint64, scan func(r Range, fn func(ID) error) error) (syms []CodedSymbol, exhausted bool, err error) {
	if end < start {
		return nil, false, fmt.Errorf("%w: symbols [%d, %d)", ErrInvalidBatch, start, end)
	}
	r = NewRange(r.Prefix, r.Bits)

	ix.mu.RLock()
	live, next := ix.live, ix.next
	if live == nil || !live.root.ContainsRange(r) {
		ix.mu.RUnlock()
		if live == nil || (next != nil && next.root.ContainsRange(r)) {
			return nil, false, ErrBusy
		}
		return nil, false, ErrOutsideRoot
	}
	root := live.root
	var limit uint64 // the largest cache of r
	switch {
	case r.Equal(root):
		limit = RootSymbols
	case r.Bits == root.Bits+FanoutBits:
		limit = Level1Symbols
	case r.Bits-root.Bits <= ix.leafBits:
		limit = BucketSymbols
	}
	lo, n, aligned := root.BucketSpan(r, ix.leafBits)
	if !aligned {
		// Below leaf depth the index has no count of r itself; the bound is
		// the count of the whole containing bucket. This is conservative: a
		// sparse sub-range of a bucket holding more than ScanLimit elements
		// is exhausted although a scan of it would be small.
		lo, n = root.ChildIndex(r.Prefix, ix.leafBits), 1
	}
	from, to := min(start, limit), min(end, limit)
	var count countNode
	ix.readLive(func() {
		live.counts.Range(int(lo), int(lo+n), &count)
		if end > limit && scan != nil && count <= ScanLimit {
			return // the scan path, below
		}
		syms = make([]CodedSymbol, to-from)
		switch limit {
		case RootSymbols:
			copy(syms, live.rootSyms[from:to])
		case Level1Symbols:
			copy(syms, live.level1Syms[level1(root, r.Prefix)][from:to])
		case BucketSymbols:
			if to > from {
				var b bucketSymbols
				live.bucketSyms.Range(int(lo), int(lo+n), &b)
				copy(syms, b[from:to])
			}
		}
	})
	ix.mu.RUnlock()

	if syms != nil {
		return syms, to == limit && (scan == nil || count > ScanLimit), nil
	}
	syms = make([]CodedSymbol, end-start)
	err = scan(r, func(id ID) error {
		if r.ContainsID(id) {
			Encode(ix.mapper, syms, start, id, 1)
		}
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("fingerprint: scan %s: %w", r, err)
	}
	return syms, false, nil
}
