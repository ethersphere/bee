// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/afero"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"resenje.org/multex"
)

// PutterSession provides a session
type PutterSession interface {
	storage.Putter
	// Done is used to close the session and optionally assign a swarm.Address to
	// this session.
	Done(swarm.Address) error
	// Cleanup is used to cleanup any state related to this session in case of
	// any error.
	Cleanup() error
}

type SessionInfo = upload.TagItem

type UploadStore interface {
	Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error)
	NewSession() (uint64, error)
	GetSessionInfo(tagID uint64) (SessionInfo, error)
}

type PinStore interface {
	NewCollection(context.Context) (PutterSession, error)
	DeletePin(context.Context, swarm.Address) error
	Pins() ([]swarm.Address, error)
	HasPin(swarm.Address) (bool, error)
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

const sharkyNoOfShards = 32

func initInmemRepository() (*storage.Repository, error) {
	store, err := leveldbstore.New("", &opt.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed creating inmem levelDB index store: %w", err)
	}

	sharky, err := sharky.New(
		&memFS{Fs: afero.NewMemMapFs()},
		sharkyNoOfShards,
		swarm.SocMaxChunkSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating inmem sharky instance: %w", err)
	}

	txStore := leveldbstore.NewTxStore(store)
	txChunkStore := chunkstore.NewTxChunkStore(txStore, sharky)

	return storage.NewRepository(txStore, txChunkStore), nil
}

func initDiskRepository(basePath string) (*storage.Repository, error) {
	store, err := leveldbstore.New(path.Join(basePath, "indexstore"), &opt.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed creating inmem levelDB index store: %w", err)
	}

	sharky, err := sharky.New(
		&dirFS{basedir: path.Join(basePath, "sharky")},
		sharkyNoOfShards,
		swarm.SocMaxChunkSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating inmem sharky instance: %w", err)
	}

	txStore := leveldbstore.NewTxStore(store)
	txChunkStore := chunkstore.NewTxChunkStore(txStore, sharky)

	return storage.NewRepository(txStore, txChunkStore), nil
}

type Options struct{}

type DB struct {
	repo *storage.Repository
	lock *multex.Multex
}

func New(dirPath string, opts *Options) (*DB, error) {
	// TODO: migration handling and sharky recovery
	var (
		repo *storage.Repository
		err  error
	)
	if dirPath == "" {
		repo, err = initInmemRepository()
		if err != nil {
			return nil, err
		}
	} else {
		repo, err = initDiskRepository(dirPath)
		if err != nil {
			return nil, err
		}
	}

	return &DB{
		repo: repo,
		lock: multex.New(),
	}, nil
}

type putterSessionImpl struct {
	storage.Putter
	done    func(swarm.Address) error
	cleanup func() error
}

func (p *putterSessionImpl) Done(addr swarm.Address) error { return p.done(addr) }

func (p *putterSessionImpl) Cleanup() error { return p.cleanup() }

// Upload provides a PutterSession which can be used to upload data to the node.
// If pin is set to true, it also creates a new pinning collection for the session.
// If the tagID is specified, it updates the same tag, if not, it creates a new one.
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
			)
		},
		cleanup: func() error {
			return rollback()
		},
	}, nil
}

// NewSession provides a new tag ID to use for Upload session.
func (db *DB) NewSession() (uint64, error) {
	db.lock.Lock("upload_session")
	defer db.lock.Unlock("upload_session")

	return upload.NextTag(db.repo.IndexStore())
}

// GetSessionInfo returns the session related information for this tagID
func (db *DB) GetSessionInfo(tagID uint64) (SessionInfo, error) {
	return upload.GetTagInfo(db.repo.IndexStore(), tagID)
}

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

func (db *DB) DeletePin(ctx context.Context, root swarm.Address) error {
	txnRepo, commit, rollback := db.repo.NewTx(ctx)

	err := pinstore.DeletePin(txnRepo, root)
	if err != nil {
		return multierror.Append(err, rollback()).ErrorOrNil()
	}

	return commit()
}

func (db *DB) Pins() ([]swarm.Address, error) {
	return pinstore.Pins(db.repo.IndexStore())
}

func (db *DB) HasPin(root swarm.Address) (bool, error) {
	return pinstore.HasPin(db.repo.IndexStore(), root)
}
