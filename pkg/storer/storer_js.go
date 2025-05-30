//go:build js
// +build js

package storer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/events"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"resenje.org/multex"
)

// DB implements all the component stores described above.
type DB struct {
	logger log.Logger
	tracer *tracing.Tracer

	storage             transaction.Storage
	multex              *multex.Multex[any]
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
}

// New returns a newly constructed DB object which implements all the above
// component stores.
func New(ctx context.Context, dirPath string, opts *Options) (*DB, error) {
	var (
		err          error
		pinIntegrity *PinIntegrity
		st           transaction.Storage
		dbCloser     io.Closer
	)
	if opts == nil {
		opts = defaultOptions()
	}

	if opts.Logger == nil {
		opts.Logger = log.Noop
	}

	lock := multex.New[any]()

	if dirPath == "" {
		st, dbCloser, err = initInmemRepository()
		if err != nil {
			return nil, err
		}
	} else {
		st, pinIntegrity, dbCloser, err = initDiskRepository(ctx, dirPath, opts)
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
		)
		if err != nil {
			return nil, err
		}
		db.reserve = rs

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

	db.inFlight.Add(1)
	go db.cacheWorker(ctx)

	return db, nil
}
