// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Upload is the implementation of UploadStore.Upload method.
func (db *DB) Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error) {
	if tagID == 0 {
		return nil, fmt.Errorf("storer: tagID required")
	}

	var (
		uploadPutter  internal.PutterCloserWithReference
		pinningPutter internal.PutterCloserWithReference
		err           error
	)

	err = db.Execute(ctx, func(txnRepo internal.Storage) error {
		uploadPutter, err = upload.NewPutter(txnRepo, tagID)
		if err != nil {
			return fmt.Errorf("upload.NewPutter: %w", err)
		}

		if pin {
			pinningPutter, err = pinstore.NewCollection(txnRepo)
			if err != nil {
				return fmt.Errorf("pinstore.NewCollection: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	var mtx sync.Mutex

	return &putterSession{
		Putter: putterWithMetrics{
			storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
				mtx.Lock()
				defer mtx.Unlock()

				return db.Execute(ctx, func(s internal.Storage) error {

					b, err := s.IndexStore().Batch(ctx)
					if err != nil {
						return err
					}
					err = errors.Join(
						uploadPutter.Put(ctx, s, b, chunk),
						func() error {
							if pinningPutter != nil {
								return pinningPutter.Put(ctx, s, b, chunk)
							}
							return nil
						}(),
					)
					if err != nil {
						return err
					}
					return b.Commit()
				})
			}),
			db.metrics,
			"uploadstore",
		},
		done: func(address swarm.Address) error {
			mtx.Lock()
			defer mtx.Unlock()
			defer db.events.Trigger(subscribePushEventKey)

			return db.Execute(ctx, func(s internal.Storage) error {

				b, err := s.IndexStore().Batch(ctx)
				if err != nil {
					return err
				}

				err = errors.Join(
					uploadPutter.Close(s, b, address),
					func() error {
						if pinningPutter != nil {
							return pinningPutter.Close(s, b, address)
						}
						return nil
					}(),
				)
				if err != nil {
					return err
				}
				return b.Commit()
			})
		},
		cleanup: func() error {
			mtx.Lock()
			defer mtx.Unlock()
			defer db.events.Trigger(subscribePushEventKey)
			return errors.Join(
				uploadPutter.Cleanup(db),
				func() error {
					if pinningPutter != nil {
						return pinningPutter.Cleanup(db)
					}
					return nil
				}(),
			)
		},
	}, nil
}

// NewSession is the implementation of UploadStore.NewSession method.
func (db *DB) NewSession() (SessionInfo, error) {
	db.lock.Lock(lockKeyNewSession)
	defer db.lock.Unlock(lockKeyNewSession)

	return upload.NextTag(db.repo.IndexStore())
}

// Session is the implementation of the UploadStore.Session method.
func (db *DB) Session(tagID uint64) (SessionInfo, error) {
	return upload.TagInfo(db.repo.IndexStore(), tagID)
}

// DeleteSession is the implementation of the UploadStore.DeleteSession method.
func (db *DB) DeleteSession(tagID uint64) error {
	return upload.DeleteTag(db.repo.IndexStore(), tagID)
}

// ListSessions is the implementation of the UploadStore.ListSessions method.
func (db *DB) ListSessions(offset, limit int) ([]SessionInfo, error) {
	const maxPageSize = 1000

	limit = min(limit, maxPageSize)

	tags, err := upload.ListAllTags(db.repo.IndexStore())
	if err != nil {
		return nil, err
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i].TagID < tags[j].TagID
	})

	return tags[min(offset, len(tags)):min(offset+limit, len(tags))], nil
}

// BatchHint is the implementation of the UploadStore.BatchHint method.
func (db *DB) BatchHint(address swarm.Address) ([]byte, error) {
	return upload.BatchIDForChunk(db.repo.IndexStore(), address)
}
