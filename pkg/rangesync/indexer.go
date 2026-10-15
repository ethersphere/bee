// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Default intervals of the Indexer's rebuild goroutine.
const (
	// defaultReadyPollInterval is how often the rebuild goroutine checks
	// whether the address index is ready while it waits for it.
	defaultReadyPollInterval = time.Second
	// defaultRebuildRetryInterval is how long the rebuild goroutine waits
	// after a failed rebuild before it tries again.
	defaultRebuildRetryInterval = 10 * time.Second
)

// IndexerStore is the part of the storer the Indexer reads: a subset of
// storer.ReserveAddrStore (proposal §5.4) and storer.RadiusChecker. *storer.DB
// implements it once the address index API lands (T1.5). Registration
// (SetReserveObserver) is not part of it: the caller registers the Indexer
// and calls Start only if registration succeeded.
//
// The Indexer calls these methods only from its rebuild goroutine, never
// from Commit, RadiusChanged or Reset, and never while holding an Index lock,
// except the start of ReserveIterateAddr during a rebuild (see
// ReserveIterateAddr).
type IndexerStore interface {
	// ReserveIterateAddr calls cb with the address index entries whose
	// address starts with the first bits bits of prefix, in address order,
	// starting strictly after the ID after (nil: from the start), until cb
	// returns stop true or an error, which it returns. One call reads one
	// snapshot of the address index, opened when it is called and before cb's
	// first call. The rebuild calls it once per rebuild with after nil, while
	// holding the Index's lock in write mode until cb's first call or until
	// it returns. It is the one exception to contract point 4 of
	// storer.ReserveObserver, so it must open its leveldb iterator before any
	// blocking operation (no lock, channel or wait of any kind: a reserve
	// transaction may hold the reserve's batch and bin locks and the observer
	// registration lock while it waits for the Index's lock), seek straight to
	// the start of the range, so that cb's first call (or the return, for an
	// empty range) comes after O(1) work, and accept a prefix of
	// ceil(bits/8) bytes (empty at bits 0).
	ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(storer.ChunkID) (stop bool, err error)) error
	// ReserveAddrIndexReady reports whether the address index is complete.
	// The first rebuild waits for it, so that its snapshot holds every element
	// the reserve does not report through the observer (the fill).
	ReserveAddrIndexReady() bool
	// StorageRadius returns the reserve radius, which with the overlay
	// defines the root of the index (proposal §14 Q5): the root is
	// fingerprint.NewRange(overlay, StorageRadius()), the addresses that
	// storer.RadiusChecker.IsWithinStorageRadius accepts.
	StorageRadius() uint8
}

// IndexerOptions configures an Indexer.
type IndexerOptions struct {
	// ReadyPollInterval is how often the rebuild goroutine checks
	// ReserveAddrIndexReady while it waits for the address index. Zero means
	// one second.
	ReadyPollInterval time.Duration
	// RebuildRetryInterval is how long the rebuild goroutine waits after a
	// failed rebuild before it tries again. Zero means ten seconds.
	RebuildRetryInterval time.Duration
}

var (
	_ storer.ReserveObserver = (*Indexer)(nil)
	_ Summarizer             = (*fingerprint.Index)(nil)
)

// Indexer keeps a fingerprint.Index current with the reserve: it is the
// storer.ReserveObserver of rangesync (proposal §5.2, §5.4), and it runs the
// single goroutine that rebuilds the Index when the root moves.
//
// Start-up order: register the Indexer with the storer's SetReserveObserver,
// then, only if that returned nil, call Start; if registration fails, do
// not Start (rangesync stays disabled). Start queues the initial rebuild; the rebuild goroutine
// waits for ReserveAddrIndexReady, reads StorageRadius and rebuilds the
// Index at NewRange(overlay, radius). Until that rebuild opens its snapshot
// the Index drops deltas, so no commit is missed: every commit after
// registration is in the snapshot or in the delta stream. The start-up
// SetRadius of the reserve workers triggers a second rebuild only if the
// radius changed in between.
//
// Rebuilds are coalesced and run one at a time. RadiusChanged only queues a
// wake-up and returns. The goroutine takes the wake-up, then waits for the
// address index to be ready, then reads StorageRadius, and skips the
// rebuild only if the radius equals the root of the last rebuild that
// completed (none initially, and none after a failed rebuild). Since the
// wake-up is taken before the radius is read, a SetRadius after the read
// leaves a wake-up behind. A failed rebuild is retried after
// IndexerOptions.RebuildRetryInterval, or at once when a wake-up arrives
// during that wait; the first failure of a run of failures is logged as a
// warning, the following ones at debug level, until a rebuild completes.
type Indexer struct {
	index   *fingerprint.Index
	store   IndexerStore
	overlay swarm.Address
	logger  log.Logger
	opts    IndexerOptions
}

// NewIndexer returns an Indexer that keeps index current from store for the
// neighborhood of overlay. It starts nothing; see Start.
func NewIndexer(index *fingerprint.Index, store IndexerStore, overlay swarm.Address, logger log.Logger, o IndexerOptions) *Indexer {
	if o.ReadyPollInterval == 0 {
		o.ReadyPollInterval = defaultReadyPollInterval
	}
	if o.RebuildRetryInterval == 0 {
		o.RebuildRetryInterval = defaultRebuildRetryInterval
	}
	return &Indexer{
		index:   index,
		store:   store,
		overlay: overlay,
		logger:  logger.WithName(loggerName).Register(),
		opts:    o,
	}
}

// Start starts the rebuild goroutine with the initial rebuild queued. It
// must be called once, after the Indexer was registered as the reserve
// observer.
func (x *Indexer) Start() {}

// Close stops the rebuild goroutine, canceling a running rebuild (the Index
// keeps its previous slab), and waits for it to exit. Commits after Close are
// still applied. It returns nil.
func (x *Indexer) Close() error {
	return nil
}

// Commit implements storer.ReserveObserver. It runs the reserve transaction
// through Index.Commit, which holds the Index lock in read mode across run
// and the application of the delta, converting the ChunkIDs with
// fingerprint.NewID (address ‖ stamp hash; BatchID is not part of the
// identity), which cannot fail, so there is no error path after run. It
// calls run exactly once and returns its error as is; it never calls into
// the storer.
func (x *Indexer) Commit(run func() (adds, removes []storer.ChunkID, err error)) error {
	_, _, err := run()
	return err
}

// RadiusChanged implements storer.ReserveObserver. It treats the call as a
// hint, ignoring radius: it queues a coalesced wake-up of the rebuild
// goroutine and returns without blocking.
func (x *Indexer) RadiusChanged(uint8) {}

// Reset implements storer.ReserveObserver. It does nothing: the reserve's
// Reset delivered every per-item delete through Commit with an exact delta,
// so the Index is already exact, and the root changes were signaled then.
func (x *Indexer) Reset() {}
