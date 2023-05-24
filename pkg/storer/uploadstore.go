// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"
	"sort"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// Upload is the implementation of UploadStore.Upload method.
func (db *DB) Upload(ctx context.Context, pin bool, tagID uint64) (PutterSession, error) {
	if tagID == 0 {
		return nil, fmt.Errorf("storer: tagID required")
	}

	txnRepo, commit, rollback := db.repo.NewTx(ctx)

	uploadPutter, err := upload.NewPutter(txnRepo, tagID)
	if err != nil {
		return nil, err
	}

	var pinningPutter internal.PutterCloserWithReference
	if pin {
		pinningPutter = pinstore.NewCollection(txnRepo, nil)
	}

	db.markDirty(tagID)
	tagCloser := func() {
		db.clearDirty(tagID)
		db.events.Trigger(subscribePushEventKey)
	}

	return &putterSession{
		Putter: putterWithMetrics{
			storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
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
			db.metrics,
			"uploadstore",
		},
		done: func(address swarm.Address) error {
			defer tagCloser()
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
			defer tagCloser()
			return rollback()
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

func (db *DB) DeleteSession(tagID uint64) error {
	return upload.DeleteTag(db.repo.IndexStore(), tagID)
}

func (db *DB) ListSessions(offset, limit int) ([]SessionInfo, error) {
	const maxPageSize = 1000

	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

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
