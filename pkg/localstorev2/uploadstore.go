// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	storage "github.com/ethersphere/bee/pkg/storagev2"
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
		pinningPutter = pinstore.NewCollection(txnRepo)
	}

	db.markDirty(tagID)

	return &putterSession{
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
			defer db.clearDirty(tagID)

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
