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

	"github.com/ethersphere/bee/pkg/log"
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/pkg/storage/migration"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/storer/internal/events"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/pkg/storer/migration"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/util"
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
	// BatchHint will return the batch ID hint for the chunk reference if known.
	BatchHint(swarm.Address) ([]byte, error)
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
	Download(pin bool) storage.Getter
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
	ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte) (swarm.Chunk, error)
	ReserveHas(addr swarm.Address, batchID []byte) (bool, error)
	ReservePutter() storage.Putter
	SubscribeBin(ctx context.Context, bin uint8, start uint64) (<-chan *BinC, func(), <-chan error)
	ReserveLastBinIDs() ([]uint64, error)
	RadiusChecker
}

// RadiusChecker provides the radius related functionality.
type RadiusChecker interface {
	IsWithinStorageRadius(addr swarm.Address) bool
	StorageRadius() uint8
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

type memFS struct {
	afero.Fs
}

func (m *memFS) Open(path string) (fs.File, error) {
	return m.Fs.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
}

type dirFS struct {
	basedir string
}

func (d *dirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
}

var sharkyNoOfShards = 32
var ErrDBQuit = errors.New("db quit")

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

func initInmemRepository() (storage.Repository, io.Closer, error) {
	store, err := leveldbstore.New("", nil)
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

	txStore := leveldbstore.NewTxStore(store)
	txChunkStore := chunkstore.NewTxChunkStore(txStore, sharky)

	return storage.NewRepository(txStore, txChunkStore), closer(store, sharky), nil
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
	defaultBgCacheWorkers         = 16

	indexPath  = "indexstore"
	sharkyPath = "sharky"
)

func initStore(basePath string, opts *Options) (*leveldbstore.Store, error) {
	ldbBasePath := path.Join(basePath, indexPath)

	if _, err := os.Stat(ldbBasePath); os.IsNotExist(err) {
		err := os.MkdirAll(ldbBasePath, 0777)
		if err != nil {
			return nil, err
		}
	}
	store, err := leveldbstore.New(path.Join(basePath, "indexstore"), &opt.Options{
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

func initDiskRepository(ctx context.Context, basePath string, opts *Options) (storage.Repository, io.Closer, error) {
	store, err := initStore(basePath, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating levelDB index store: %w", err)
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
		err := os.Mkdir(sharkyBasePath, 0777)
		if err != nil {
			return nil, nil, err
		}
	}

	recoveryCloser, err := sharkyRecovery(ctx, sharkyBasePath, store, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to recover sharky: %w", err)
	}

	sharky, err := sharky.New(
		&dirFS{basedir: sharkyBasePath},
		sharkyNoOfShards,
		swarm.SocMaxChunkSize,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating sharky instance: %w", err)
	}

	txStore := leveldbstore.NewTxStore(store)
	if err := txStore.Recover(); err != nil {
		return nil, nil, fmt.Errorf("failed to recover index store: %w", err)
	}

	txChunkStore := chunkstore.NewTxChunkStore(txStore, sharky)
	if err := txChunkStore.Recover(); err != nil {
		return nil, nil, fmt.Errorf("failed to recover chunk store: %w", err)
	}

	return storage.NewRepository(txStore, txChunkStore), closer(store, sharky, recoveryCloser), nil
}

func initCache(ctx context.Context, capacity uint64, repo storage.Repository) (*cache.Cache, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	txnRepo, commit, rollback := repo.NewTx(ctx)
	c, err := cache.New(ctx, txnRepo, capacity)
	if err != nil {
		return nil, fmt.Errorf("cache.New: %w", errors.Join(err, rollback()))
	}

	return c, commit()
}

type noopRadiusSetter struct{}

func (noopRadiusSetter) SetStorageRadius(uint8) {}

func performEpochMigration(ctx context.Context, basePath string, opts *Options) (retErr error) {
	store, err := initStore(basePath, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	sharkyBasePath := path.Join(basePath, sharkyPath)
	var sharkyRecover *sharky.Recovery
	// if this is a fresh node then perform an empty epoch migration
	if _, err := os.Stat(sharkyBasePath); err == nil {
		sharkyRecover, err = sharky.NewRecovery(sharkyBasePath, sharkyNoOfShards, swarm.SocMaxChunkSize)
		if err != nil {
			return err
		}
		defer sharkyRecover.Close()
	}

	logger := opts.Logger.WithName("epochmigration").Register()

	var rs reservePutter

	if opts.ReserveCapacity > 0 {
		rs, err = reserve.New(
			opts.Address,
			store,
			opts.ReserveCapacity,
			noopRadiusSetter{},
			logger,
			func(_ context.Context, _ internal.Storage, _ ...swarm.Address) error {
				return nil
			},
		)
		if err != nil {
			return err
		}
	}

	defer func() {
		if sharkyRecover != nil {
			retErr = errors.Join(retErr, sharkyRecover.Save())
		}
	}()

	return epochMigration(ctx, basePath, opts.StateStore, store, rs, sharkyRecover, logger)
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
	CacheCapacity             uint64
	Logger                    log.Logger

	Address        swarm.Address
	WarmupDuration time.Duration
	Batchstore     postage.Storer
	ValidStamp     postage.ValidStampFn
	RadiusSetter   topology.SetStorageRadiuser
	StateStore     storage.StateStorer

	ReserveCapacity       int
	ReserveWakeUpDuration time.Duration
}

func defaultOptions() *Options {
	return &Options{
		LdbOpenFilesLimit:         defaultOpenFilesLimit,
		LdbBlockCacheCapacity:     defaultBlockCacheCapacity,
		LdbWriteBufferSize:        defaultWriteBufferSize,
		LdbDisableSeeksCompaction: defaultDisableSeeksCompaction,
		CacheCapacity:             defaultCacheCapacity,
		Logger:                    log.Noop,
		ReserveCapacity:           4_194_304, // 2^22 chunks
		ReserveWakeUpDuration:     time.Minute * 15,
	}
}

// DB implements all the component stores described above.
type DB struct {
	logger  log.Logger
	metrics metrics

	repo                storage.Repository
	lock                *multex.Multex
	cacheObj            *cache.Cache
	retrieval           retrieval.Interface
	pusherFeed          chan *pusher.Op
	quit                chan struct{}
	bgCacheLimiter      chan struct{}
	bgCacheLimiterWg    sync.WaitGroup
	dbCloser            io.Closer
	subscriptionsWG     sync.WaitGroup
	events              *events.Subscriber
	directUploadLimiter chan struct{}

	reserve          *reserve.Reserve
	inFlight         *util.WaitingCounter
	reserveBinEvents *events.Subscriber
	baseAddr         swarm.Address
	batchstore       postage.Storer
	validStamp       postage.ValidStampFn
	setSyncerOnce    sync.Once
	syncer           Syncer
	opts             workerOpts
}

type workerOpts struct {
	warmupDuration time.Duration
	wakeupDuration time.Duration
}

// New returns a newly constructed DB object which implements all the above
// component stores.
func New(ctx context.Context, dirPath string, opts *Options) (*DB, error) {
	var (
		repo     storage.Repository
		err      error
		dbCloser io.Closer
	)
	if opts == nil {
		opts = defaultOptions()
	}

	metrics := newMetrics()
	opts.LdbStats.CompareAndSwap(nil, &metrics.LevelDBStats)

	if dirPath == "" {
		repo, dbCloser, err = initInmemRepository()
		if err != nil {
			return nil, err
		}
	} else {
		// only perform migration if not done already
		if _, err := os.Stat(path.Join(dirPath, indexPath)); err != nil {
			err = performEpochMigration(ctx, dirPath, opts)
			if err != nil {
				return nil, err
			}
		}

		repo, dbCloser, err = initDiskRepository(ctx, dirPath, opts)
		if err != nil {
			return nil, err
		}
	}

	err = migration.Migrate(repo.IndexStore(), localmigration.AllSteps())
	if err != nil {
		return nil, err
	}

	cacheObj, err := initCache(ctx, opts.CacheCapacity, repo)
	if err != nil {
		return nil, err
	}

	logger := opts.Logger.WithName(loggerName).Register()

	db := &DB{
		metrics:          metrics,
		logger:           logger,
		baseAddr:         opts.Address,
		repo:             repo,
		lock:             multex.New(),
		cacheObj:         cacheObj,
		retrieval:        noopRetrieval{},
		pusherFeed:       make(chan *pusher.Op),
		quit:             make(chan struct{}),
		bgCacheLimiter:   make(chan struct{}, 16),
		dbCloser:         dbCloser,
		batchstore:       opts.Batchstore,
		validStamp:       opts.ValidStamp,
		events:           events.NewSubscriber(),
		reserveBinEvents: events.NewSubscriber(),
		opts: workerOpts{
			warmupDuration: opts.WarmupDuration,
			wakeupDuration: opts.ReserveWakeUpDuration,
		},
		directUploadLimiter: make(chan struct{}, pusher.ConcurrentPushes),
		inFlight:            new(util.WaitingCounter),
	}

	if db.validStamp == nil {
		db.validStamp = postage.ValidStamp(db.batchstore)
	}

	if opts.ReserveCapacity > 0 {
		rs, err := reserve.New(
			opts.Address,
			repo.IndexStore(),
			opts.ReserveCapacity,
			opts.RadiusSetter,
			logger,
			func(ctx context.Context, store internal.Storage, addrs ...swarm.Address) error {
				defer func() { db.metrics.CacheSize.Set(float64(db.cacheObj.Size())) }()

				db.lock.Lock(cacheAccessLockKey)
				defer db.lock.Unlock(cacheAccessLockKey)

				return cacheObj.MoveFromReserve(ctx, store, addrs...)
			},
		)
		if err != nil {
			return nil, err
		}
		db.reserve = rs

		db.metrics.StorageRadius.Set(float64(rs.Radius()))
		db.metrics.ReserveSize.Set(float64(rs.Size()))
	}
	db.metrics.CacheSize.Set(float64(db.cacheObj.Size()))

	// Cleanup any dirty state in upload and pinning stores, this could happen
	// in case of dirty shutdowns
	err = errors.Join(
		upload.CleanupDirty(db),
		pinstore.CleanupDirty(db),
	)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Metrics returns set of prometheus collectors.
func (db *DB) Metrics() []prometheus.Collector {
	collectors := m.PrometheusCollectorsFromFields(db.metrics)
	if v, ok := db.repo.(m.Collector); ok {
		collectors = append(collectors, v.Metrics()...)
	}
	return collectors
}

func (db *DB) Close() error {
	close(db.quit)

	bgReserveWorkersClosed := make(chan struct{})
	go func() {
		defer close(bgReserveWorkersClosed)
		if c := db.inFlight.Wait(5 * time.Second); c > 0 {
			db.logger.Warning("db shutting down with running goroutines")
		}
	}()

	bgCacheWorkersClosed := make(chan struct{})
	go func() {
		defer close(bgCacheWorkersClosed)
		db.bgCacheLimiterWg.Wait()
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

func (db *DB) StartReserveWorker(ctx context.Context, s Syncer, radius func() (uint8, error)) {
	db.setSyncerOnce.Do(func() {
		db.syncer = s
		go db.startReserveWorkers(ctx, db.opts.warmupDuration, db.opts.wakeupDuration, radius)
	})
}

type noopRetrieval struct{}

func (noopRetrieval) RetrieveChunk(_ context.Context, _ swarm.Address, _ swarm.Address) (swarm.Chunk, error) {
	return nil, storage.ErrNotFound
}

func (db *DB) ChunkStore() storage.ReadOnlyChunkStore {
	return db.repo.ChunkStore()
}

// Execute implements the internal.TxExecutor interface.
func (db *DB) Execute(ctx context.Context, do func(internal.Storage) error) error {
	tx, commit, rollback := db.repo.NewTx(ctx)
	if err := do(tx); err != nil {
		return errors.Join(err, rollback())
	}
	return commit()
}

type putterSession struct {
	storage.Putter
	done    func(swarm.Address) error
	cleanup func() error
}

func (p *putterSession) Done(addr swarm.Address) error { return p.done(addr) }

func (p *putterSession) Cleanup() error { return p.cleanup() }
