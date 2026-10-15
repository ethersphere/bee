// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/stabilization"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/events"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"resenje.org/multex"
)

// PutterSession provides a session around the storage.Putter. The session on
// successful completion commits all the operations or in case of error, rolls back
// the state.
type PutterSession interface {
	storage.Putter
	// Done is used to close the session and optionally assign a swarm.Address to
	// this session.
	Done(swarm.Address) error
	// Cleanup is used to cleanup any state related to this session in case of
	// any error.
	Cleanup() error
}

// SessionInfo is a type which exports the storer tag object. This object
// stores all the relevant information about a particular session.
type SessionInfo = upload.TagItem

// UploadStore is a logical component of the storer which deals with the upload
// of data to swarm.
type UploadStore interface {
	// Upload provides a PutterSession which is tied to the tagID. Optionally if
	// users requests to pin the data, a new pinning collection is created.
	Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error)
	// NewSession can be used to obtain a tag ID to use for a new Upload session.
	NewSession() (SessionInfo, error)
	// Session will show the information about the session.
	Session(tagID uint64) (SessionInfo, error)
	// DeleteSession will delete the session info associated with the tag id.
	DeleteSession(tagID uint64) error
	// ListSessions will list all the Sessions currently being tracked.
	ListSessions(offset, limit int) ([]SessionInfo, error)
}

// PinStore is a logical component of the storer which deals with pinning
// functionality.
type PinStore interface {
	// NewCollection can be used to create a new PutterSession which writes a new
	// pinning collection. The address passed in during the Done of the session is
	// used as the root referencce.
	NewCollection(context.Context) (PutterSession, error)
	// DeletePin deletes all the chunks associated with the collection pointed to
	// by the swarm.Address passed in.
	DeletePin(context.Context, swarm.Address) error
	// Pins returns all the root references of pinning collections.
	Pins() ([]swarm.Address, error)
	// HasPin is a helper which checks if a collection exists with the root
	// reference passed in.
	HasPin(swarm.Address) (bool, error)
}

// PinIterator is a helper interface which can be used to iterate over all the
// chunks in a pinning collection.
type PinIterator interface {
	IteratePinCollection(root swarm.Address, iterateFn func(swarm.Address) (bool, error)) error
}

// CacheStore is a logical component of the storer that deals with cache
// content.
type CacheStore interface {
	// Lookup method provides a storage.Getter wrapped around the underlying
	// ChunkStore which will update cache related indexes if required on successful
	// lookups.
	Lookup() storage.Getter
	// Cache method provides a storage.Putter which will add the chunks to cache.
	// This will add the chunk to underlying store as well as new indexes which
	// will keep track of the chunk in the cache.
	Cache() storage.Putter
}

// NetStore is a logical component of the storer that deals with network. It will
// push/retrieve chunks from the network.
type NetStore interface {
	// DirectUpload provides a session which can be used to push chunks directly
	// to the network.
	DirectUpload() PutterSession
	// Download provides a getter which can be used to download data. If the data
	// is found locally, its returned immediately, otherwise it is retrieved from
	// the network.
	Download(cache bool) storage.Getter
	// PusherFeed is the feed for direct push chunks. This can be used by the
	// pusher component to push out the chunks.
	PusherFeed() <-chan *pusher.Op
}

var _ Reserve = (*DB)(nil)

// Reserve is a logical component of the storer that deals with reserve
// content. It will implement all the core functionality required for the protocols.
type Reserve interface {
	ReserveStore
	EvictBatch(ctx context.Context, batchID []byte) error
	ReserveSample(context.Context, []byte, uint8, uint64, *big.Int) (Sample, error)
	ReserveSize() int
}

// ReserveIterator is a helper interface which can be used to iterate over all
// the chunks in the reserve.
type ReserveIterator interface {
	ReserveIterateChunks(cb func(swarm.Chunk) (bool, error)) error
}

// ReserveStore is a logical component of the storer that deals with reserve
// content. It will implement all the core functionality required for the protocols.
type ReserveStore interface {
	ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte, stampHash []byte) (swarm.Chunk, error)
	ReserveHas(addr swarm.Address, batchID []byte, stampHash []byte) (bool, error)
	ReservePutter() storage.Putter
	SubscribeBin(ctx context.Context, bin uint8, start uint64) (<-chan *BinC, func(), <-chan error)
	ReserveLastBinIDs() ([]uint64, uint64, error)
	RadiusChecker
}

// RadiusChecker provides the radius related functionality.
type RadiusChecker interface {
	IsWithinStorageRadius(addr swarm.Address) bool
	StorageRadius() uint8
	CommittedDepth() uint8
	CapacityDoubling() uint8
}

// ChunkID identifies one (chunk, stamp) pair in the reserve: one element of
// the set kept by the reserve address index. Its identity is
// (Address, StampHash) only, so consumers must not key on BatchID
// (rangesync fingerprint IDs are address ‖ stamp hash). BatchID is always
// set and is carried along so the chunk can be resolved without a lookup.
// The byte slices may alias memory owned by the reserve or by the caller of
// Put and must be treated as read-only. ChunkID is defined in the reserve
// package so the reserve can build deltas without importing the storer.
type ChunkID = reserve.ChunkID

// The reserve calls its observer through reserve.Observer; the two
// interfaces must stay interchangeable.
var (
	_ reserve.Observer = ReserveObserver(nil)
	_ ReserveObserver  = reserve.Observer(nil)
)

// ReserveObserver receives every committed change of the reserve's
// (address, stamp hash) set made by the reserve's Put, eviction and Reset
// paths while the reserve address index is enabled.
//
// Registration (ReserveAddrStore.SetReserveObserver): one observer is
// registered, once; a second registration, a nil observer and a
// registration while the address index is disabled are rejected with an
// error. Registration returns only after every set-changing
// transaction that started without the observer has committed, and every
// transaction started after it is wrapped by Commit, so a rebuild started
// after registration sees every later commit either in its snapshot or in
// the delta stream. The reserve guarantees this with a reserve-level
// registration lock: each set-changing transaction loads the observer and
// runs, wrapped or not, under the read lock, and registration takes the
// write lock. The read lock is taken per transaction, in the goroutine
// that runs it, and released when that transaction (and its Commit) has
// returned; it is never held across waiting for other transactions (such
// as the per-item goroutines of eviction and Reset) and never taken twice
// by one goroutine, since a holder waiting for a goroutine that is blocked
// behind a pending registration deadlocks. Lock order: reserve per-batch
// and per-bin locks, registration lock (read), the observer's lock (read,
// inside Commit), per-address chunk locks, leveldb.
//
// Not reported: changes made before registration, and changes not made by
// the Put, eviction and Reset paths, such as start-up recovery, migrations
// and the address index fill (including a background fill running
// concurrently with Commit). The observer's first rebuild, from a snapshot
// opened once the address index is ready, covers them.
//
// All methods may be called concurrently from several goroutines.
type ReserveObserver interface {
	// Commit wraps every reserve transaction that may change the set. run
	// executes the transaction and returns the delta it committed; a
	// successful transaction may return an empty delta.
	//
	// The delta is exact: an id is in removes only if its address index
	// entry existed and was deleted in this transaction, and in adds only
	// if an entry was written where none existed. A wrong remove would
	// subtract an element the observer never added and leave a phantom
	// difference forever. The delta is a group difference, so adds and
	// removes may be applied in either order. The slices are not reused by
	// the reserve and may be retained (the ChunkID contents are read-only,
	// see ChunkID).
	//
	// A transaction failed when its leveldb batch was not written; then run
	// returns an error and an empty delta. A transaction whose batch was
	// written succeeded, even if it reports an error afterwards (releasing
	// sharky locations after the batch commit): run returns its full delta
	// and a nil error, and the reserve returns that error to its own caller
	// after Commit returns.
	//
	// Holding a read lock across run and the application of its delta is
	// what lets a rebuild that opens an index snapshot under the write lock
	// see every commit either in the snapshot or in the delta stream, never
	// both and never neither.
	//
	// Until its first rebuild opens a snapshot the observer may drop deltas,
	// deciding from its own state, read under the read lock it holds across
	// run, never by asking the storer (point 3). It switches from dropping
	// to queuing for the new index in the same write-locked section that
	// opens the first snapshot iterator, so a commit is either before that
	// section (in the snapshot) or after it (queued). Keeping on dropping
	// until the first index is swapped in would lose every commit made
	// during the first iteration.
	//
	// Contract:
	//  1. Commit calls run exactly once, synchronously, and returns its
	//     error as is; when run returns an error, Commit applies nothing.
	//     run is not retained or called after Commit returns.
	//  2. The reserve's Put, eviction and Reset paths call Commit once per
	//     transaction that may change the set and never from inside run
	//     (no nesting: a recursive read lock deadlocks as soon as a rebuild
	//     is waiting for the write lock). Operations that commit several
	//     transactions, such as batch eviction and Reset, wrap each one
	//     separately, never the whole loop.
	//  3. The reserve may hold its per-batch and per-bin locks and the
	//     registration read lock when it calls Commit, so Commit must not
	//     call into the storer.
	//  4. The observer never holds its own lock, in either mode, across a
	//     call into the storer: a reader waiting for a reserve lock, held by
	//     a writer that waits for the read lock behind a pending rebuild,
	//     deadlocks. The observer's lock covers in-memory work only.
	Commit(run func() (adds, removes []ChunkID, err error)) error

	// RadiusChanged is a hint that the reserve radius may have changed. It
	// is called at the end of every reserve SetRadius, after the new radius
	// is in effect (also when persisting it failed), with the value that
	// call set. SetRadius loads the observer after storing the radius,
	// through a synchronized load, so a SetRadius racing with registration
	// either notifies the observer or stored its radius before the
	// observer's first rebuild reads it. Concurrent SetRadius calls are not
	// serialized, so calls may arrive out of order and the argument may
	// already be stale: the observer must not trust it, and must not drop a
	// call because the argument equals its current root (root 5, rebuild to
	// 6 queued, a later 5 dropped leaves the index at 6 and the reserve at
	// 5). It only queues an asynchronous rebuild (a coalescing wake-up) and
	// returns. The rebuild goroutine takes the wake-up before it reads
	// StorageRadius, without holding the observer's lock, and skips only if
	// the value equals the root of the last rebuild that completed; a
	// failed rebuild leaves no such root, and initially there is none (0 is
	// a valid radius). It is called from the reserve workers, so it must
	// not block and must not call into the storer.
	RadiusChanged(radius uint8)

	// Reset is called once at the end of a successful reserve Reset, after
	// every per-item transaction went through Commit; the reserve loads the
	// observer for it through a synchronized load. It must not call into
	// the storer.
	Reset()
}

var _ ReserveAddrStore = (*DB)(nil)

var (
	// ErrAddrIndexDisabled is returned by the ReserveAddrStore methods that
	// return an error when the reserve address index is disabled
	// (Options.ReserveAddrIndex false, or no reserve).
	ErrAddrIndexDisabled = reserve.ErrAddrIndexDisabled
	// ErrNilObserver is returned by SetReserveObserver for a nil observer.
	ErrNilObserver = reserve.ErrNilObserver
	// ErrObserverRegistered is returned by SetReserveObserver when an
	// observer is already registered: there is one observer, registered
	// once.
	ErrObserverRegistered = reserve.ErrObserverRegistered
)

// ReserveAddrStore gives access to the reserve address index: the set of
// (chunk address, stamp hash) pairs of the reserve, one element per ChunkID,
// ordered by address ‖ stamp hash. rangesync reconciles this set with its
// peers and keeps its in-memory fingerprint index over it current through a
// ReserveObserver.
//
// *DB implements it. With the address index disabled (Options.ReserveAddrIndex
// false, or no reserve) every method that returns an error returns
// ErrAddrIndexDisabled and ReserveAddrIndexReady reports false.
//
// The index is complete once ReserveAddrIndexReady reports true. Before that
// (a fill in progress) the reading methods work on the entries written so
// far: a subset of the reserve set, which may miss elements the reserve holds
// but never holds an element the reserve does not.
//
// ReserveIterateAddr and ReserveHasID take no reserve lock; ReserveGetByID
// takes the reserve's per-batch lock, as ReserveGet does. None of the methods
// may be called from inside a ReserveObserver method (contract point 3), and
// an observer must not hold its own lock across any of them (point 4), with
// the one exception documented on ReserveIterateAddr.
type ReserveAddrStore interface {
	// ReserveIterateAddr calls cb with the elements whose address starts
	// with the first bits bits of prefix, in ascending order of
	// address ‖ stamp hash, starting strictly after after (nil: at the
	// start of the range), until cb returns stop true or an error.
	// ReserveIterateAddr returns cb's error wrapped (errors.Is matches it).
	//
	// Bits of prefix past bits are ignored and missing prefix bytes count
	// as zero, as for a fingerprint range, so a range's prefix of
	// ceil(bits/8) bytes (empty when bits is 0) and its bit length can be
	// passed as they are. after is compared bytewise with
	// address ‖ stamp hash (an element ID, 64 bytes, is the usual cursor);
	// it may lie outside the range: before the range the iteration starts
	// at the range start, past it there is nothing to iterate.
	//
	// One call is one Iterate over the index store and reads one leveldb
	// snapshot: a single iterator is opened when it is called, before cb's
	// first call, and commits made after that are not seen. Resuming with
	// after opens a new snapshot, so a scan paged over several calls is not
	// one consistent view. cb runs while the iterator is open, which pins
	// the snapshot's leveldb files, and should not block for long.
	//
	// The one exception to ReserveObserver contract point 4: an observer
	// may call ReserveIterateAddr while holding its own lock in write mode,
	// until cb's first call or, for an empty range, until it returns (the
	// rebuild opens its snapshot this way, while reserve writers holding
	// per-batch locks wait for that lock in read mode inside Commit). To
	// keep that deadlock-free, ReserveIterateAddr must, and every change to
	// it must keep to this:
	//  1. open its leveldb iterator before any blocking operation: no
	//     reserve lock (per-batch, per-bin, the observer registration
	//     lock), channel or wait of any kind before cb's first call;
	//  2. seek straight to the masked start of the range (or to after, when
	//     after lies inside it), so cb's first call, or the return for an
	//     empty range, comes after O(1) work, never after scanning or
	//     filtering keys outside the range;
	//  3. be a single Iterate call, so the whole call is one snapshot.
	//
	// Every ChunkID passed to cb has Address, StampHash and BatchID set; its
	// slices are not reused, so cb may retain it (read-only, see ChunkID).
	ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(ChunkID) (stop bool, err error)) error

	// ReserveGetByID returns the chunk of the element (addr, stampHash),
	// with its stamp, as ReserveGet does: it resolves the batch ID from the
	// address index and then reads the chunk under the reserve's per-batch
	// lock. It returns an error wrapping storage.ErrNotFound when the
	// element is not in the index, or was removed between the two steps.
	ReserveGetByID(ctx context.Context, addr swarm.Address, stampHash []byte) (swarm.Chunk, error)

	// ReserveHasID reports whether the element (addr, stampHash) is in the
	// address index. Peers exchange element IDs without the batch ID, so
	// ReserveHas, which needs it, cannot be used. Until
	// ReserveAddrIndexReady reports true it may report false for an element
	// the reserve holds.
	ReserveHasID(addr swarm.Address, stampHash []byte) (bool, error)

	// ReserveAddrIndexReady reports whether the address index is complete:
	// a valid marker (see Options.ReserveAddrIndex) was found at start-up or
	// has been written since. It is false while the index is disabled and
	// while a fill runs; once true it stays true for the lifetime of the DB.
	//
	// It reads an in-memory flag (no store access), so callers may poll it;
	// rangesync answers Busy until it reports true. The flag is set by an
	// atomic store only after every fill transaction has committed and the
	// marker write has returned, so once it reports true every element of
	// the reserve is visible to any iterator opened afterwards. A background
	// fill holds an element's per-batch lock across reading it, re-checking
	// that the reserve still holds it, writing its index entry and
	// committing, so an element removed during the fill is never written.
	ReserveAddrIndexReady() bool

	// SetReserveObserver registers the single observer of the address
	// index set; see ReserveObserver for what registration guarantees. It
	// may be called before ReserveAddrIndexReady reports true and does not
	// wait for it. It returns ErrAddrIndexDisabled when the address index
	// is disabled, ErrNilObserver for a nil observer and
	// ErrObserverRegistered when an observer is already registered; the
	// last check does not wait for in-flight transactions.
	SetReserveObserver(o ReserveObserver) error
}

// LocalStore is a read-only ChunkStore. It can be used to check if chunk is known
// locally, but it cannot tell what is the context of the chunk (whether it is
// pinned, uploaded, etc.).
type LocalStore interface {
	ChunkStore() storage.ReadOnlyChunkStore
}

// Debugger is a helper interface which can be used to debug the storer.
type Debugger interface {
	DebugInfo(context.Context) (Info, error)
}

type NeighborhoodStats interface {
	NeighborhoodsStat(ctx context.Context) ([]*NeighborhoodStat, error)
}

type memFS struct {
	afero.Fs
}

func (m *memFS) Open(path string) (fs.File, error) {
	return m.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
}

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0o600)
}

var (
	sharkyNoOfShards = 32
	ErrDBQuit        = errors.New("db quit")
)

type closerFn func() error

func (c closerFn) Close() error { return c() }

func closer(closers ...io.Closer) io.Closer {
	return closerFn(func() error {
		var err error
		for _, closer := range closers {
			err = errors.Join(err, closer.Close())
		}
		return err
	})
}

func initInmemRepository() (transaction.Storage, io.Closer, error) {
	store, _, err := leveldbstore.New("", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating inmem levelDB index store: %w", err)
	}

	sharky, err := sharky.New(
		&memFS{Fs: afero.NewMemMapFs()},
		sharkyNoOfShards,
		swarm.SocMaxChunkSize,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating inmem sharky instance: %w", err)
	}

	return transaction.NewStorage(sharky, store), closer(store, sharky), nil
}

// loggerName is the tree path name of the logger for this package.
const loggerName = "storer"

// Default options for levelDB.
const (
	defaultOpenFilesLimit         = uint64(256)
	defaultBlockCacheCapacity     = uint64(32 * 1024 * 1024)
	defaultWriteBufferSize        = uint64(32 * 1024 * 1024)
	defaultDisableSeeksCompaction = false
	defaultCacheCapacity          = uint64(1_000_000)
	defaultBgCacheWorkers         = 32
	DefaultReserveCapacity        = 1 << 22 // 4194304 chunks

	indexPath  = "indexstore"
	sharkyPath = "sharky"
)

func initStore(basePath string, opts *Options) (*leveldbstore.Store, error) {
	ldbBasePath := path.Join(basePath, indexPath)

	if _, err := os.Stat(ldbBasePath); os.IsNotExist(err) {
		err := os.MkdirAll(ldbBasePath, 0o700)
		if err != nil {
			return nil, err
		}
	}
	store, _, err := leveldbstore.New(path.Join(basePath, "indexstore"), &opt.Options{
		OpenFilesCacheCapacity: int(opts.LdbOpenFilesLimit),
		BlockCacheCapacity:     int(opts.LdbBlockCacheCapacity),
		WriteBuffer:            int(opts.LdbWriteBufferSize),
		DisableSeeksCompaction: opts.LdbDisableSeeksCompaction,
		CompactionL0Trigger:    8,
		Filter:                 filter.NewBloomFilter(64),
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating levelDB index store: %w", err)
	}

	return store, nil
}

func initDiskRepository(
	ctx context.Context,
	basePath string,
	opts *Options,
) (transaction.Storage, *PinIntegrity, io.Closer, int, error) {
	store, err := initStore(basePath, opts)
	if err != nil {
		return nil, nil, nil, 0, fmt.Errorf("failed creating levelDB index store: %w", err)
	}

	err = migration.Migrate(store, "core-migration", localmigration.BeforeInitSteps(store, opts.Logger))
	if err != nil {
		return nil, nil, nil, 0, errors.Join(store.Close(), fmt.Errorf("failed core migration: %w", err))
	}

	if opts.LdbStats.Load() != nil {
		go func() {
			ldbStats := opts.LdbStats.Load()
			logger := log.NewLogger(loggerName).Register()
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					stats := new(leveldb.DBStats)
					switch err := store.DB().Stats(stats); {
					case errors.Is(err, leveldb.ErrClosed):
						return
					case err != nil:
						logger.Error(err, "snapshot levelDB stats")
					default:
						ldbStats.WithLabelValues("write_delay_count").Observe(float64(stats.WriteDelayCount))
						ldbStats.WithLabelValues("write_delay_duration").Observe(stats.WriteDelayDuration.Seconds())
						ldbStats.WithLabelValues("alive_snapshots").Observe(float64(stats.AliveSnapshots))
						ldbStats.WithLabelValues("alive_iterators").Observe(float64(stats.AliveIterators))
						ldbStats.WithLabelValues("io_write").Observe(float64(stats.IOWrite))
						ldbStats.WithLabelValues("io_read").Observe(float64(stats.IORead))
						ldbStats.WithLabelValues("block_cache_size").Observe(float64(stats.BlockCacheSize))
						ldbStats.WithLabelValues("opened_tables_count").Observe(float64(stats.OpenedTablesCount))
						ldbStats.WithLabelValues("mem_comp").Observe(float64(stats.MemComp))
						ldbStats.WithLabelValues("level_0_comp").Observe(float64(stats.Level0Comp))
						ldbStats.WithLabelValues("non_level_0_comp").Observe(float64(stats.NonLevel0Comp))
						ldbStats.WithLabelValues("seek_comp").Observe(float64(stats.SeekComp))
						for i := 0; i < len(stats.LevelSizes); i++ {
							ldbStats.WithLabelValues(fmt.Sprintf("level_%d_size", i)).Observe(float64(stats.LevelSizes[i]))
							ldbStats.WithLabelValues(fmt.Sprintf("level_%d_tables_count", i)).Observe(float64(stats.LevelTablesCounts[i]))
							ldbStats.WithLabelValues(fmt.Sprintf("level_%d_read", i)).Observe(float64(stats.LevelRead[i]))
							ldbStats.WithLabelValues(fmt.Sprintf("level_%d_write", i)).Observe(float64(stats.LevelWrite[i]))
							ldbStats.WithLabelValues(fmt.Sprintf("level_%d_duration", i)).Observe(stats.LevelDurations[i].Seconds())
						}
					}
				}
			}
		}()
	}

	sharkyBasePath := path.Join(basePath, sharkyPath)

	if _, err := os.Stat(sharkyBasePath); os.IsNotExist(err) {
		err := os.Mkdir(sharkyBasePath, 0o700)
		if err != nil {
			return nil, nil, nil, 0, err
		}
	}

	recoveryCloser, pruned, err := sharkyRecovery(ctx, sharkyBasePath, store, opts)
	if err != nil {
		return nil, nil, nil, 0, fmt.Errorf("failed to recover sharky: %w", err)
	}

	sharky, err := sharky.New(
		&dirFS{basedir: sharkyBasePath},
		sharkyNoOfShards,
		swarm.SocMaxChunkSize,
	)
	if err != nil {
		return nil, nil, nil, 0, fmt.Errorf("failed creating sharky instance: %w", err)
	}

	pinIntegrity := &PinIntegrity{
		Store:  store,
		Sharky: sharky,
	}

	return transaction.NewStorage(sharky, store), pinIntegrity, closer(store, sharky, recoveryCloser), pruned, nil
}

const lockKeyNewSession string = "new_session"

// Options provides a container to configure different things in the storer.
type Options struct {
	// These are options related to levelDB. Currently, the underlying storage used is levelDB.
	LdbStats                  atomic.Pointer[prometheus.HistogramVec]
	LdbOpenFilesLimit         uint64
	LdbBlockCacheCapacity     uint64
	LdbWriteBufferSize        uint64
	LdbDisableSeeksCompaction bool
	Logger                    log.Logger
	Tracer                    *tracing.Tracer

	Address           swarm.Address
	StartupStabilizer stabilization.Subscriber
	Batchstore        postage.Storer
	ValidStamp        postage.ValidStampFn
	RadiusSetter      topology.SetStorageRadiuser
	StateStore        storage.StateStorer

	ReserveCapacity         int
	ReserveWakeUpDuration   time.Duration
	ReserveMinEvictCount    uint64
	ReserveCapacityDoubling int

	CacheCapacity      uint64
	CacheMinEvictCount uint64

	MinimumStorageRadius uint

	// ReserveAddrIndex enables the reserve address index, the
	// (chunk address, stamp hash) set served by ReserveAddrStore and
	// reported to a ReserveObserver. The node enables it only on full nodes
	// that run rangesync. When it is false (the default) no address index
	// entry is written and the reserve behaves as without the index. A
	// storer without a reserve (ReserveCapacity 0) treats it as false.
	//
	// Validity marker. The index is trusted only behind its marker, which
	// records the index layout version and a digest of the reserve state:
	// the reserve epoch (0 when the epoch item is absent), the last bin ID
	// of every bin, and the number of BatchRadiusItems counted in the
	// store (never the in-memory reserve size, which can drift). The epoch
	// and the bin IDs are read first and the count after them, in one
	// store snapshot where possible. Within an epoch bin IDs only grow and
	// every put takes a new one, and a removal without a put lowers the
	// count, so the digest matches only a reserve that has not changed
	// since the marker was written. A read torn by a concurrent write is
	// safe in this order: a put landing between the two reads leaves the
	// recorded bin IDs behind the store's for good, so the marker never
	// matches again (a refill, never a stale index); a removal between them
	// is in the count and in the index alike. The marker is written when a
	// fill completes and rewritten by a clean Close (see Close). A reserve
	// changed by anything that does not maintain the index (an older bee
	// build after a downgrade, a storer opened with the index disabled or
	// in keep mode) no longer matches it. After an unclean shutdown the
	// reserve normally does not match either, so a crash costs a refill,
	// although the index, written in the same transactions as the reserve,
	// is still exact. A reserve reset deletes the epoch item and only the
	// next start recreates it (with a new value), so a reset costs one
	// refill at the next start.
	//
	// Rule for migrations and tools: anything that rewrites reserve items
	// while reusing bin IDs, or changes stamp hashes in place, must delete
	// the epoch item or the marker, or the digest could match a changed
	// reserve.
	//
	// New reconciles the option with the marker at start-up; this is not a
	// numbered migration:
	//   - enabled, marker of the current version whose digest matches:
	//     nothing to do;
	//   - enabled, otherwise: the index entries are dropped synchronously in
	//     New, before index writes are switched on, then the index is filled
	//     from the reserve and the marker is written last, so an interrupted
	//     fill is redone at the next start. Only the fill may run in the
	//     background (after New returned); until the marker is written
	//     ReserveAddrIndexReady reports false;
	//   - disabled: if a marker exists it is deleted (writes stop, so the
	//     index would go stale); if any index entry exists, the entries are
	//     dropped in a background goroutine. With neither present nothing is
	//     written. An interrupted drop is resumed at the next start with
	//     the index disabled;
	//   - keep: see ReserveAddrIndexKeep.
	//
	// After a drop completes, the index key range is compacted, so the
	// tombstones it left do not slow down later seeks into the range.
	//
	// Start-up recovery of the chunk store (before the reserve opens)
	// deletes the index entry of each corrupted chunk it removes whenever
	// the index is enabled or a marker is present.
	//
	// Turning the index off and on again therefore costs one refill, and a
	// stale index is never served.
	ReserveAddrIndex bool

	// ReserveAddrIndexKeep is for tooling (the bee db commands) that opens
	// the storer with New without serving rangesync. With ReserveAddrIndex
	// false it leaves the marker and the index entries as they are, instead
	// of deleting them, so a command that does not change the reserve
	// (info, export, import of pinning collections) does not force a
	// refill. The index is not maintained meanwhile, and ReserveAddrStore
	// behaves as with the index disabled. A command that changes the
	// reserve (import of a reserve) should open the storer with the index
	// disabled, so the marker is deleted explicitly; a change made in keep
	// mode anyway is caught by the marker's digest at the next enabled
	// start. It is ignored when ReserveAddrIndex is true.
	ReserveAddrIndexKeep bool

	// reserveTestOptions are passed to reserve.New after the others (set by
	// tests through export_test.go).
	reserveTestOptions []reserve.Option
}

func defaultOptions() *Options {
	return &Options{
		LdbOpenFilesLimit:         defaultOpenFilesLimit,
		LdbBlockCacheCapacity:     defaultBlockCacheCapacity,
		LdbWriteBufferSize:        defaultWriteBufferSize,
		LdbDisableSeeksCompaction: defaultDisableSeeksCompaction,
		CacheCapacity:             defaultCacheCapacity,
		Logger:                    log.Noop,
		ReserveCapacity:           DefaultReserveCapacity,
		ReserveWakeUpDuration:     time.Minute * 30,
	}
}

// cacheLimiter is used to limit the number
// of concurrent cache background workers.
type cacheLimiter struct {
	wg     sync.WaitGroup
	sem    chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

// DB implements all the component stores described above.
type DB struct {
	logger log.Logger
	tracer *tracing.Tracer

	metrics             metrics
	storage             transaction.Storage
	multex              *multex.Multex
	cacheObj            *cache.Cache
	retrieval           retrieval.Interface
	pusherFeed          chan *pusher.Op
	quit                chan struct{}
	cacheLimiter        cacheLimiter
	dbCloser            io.Closer
	subscriptionsWG     sync.WaitGroup
	events              *events.Subscriber
	directUploadLimiter chan struct{}

	reserve          *reserve.Reserve
	inFlight         sync.WaitGroup
	reserveBinEvents *events.Subscriber
	baseAddr         swarm.Address
	batchstore       postage.Storer
	validStamp       postage.ValidStampFn
	setSyncerOnce    sync.Once
	syncer           Syncer
	reserveOptions   reserveOpts

	pinIntegrity *PinIntegrity

	addrIndex addrIndexState
}

type reserveOpts struct {
	startupStabilizer  stabilization.Subscriber
	wakeupDuration     time.Duration
	minEvictCount      uint64
	cacheMinEvictCount uint64
	minimumRadius      uint8
	capacityDoubling   int
}

// New returns a newly constructed DB object which implements all the above
// component stores.
func New(ctx context.Context, dirPath string, opts *Options) (*DB, error) {
	var (
		err          error
		pinIntegrity *PinIntegrity
		st           transaction.Storage
		dbCloser     io.Closer
		pruned       int
	)
	if opts == nil {
		opts = defaultOptions()
	}

	if opts.Logger == nil {
		opts.Logger = log.Noop
	}

	lock := multex.New()
	metrics := newMetrics()
	opts.LdbStats.CompareAndSwap(nil, metrics.LevelDBStats)

	if dirPath == "" {
		st, dbCloser, err = initInmemRepository()
		if err != nil {
			return nil, err
		}
	} else {
		st, pinIntegrity, dbCloser, pruned, err = initDiskRepository(ctx, dirPath, opts)
		if err != nil {
			return nil, err
		}
	}

	defer func() {
		if err != nil && dbCloser != nil {
			err = errors.Join(err, dbCloser.Close())
		}
	}()

	sharkyBasePath := ""
	if dirPath != "" {
		sharkyBasePath = path.Join(dirPath, sharkyPath)
	}

	err = st.Run(ctx, func(s transaction.Store) error {
		return migration.Migrate(
			s.IndexStore(),
			"migration",
			localmigration.AfterInitSteps(sharkyBasePath, sharkyNoOfShards, st, opts.Logger),
		)
	})
	if err != nil {
		return nil, fmt.Errorf("failed regular migration: %w", err)
	}

	cacheObj, err := cache.New(ctx, st.IndexStore(), opts.CacheCapacity)
	if err != nil {
		return nil, err
	}

	logger := opts.Logger.WithName(loggerName).Register()

	clCtx, clCancel := context.WithCancel(ctx)
	db := &DB{
		metrics:    metrics,
		storage:    st,
		logger:     logger,
		tracer:     opts.Tracer,
		baseAddr:   opts.Address,
		multex:     lock,
		cacheObj:   cacheObj,
		retrieval:  noopRetrieval{},
		pusherFeed: make(chan *pusher.Op),
		quit:       make(chan struct{}),
		cacheLimiter: cacheLimiter{
			sem:    make(chan struct{}, defaultBgCacheWorkers),
			ctx:    clCtx,
			cancel: clCancel,
		},
		dbCloser:         dbCloser,
		batchstore:       opts.Batchstore,
		validStamp:       opts.ValidStamp,
		events:           events.NewSubscriber(),
		reserveBinEvents: events.NewSubscriber(),
		reserveOptions: reserveOpts{
			startupStabilizer:  opts.StartupStabilizer,
			wakeupDuration:     opts.ReserveWakeUpDuration,
			minEvictCount:      opts.ReserveMinEvictCount,
			cacheMinEvictCount: opts.CacheMinEvictCount,
			minimumRadius:      uint8(opts.MinimumStorageRadius),
			capacityDoubling:   opts.ReserveCapacityDoubling,
		},
		directUploadLimiter: make(chan struct{}, pusher.ConcurrentPushes),
		pinIntegrity:        pinIntegrity,
	}

	if db.validStamp == nil {
		db.validStamp = postage.ValidStamp(db.batchstore)
	}

	if opts.ReserveCapacity > 0 {
		rs, err := reserve.New(
			opts.Address,
			st,
			opts.ReserveCapacity,
			opts.RadiusSetter,
			logger,
			append([]reserve.Option{reserve.WithAddrIndex(addrIndexModeOf(opts) == addrIndexOn)}, opts.reserveTestOptions...)...,
		)
		if err != nil {
			return nil, err
		}
		db.reserve = rs

		db.metrics.StorageRadius.Set(float64(rs.Radius()))
		db.metrics.ReserveSize.Set(float64(rs.Size()))
	}
	db.metrics.CacheSize.Set(float64(db.cacheObj.Size()))
	if pruned > 0 {
		db.metrics.RecoveryPrunedChunkCount.Add(float64(pruned))
	}

	// Cleanup any dirty state in upload and pinning stores, this could happen
	// in case of dirty shutdowns
	err = errors.Join(
		upload.CleanupDirty(db.storage),
		pinstore.CleanupDirty(db.storage),
	)
	if err != nil {
		return nil, err
	}

	err = db.startAddrIndex(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("reserve address index: %w", err)
	}

	db.inFlight.Add(1)
	go db.cacheWorker(ctx)

	return db, nil
}

// ResetReserve removes all entries of the reserve.
//
// With the address index enabled the index ends up empty and valid: the fill
// completes in New, so none runs here; the reset deletes each element's index
// entry, then ResetReserve writes the marker. If the reset fails, the marker
// is not written. The reset deletes the reserve epoch item, so the marker it
// writes no longer matches at the next start: a reset costs one refill then
// (see Options.ReserveAddrIndex).
func (db *DB) ResetReserve(ctx context.Context) error {
	if err := db.reserve.Reset(ctx); err != nil {
		return err
	}
	if db.addrIndex.mode == addrIndexOn {
		if err := db.reserve.WriteAddrIndexMarker(ctx); err != nil {
			return fmt.Errorf("reset reserve: %w", err)
		}
	}
	return nil
}

// Metrics returns set of prometheus collectors.
func (db *DB) Metrics() []prometheus.Collector {
	collectors := m.PrometheusCollectorsFromFields(db.metrics)
	if v, ok := db.storage.(m.Collector); ok {
		collectors = append(collectors, v.Metrics()...)
	}
	return collectors
}

// StatusMetrics exposes metrics that are exposed on the status protocol.
func (db *DB) StatusMetrics() []prometheus.Collector {
	collectors := []prometheus.Collector{
		db.metrics.MethodCallsDuration,
	}

	type Collector interface {
		StatusMetrics() []prometheus.Collector
	}

	if v, ok := db.storage.(Collector); ok {
		collectors = append(collectors, v.StatusMetrics()...)
	}

	return collectors
}

// Close stops the DB and closes the store.
//
// With the address index disabled or in keep mode and no address index
// goroutine running, Close behaves as before the address index existed.
// Otherwise it runs in this order:
//  1. cancel a running address index fill or drop and wait for it; a fill
//     canceled by Close writes no marker;
//  2. wait for the reserve and cache workers, as before;
//  3. only if the index is enabled, ReserveAddrIndexReady reports true and
//     every wait above finished in time: compute the reserve digest and
//     rewrite the marker (see Options.ReserveAddrIndex). This step has its
//     own deadline, chosen from the measured digest time on a full
//     reserve; when it passes, or the step fails, no marker is written and
//     the next enabled start refills (an older marker left in place never
//     matches a reserve that changed after it was written);
//  4. close the store, under the existing 3 s limit.
func (db *DB) Close() error {
	close(db.quit)

	if db.addrIndex.active() {
		return db.closeWithAddrIndex()
	}

	bgReserveWorkersClosed := make(chan struct{})
	go func() {
		defer close(bgReserveWorkersClosed)
		if !syncutil.WaitWithTimeout(&db.inFlight, 5*time.Second) {
			db.logger.Warning("db shutting down with running goroutines")
		}
	}()

	bgCacheWorkersClosed := make(chan struct{})
	go func() {
		defer close(bgCacheWorkersClosed)
		if !syncutil.WaitWithTimeout(&db.cacheLimiter.wg, 5*time.Second) {
			db.logger.Warning("cache goroutines still running after the wait timeout; force closing")
			db.cacheLimiter.cancel()
		}
	}()

	var err error
	closerDone := make(chan struct{})
	go func() {
		defer close(closerDone)
		err = db.dbCloser.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		<-closerDone
		<-bgCacheWorkersClosed
		<-bgReserveWorkersClosed
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		return errors.New("storer closed with bg goroutines running")
	}

	return err
}

func (db *DB) SetRetrievalService(r retrieval.Interface) {
	db.retrieval = r
}

// StartReserveWorker starts the reserve worker. It takes an optional ready channel that is closed whenever the reserve
// worker has finished starting, as this synchronization is needed for some tests. The channel is not used for writing anywhere.
func (db *DB) StartReserveWorker(ctx context.Context, s Syncer, radius func() (uint8, error), ready chan<- struct{}) {
	db.setSyncerOnce.Do(func() {
		db.syncer = s
		go db.startReserveWorkers(ctx, radius, ready)
	})
}

type noopRetrieval struct{}

func (noopRetrieval) RetrieveChunk(_ context.Context, _ swarm.Address, _ swarm.Address) (swarm.Chunk, error) {
	return nil, storage.ErrNotFound
}

func (db *DB) ChunkStore() storage.ReadOnlyChunkStore {
	return db.storage.ChunkStore()
}

func (db *DB) PinIntegrity() *PinIntegrity {
	return db.pinIntegrity
}

func (db *DB) Lock(strs ...string) func() {
	for _, s := range strs {
		db.multex.Lock(s)
	}
	return func() {
		for _, s := range strs {
			db.multex.Unlock(s)
		}
	}
}

func (db *DB) Storage() transaction.Storage {
	return db.storage
}

type putterSession struct {
	storage.Putter
	done    func(swarm.Address) error
	cleanup func() error
}

func (p *putterSession) Done(addr swarm.Address) error { return p.done(addr) }

func (p *putterSession) Cleanup() error { return p.cleanup() }
