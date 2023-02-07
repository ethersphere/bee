// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/cache"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/afero"
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

// SessionInfo is a type which exports the localstore tag object. This object
// stores all the relevant information about a particular session.
type SessionInfo = upload.TagItem

// UploadStore is a logical component of the localstore which deals with the upload
// of data to swarm.
type UploadStore interface {
	// Upload provides a PutterSession which is tied to the tagID. Optionally if
	// users requests to pin the data, a new pinning collection is created.
	Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error)
	// NewSession can be used to obtain a tag ID to use for a new Upload session.
	NewSession() (uint64, error)
	// GetSessionInfo will show the information about the session.
	GetSessionInfo(tagID uint64) (SessionInfo, error)
}

// PinStore is a logical component of the localstore which deals with pinning
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

// CacheStore is a logical component of the localstore that deals with cache
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

type NetStore interface {
	DirectPush() storage.Putter
	Download() storage.Getter
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

type closerFn func() error

func (c closerFn) Close() error { return c() }

func closer(closers ...io.Closer) io.Closer {
	return closerFn(func() error {
		var err *multierror.Error
		for _, closer := range closers {
			err = multierror.Append(err, closer.Close())
		}
		return err.ErrorOrNil()
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

// Default options for levelDB.
const (
	defaultOpenFilesLimit         = uint64(256)
	defaultBlockCacheCapacity     = uint64(32 * 1024 * 1024)
	defaultWriteBufferSize        = uint64(32 * 1024 * 1024)
	defaultDisableSeeksCompaction = false
	defaultCacheCapacity          = uint64(1_000_000)
)

func initDiskRepository(basePath string, opts *Options) (storage.Repository, io.Closer, error) {
	ldbBasePath := path.Join(basePath, "indexstore")

	if _, err := os.Stat(ldbBasePath); os.IsNotExist(err) {
		err := os.Mkdir(ldbBasePath, 0777)
		if err != nil {
			return nil, nil, err
		}
	}
	store, err := leveldbstore.New(path.Join(basePath, "indexstore"), &opt.Options{
		OpenFilesCacheCapacity: int(opts.LdbOpenFilesLimit),
		BlockCacheCapacity:     int(opts.LdbBlockCacheCapacity),
		WriteBuffer:            int(opts.LdbWriteBufferSize),
		DisableSeeksCompaction: opts.LdbDisableSeeksCompaction,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating levelDB index store: %w", err)
	}

	sharkyBasePath := path.Join(basePath, "sharky")

	if _, err := os.Stat(sharkyBasePath); os.IsNotExist(err) {
		err := os.Mkdir(sharkyBasePath, 0777)
		if err != nil {
			return nil, nil, err
		}
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
	txChunkStore := chunkstore.NewTxChunkStore(txStore, sharky)

	return storage.NewRepository(txStore, txChunkStore), closer(store, sharky), nil
}

func initCache(capacity uint64, repo storage.Repository) (*cache.Cache, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	txnRepo, commit, rollback := repo.NewTx(ctx)
	c, err := cache.New(ctx, txnRepo, capacity)
	if err != nil {
		return nil, multierror.Append(err, rollback())
	}

	return c, commit()
}

const (
	lockKeyNewSession string = "new_session"
)

// Options provides a container to configure different things in the localstore.
type Options struct {
	// These are options related to levelDB. Currently the underlying storage used
	// is levelDB.
	LdbOpenFilesLimit         uint64
	LdbBlockCacheCapacity     uint64
	LdbWriteBufferSize        uint64
	LdbDisableSeeksCompaction bool
	CacheCapacity             uint64
}

func defaultOptions() *Options {
	return &Options{
		LdbOpenFilesLimit:         defaultOpenFilesLimit,
		LdbBlockCacheCapacity:     defaultBlockCacheCapacity,
		LdbWriteBufferSize:        defaultWriteBufferSize,
		LdbDisableSeeksCompaction: defaultDisableSeeksCompaction,
		CacheCapacity:             defaultCacheCapacity,
	}
}

// DB implements all the component stores described above.
type DB struct {
	repo       storage.Repository
	lock       *multex.Multex
	cacheObj   *cache.Cache
	retrieval  retrieval.Interface
	pusherFeed chan *pusher.Op
	quit       chan struct{}
	dbCloser   io.Closer
}

// New returns a newly constructed DB object which implements all the above
// component stores.
func New(dirPath string, opts *Options) (*DB, error) {
	// TODO: migration handling and sharky recovery
	var (
		repo     storage.Repository
		err      error
		dbCloser io.Closer
	)
	if opts == nil {
		opts = defaultOptions()
	}
	if dirPath == "" {
		repo, dbCloser, err = initInmemRepository()
		if err != nil {
			return nil, err
		}
	} else {
		repo, dbCloser, err = initDiskRepository(dirPath, opts)
		if err != nil {
			return nil, err
		}
	}

	cacheObj, err := initCache(opts.CacheCapacity, repo)
	if err != nil {
		return nil, err
	}

	return &DB{
		repo:     repo,
		lock:     multex.New(),
		cacheObj: cacheObj,
		dbCloser: dbCloser,
	}, nil
}

func (db *DB) Close() error {
	return db.dbCloser.Close()
}

type putterSessionImpl struct {
	storage.Putter
	done    func(swarm.Address) error
	cleanup func() error
}

func (p *putterSessionImpl) Done(addr swarm.Address) error { return p.done(addr) }

func (p *putterSessionImpl) Cleanup() error { return p.cleanup() }

// Upload is the implementation of UploadStore.Upload method.
func (db *DB) Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error) {
	if tagID == 0 {
		return nil, fmt.Errorf("localstore: tagID required")
	}

	txnRepo, commit, rollback := db.repo.NewTx(ctx)

	uploadPutter, err := upload.NewPutter(txnRepo, tagID)
	if err != nil {
		return nil, err
	}

	var pinningPutter internal.PutterCloserWithReference
	if pin {
		pinningPutter = pinstore.NewCollection(txnRepo)
	}

	return &putterSessionImpl{
		Putter: storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
			return multierror.Append(
				uploadPutter.Put(ctx, chunk),
				func() error {
					if pinningPutter != nil {
						return pinningPutter.Put(ctx, chunk)
					}
					return nil
				}(),
			).ErrorOrNil()
		}),
		done: func(address swarm.Address) error {
			return multierror.Append(
				uploadPutter.Close(address),
				func() error {
					if pinningPutter != nil {
						return pinningPutter.Close(address)
					}
					return nil
				}(),
				commit(),
			).ErrorOrNil()
		},
		cleanup: func() error {
			return rollback()
		},
	}, nil
}

// NewSession is the implementation of UploadStore.NewSession method.
func (db *DB) NewSession() (uint64, error) {
	db.lock.Lock(lockKeyNewSession)
	defer db.lock.Unlock(lockKeyNewSession)

	return upload.NextTag(db.repo.IndexStore())
}

// GetSessionInfo is the implementation of the UploadStore.GetSessionInfo method.
func (db *DB) GetSessionInfo(tagID uint64) (SessionInfo, error) {
	return upload.GetTagInfo(db.repo.IndexStore(), tagID)
}

// NewCollection is the implementation of the PinStore.NewCollection method.
func (db *DB) NewCollection(ctx context.Context) (PutterSession, error) {
	txnRepo, commit, rollback := db.repo.NewTx(ctx)
	pinningPutter := pinstore.NewCollection(txnRepo)

	return &putterSessionImpl{
		Putter: pinningPutter,
		done: func(address swarm.Address) error {
			return multierror.Append(
				pinningPutter.Close(address),
				commit(),
			).ErrorOrNil()
		},
		cleanup: func() error { return rollback() },
	}, nil
}

// DeletePin is the implementation of the PinStore.DeletePin method.
func (db *DB) DeletePin(ctx context.Context, root swarm.Address) error {
	txnRepo, commit, rollback := db.repo.NewTx(ctx)

	err := pinstore.DeletePin(ctx, txnRepo, root)
	if err != nil {
		return multierror.Append(err, rollback()).ErrorOrNil()
	}

	return commit()
}

// Pins is the implementation of the PinStore.Pins method.
func (db *DB) Pins() ([]swarm.Address, error) {
	return pinstore.Pins(db.repo.IndexStore())
}

// HasPin is the implementation of the PinStore.HasPin method.
func (db *DB) HasPin(root swarm.Address) (bool, error) {
	return pinstore.HasPin(db.repo.IndexStore(), root)
}

// Lookup is the implementation of the CacheStore.Lookup method.
func (db *DB) Lookup() storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
		txnRepo, commit, rollback := db.repo.NewTx(ctx)
		ch, err := db.cacheObj.Getter(txnRepo).Get(ctx, address)
		switch {
		case err == nil:
			return ch, commit()
		case errors.Is(err, storage.ErrNotFound):
			// here we would ideally have nothing to do but just to return this
			// error to the client. The commit is mainly done to end the txn.
			return nil, multierror.Append(err, commit()).ErrorOrNil()
		}
		// if we are here, it means there was some unexpected error, in which
		// case we need to rollback any changes that were already made.
		return nil, multierror.Append(err, rollback()).ErrorOrNil()
	})
}

// Cache is the implementation of the CacheStore.Cache method.
func (db *DB) Cache() storage.Putter {
	return storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
		txnRepo, commit, rollback := db.repo.NewTx(ctx)
		err := db.cacheObj.Putter(txnRepo).Put(ctx, ch)
		if err != nil {
			return multierror.Append(err, rollback()).ErrorOrNil()
		}
		return multierror.Append(err, commit()).ErrorOrNil()
	})
}
