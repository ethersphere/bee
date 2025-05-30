// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"
	"sort"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const uploadsLock = "pin-upload-store"

// Report implements the storage.PushReporter by wrapping the internal reporter
// with a transaction.
func (db *DB) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {

	unlock := db.Lock(uploadsLock)
	defer unlock()

	err := db.storage.Run(ctx, func(s transaction.Store) error {
		return upload.Report(ctx, s, chunk, state)
	})
	if err != nil {
		return fmt.Errorf("reporter.Report: %w", err)
	}

	return nil
}

// NewSession is the implementation of UploadStore.NewSession method.
func (db *DB) NewSession() (SessionInfo, error) {
	unlock := db.Lock(lockKeyNewSession)
	defer unlock()

	trx, done := db.storage.NewTransaction(context.Background())
	defer done()

	info, err := upload.NextTag(trx.IndexStore())
	if err != nil {
		return SessionInfo{}, err
	}
	return info, trx.Commit()
}

// Session is the implementation of the UploadStore.Session method.
func (db *DB) Session(tagID uint64) (SessionInfo, error) {
	return upload.TagInfo(db.storage.IndexStore(), tagID)
}

// DeleteSession is the implementation of the UploadStore.DeleteSession method.
func (db *DB) DeleteSession(tagID uint64) error {
	return db.storage.Run(context.Background(), func(s transaction.Store) error {
		return upload.DeleteTag(s.IndexStore(), tagID)
	})
}

// ListSessions is the implementation of the UploadStore.ListSessions method.
func (db *DB) ListSessions(offset, limit int) ([]SessionInfo, error) {
	const maxPageSize = 1000

	limit = min(limit, maxPageSize)

	tags, err := upload.ListAllTags(db.storage.IndexStore())
	if err != nil {
		return nil, err
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i].TagID < tags[j].TagID
	})

	return tags[min(offset, len(tags)):min(offset+limit, len(tags))], nil
}
