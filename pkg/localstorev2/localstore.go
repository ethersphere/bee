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

type SessionInfo struct {
	Split     uint64        // total no of chunks processed by the splitter for hashing
	Seen      uint64        // total no of chunks already seen
	Stored    uint64        // total no of chunks stored locally on the node
	Sent      uint64        // total no of chunks sent to the neighbourhood
	Synced    uint64        // total no of chunks synced with proof
	Address   swarm.Address // swarm.Address associated with this tag
	StartedAt int64         // start timestamp
}

type UploadStore interface {
	Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error)
	NewSession(context.Context) (uint64, error)
	GetSessionInfo(ctx context.Context, tagID uint64) (SessionInfo, error)
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
	}, nil
}

type putterSessionImpl struct {
	storage.Putter
	done    func(swarm.Address) error
	cleanup func() error
}

func (p *putterSessionImpl) Done(addr swarm.Address) error { return p.done(addr) }

func (p *putterSessionImpl) Cleanup() error { return p.cleanup() }

func (db *DB) Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error) {
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

func (db *DB) NewSession(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (db *DB) GetSessionInfo(ctx context.Context, tagID uint64) (SessionInfo, error) {
	return SessionInfo{}, nil
}
