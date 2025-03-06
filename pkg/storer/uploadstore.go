// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const uploadsLock = "pin-upload-store"

const reportEvent = "report"

// Report implements the storage.PushReporter by wrapping the internal reporter
// with a transaction.
func (db *DB) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {

	db.tagCache.Lock()
	defer db.tagCache.Unlock()

	update, ok := db.tagCache.tags[chunk.TagID()]
	if !ok {
		update = &upload.TagUpdate{}
		db.tagCache.tags[chunk.TagID()] = update
	}

	switch state {
	case storage.ChunkSent:
		update.Sent++
	case storage.ChunkSynced:
		update.Synced++
	case storage.ChunkStored:
		update.Synced++
		update.Stored++
	}

	db.events.Trigger(reportEvent)

	return nil
}

func (db *DB) reporter(ctx context.Context) {

	defer db.inFlight.Done()

	reportEvent, unsubscribe := db.events.Subscribe(reportEvent)
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reportEvent:
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 5):
				unlock := db.Lock(uploadsLock)
				db.tagCache.Lock()
				for id, t := range db.tagCache.tags {
					if err := db.storage.Run(ctx, func(s transaction.Store) error {
						return upload.Report(s, id, t)
					}); err != nil {
						db.logger.Debug("report failed", "error", err)
					}
				}
				db.tagCache.tags = make(map[uint64]*upload.TagUpdate) // reset
				db.tagCache.Unlock()
				unlock()
			}
		}
	}
}

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

	err = db.storage.Run(ctx, func(s transaction.Store) error {
		uploadPutter, err = upload.NewPutter(s.IndexStore(), tagID)
		if err != nil {
			return fmt.Errorf("upload.NewPutter: %w", err)
		}

		if pin {
			pinningPutter, err = pinstore.NewCollection(s.IndexStore())
			if err != nil {
				return fmt.Errorf("pinstore.NewCollection: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &putterSession{
		Putter: putterWithMetrics{
			storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
				unlock := db.Lock(uploadsLock)
				defer unlock()
				return errors.Join(
					db.storage.Run(ctx, func(s transaction.Store) error {
						return uploadPutter.Put(ctx, s, chunk)
					}),
					func() error {
						if pinningPutter != nil {
							return db.storage.Run(ctx, func(s transaction.Store) error {
								return pinningPutter.Put(ctx, s, chunk)
							})
						}
						return nil
					}(),
				)
			}),
			db.metrics,
			"uploadstore",
		},
		done: func(address swarm.Address) error {
			defer db.events.Trigger(subscribePushEventKey)
			unlock := db.Lock(uploadsLock)
			defer unlock()

			return errors.Join(
				uploadPutter.Close(db.storage, address),
				func() error {
					if pinningPutter != nil {
						pinErr := pinningPutter.Close(db.storage, address)
						if errors.Is(pinErr, pinstore.ErrDuplicatePinCollection) {
							pinErr = pinningPutter.Cleanup(db.storage)
						}
						return pinErr
					}
					return nil
				}(),
			)
		},
		cleanup: func() error {
			defer db.events.Trigger(subscribePushEventKey)
			unlock := db.Lock(uploadsLock)
			defer unlock()

			return errors.Join(
				uploadPutter.Cleanup(db.storage),
				func() error {
					if pinningPutter != nil {
						return pinningPutter.Cleanup(db.storage)
					}
					return nil
				}(),
			)
		},
	}, nil
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
