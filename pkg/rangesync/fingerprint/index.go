// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Errors returned by the Index.
var (
	// ErrBusy is returned by Summary and RootSummary for a range that no
	// slab covers yet: before the first Rebuild completes and, while a
	// Rebuild to a new root runs, for ranges of the new root outside the
	// current one. Rebuild returns it when another Rebuild is running.
	ErrBusy = errors.New("fingerprint: index busy")
	// ErrBelowLeaf is returned by Summary for a range inside the root but
	// more than LeafBits bits below it. The index keeps no summaries below
	// its buckets; the caller scans the address index instead.
	ErrBelowLeaf = errors.New("fingerprint: range below leaf depth")
	// ErrOutsideRoot is returned by Summary for a range that does not lie
	// inside the root of the index, for example a range of a session scoped
	// on a root the index has since moved away from.
	ErrOutsideRoot = errors.New("fingerprint: range outside root")
)

var errNotImplemented = errors.New("fingerprint: not implemented")

// maxIndexLeafBits bounds IndexOptions.LeafBits: 1<<16 buckets already hold
// a 128 MB LtHash slab.
const maxIndexLeafBits = 16

// IndexOptions configures an Index.
type IndexOptions struct {
	// LeafBits is the depth of the buckets below the root: the index keeps
	// 1<<LeafBits buckets and answers Summary for ranges down to that depth.
	// Zero means LeafBits, the protocol value; other values, up to 16, are
	// for tests.
	LeafBits uint8
	// RootChangeDebounce is the debounce interval of SubscribeRootChange.
	// Zero signals every change at once (tests); production passes the status
	// gossip debounce.
	RootChangeDebounce time.Duration
}

// Index keeps the summaries (count and LtHash) of the elements under a root
// range, per bucket, in Fenwick trees, so that the summary of any range at or
// above leaf depth takes O(LeafBits) group operations (proposal §5.2). The
// buckets are the 1<<LeafBits sub-ranges LeafBits bits below the root (bits
// root.Bits … root.Bits+LeafBits−1 of the address, Range.BucketSpan and
// Range.ChildIndex). Phase 2 adds the coded-symbol caches to the same slabs.
//
// The index is kept current from the reserve's committed deltas (Commit,
// Apply) and rebuilt from one snapshot of the address index when the root
// moves (Rebuild). Elements outside the root are ignored by Commit, Apply and
// Rebuild and counted in a metric.
//
// # States
//
// Before the first Rebuild opens its snapshot the index has no slab: deltas
// are dropped (the first snapshot covers them) and Summary returns ErrBusy.
// Rebuild switches from dropping to collecting deltas for the new slab in the
// same write-locked section that opens its snapshot, so every commit is
// either in the snapshot or in the delta stream. Once a Rebuild completes,
// its slab answers Summary, and from then on the index always has a live
// slab. A failed first Rebuild returns the index to dropping deltas.
//
// There is no Reset: the reserve's Reset wraps every per-item delete in the
// observer's Commit with an exact delta, so the index is exact once it
// returns, without any action of its own.
//
// # Locking
//
// mu is taken before slabMu or nextMu, and slabMu and nextMu are never held
// together:
//
//   - mu, a sync.RWMutex, orders commits against a rebuild's snapshot.
//     Commit and Apply hold it in read mode across the transaction and the
//     application of its delta; Summary holds it in read mode to pick the
//     slab; Rebuild holds it in write mode only to open its snapshot (rebuild
//     step 1) and to swap the slabs (step 5). rangesync never calls the
//     storer while holding mu, except Commit, which holds mu in read mode
//     across the reserve's own run, and the opening of the rebuild's
//     snapshot (the start of the iterate function passed to Rebuild), which
//     must not wait for anything a commit may hold.
//   - slabMu, a sync.RWMutex, guards the contents of the live slab: Fenwick
//     mutations need exclusive access, since they write the upper nodes that
//     every query reads, while commits from different reserve batches apply
//     their deltas concurrently under mu's read lock. Commit and Apply take it
//     in write mode, Summary in read mode. It is taken after mu.
//   - nextMu, a sync.Mutex, guards the contents of the slab a running Rebuild
//     builds and its converted flag (step 5): the rebuild's own additions and
//     the conversion (taken alone, without mu) and the deltas of concurrent
//     commits (taken after mu). It is never held together with slabMu.
//
// The SHAKE expansion of an element (~5 µs) is done under mu's read lock,
// since the delta of a commit is known only after its transaction ran and
// must be applied before the read lock is released, but outside slabMu and
// nextMu, so only the 2 KB additions are serialized. Each element of a delta
// is expanded once; during a Rebuild the same LtHash is added into both the
// live and the new slab. The subscriptions of
// SubscribeRootChange have their own mutex, taken last and never held while
// taking another.
//
// # Memory
//
// A slab holds one Fenwick tree per summary component, through unexported
// Group adapters: counts as uint32 modulo 2^32 (4 B per bucket, 16 KB at
// B = 4096) and LtHash through a defined type whose Add and Sub are AddHash
// and SubHash (2 KB per bucket, 8 MB at B = 4096), proposal §5.3. While a
// Rebuild runs, the slab being built doubles that.
//
// # Rebuild
//
// Rebuild follows proposal §5.2, as confirmed by the T1.0 spike:
//
//  1. Allocate the empty new slab without any lock, then take mu in write
//     mode, install the new slab (from then on every commit's delta is
//     applied to it as well, step 3), and call iterate, which opens one
//     snapshot of the address index. mu is released in the first call of the
//     element callback, or on any other exit from iterate: its return (an
//     empty root, or an error before the first element) or a panic.
//  2. Build the new slab from the snapshot, as plain per-bucket sums, without
//     mu.
//  3. Commits that land after the snapshot was opened apply their deltas to
//     both the live and the new slab.
//  4. Commits hold mu in read mode across the transaction and the
//     application of the delta, so each is either in the snapshot or in the
//     delta stream, never both and never neither.
//  5. Under nextMu, convert the new slab's bucket sums to Fenwick trees
//     with NewFenwickFromBuckets (O(B) group operations) and set its
//     converted flag in the same critical section. A commit's delta to the
//     new slab is a plain bucket addition before the flag and a Fenwick
//     update (Fenwick.Add, Sub or Update) after it, so a commit between the
//     conversion and the swap is applied correctly; a plain bucket addition
//     into a converted tree would corrupt it for good. Then take mu in write
//     mode only to swap the slabs (O(1)); commits after the swap apply to the
//     new live slab alone.
//
// While a Rebuild runs, the live slab keeps answering for its own root, and
// Summary returns ErrBusy for ranges of the new root it does not cover.
//
// All methods are safe for concurrent use.
type Index struct {
	leafBits uint8
	debounce time.Duration
	metrics  metrics
}

// NewIndex returns an empty index with no root: until the first Rebuild opens
// its snapshot, deltas are dropped and Summary returns ErrBusy. It panics if
// o.LeafBits is over 16.
func NewIndex(o IndexOptions) *Index {
	leafBits := o.LeafBits
	if leafBits == 0 {
		leafBits = LeafBits
	}
	if leafBits > maxIndexLeafBits {
		panic(fmt.Sprintf("fingerprint: index leaf bits %d over %d", leafBits, maxIndexLeafBits))
	}
	return &Index{
		leafBits: leafBits,
		debounce: o.RootChangeDebounce,
		metrics:  newMetrics(),
	}
}

// LeafBits returns the depth of the buckets below the root, sent as
// Status.LeafBits.
func (ix *Index) LeafBits() uint8 {
	return ix.leafBits
}

// Commit runs a reserve transaction and applies the delta it committed,
// holding mu in read mode across both: it is the index half of
// storer.ReserveObserver.Commit (whose four-point contract it follows), with
// the delta already converted to IDs. run is called exactly once,
// synchronously, and is not retained. If run returns an error, nothing is
// applied and the error is returned as is (callers test it with errors.Is).
// Commit must not be called from inside run of another Commit: a recursive
// read lock deadlocks as soon as a Rebuild waits for the write lock.
//
// The delta is a group difference: adds and removes are applied in any
// order, and their slices are only read. Elements outside every root are
// ignored and counted in a metric. Before the first Rebuild opened its
// snapshot, the delta is dropped.
// A change inside the live root is signaled to SubscribeRootChange.
func (ix *Index) Commit(run func() (adds, removes []ID, err error)) error {
	_, _, err := run()
	if err != nil {
		return err
	}
	return errNotImplemented
}

// Apply applies a delta: it is Commit with a run that returns adds and
// removes. It is for tests and has no production caller: it is exact only
// when the changes it reports are in no snapshot that a running Rebuild
// opened, so changes made by reserve transactions go through Commit.
func (ix *Index) Apply(adds, removes []ID) {
	_ = ix.Commit(func() ([]ID, []ID, error) { return adds, removes, nil })
}

// Summary returns the number of elements in r and the digest of their
// LtHash, with the zero Digest when the count is 0. r is canonicalized
// first, so bits of its prefix past r.Bits are ignored.
//
// It returns ErrBelowLeaf if r lies inside the root more than LeafBits bits
// below it, ErrBusy if no slab covers r yet (see the type documentation),
// and ErrOutsideRoot if r does not lie inside the root of the live slab or
// of a running Rebuild. It never calls into the storer. A range at or above
// leaf depth reads O(LeafBits) Fenwick nodes per tree, under mu and slabMu
// in read mode; the keccak256 digest is computed after both are
// released.
func (ix *Index) Summary(r Range) (Summary, error) {
	return Summary{}, errNotImplemented
}

// RootSummary returns the root of the live slab and its Summary, read
// together under one lock, for the status message. It returns ErrBusy when
// there is no live slab.
func (ix *Index) RootSummary() (Range, Summary, error) {
	return Range{}, Summary{}, errNotImplemented
}

// Rebuild replaces the index with one of root, built from one snapshot of
// the address index, following the protocol in the type documentation.
// root is canonicalized first; root.Bits+LeafBits must be at most MaxBits.
//
// iterate must open one consistent snapshot of the elements of root when it
// is called, then call fn for each element of the snapshot, sequentially, in
// any order, and return the first error fn returns. Rebuild holds mu in
// write mode from before iterate is called until fn's first call, or until
// iterate exits in any other way (it returns, also with an error before the
// first element, or panics). So iterate must open its snapshot before any
// blocking operation (no lock, channel or wait of any kind: a reserve
// transaction may hold the reserve's batch and bin locks and its
// registration lock while waiting for mu), must reach the first element (or
// its return, for an empty root) after O(1) work, by seeking straight to the
// start of root, and must not call into the index. iterate is
// one call over one snapshot: paging with a cursor, which opens a new
// snapshot per page, would count commits made between pages twice or not at
// all. Elements outside root are ignored and counted in a metric.
//
// If iterate returns an error, the new slab is discarded, the live slab (if
// any) stays, and the error is returned wrapped; with no live slab the index
// returns to dropping deltas. Rebuild returns ErrBusy if another Rebuild is
// running. On success the new slab replaces the live one and the change is
// signaled to SubscribeRootChange. Cancellation is up to iterate: fn's
// caller stops when fn returns an error, for example on shutdown.
func (ix *Index) Rebuild(root Range, iterate func(fn func(ID) error) error) error {
	return errNotImplemented
}

// SubscribeRootChange returns a channel that receives a value after the root
// summary may have changed: a Commit or Apply that changed an element inside
// the live root, or the swap at the end of a Rebuild. Signals
// coalesce (the channel has a buffer of one and is never blocked on) and are
// debounced by IndexOptions.RootChangeDebounce: the first change after a
// signal is delivered that long after it happened, together with every
// change in between; with zero it is delivered at once. unsubscribe closes
// the channel and is safe to call more than once.
func (ix *Index) SubscribeRootChange() (c <-chan struct{}, unsubscribe func()) {
	ch := make(chan struct{}, 1)
	var once sync.Once
	return ch, func() { once.Do(func() { close(ch) }) }
}

// Close stops a pending debounce timer; changes after Close are not
// signaled. Subscriptions stay open until unsubscribed. It does not stop a
// running Rebuild (its iterate function does). It returns nil.
func (ix *Index) Close() error {
	return nil
}

// Metrics returns the index's prometheus collectors.
func (ix *Index) Metrics() []prometheus.Collector {
	return ix.metrics.collectors()
}
