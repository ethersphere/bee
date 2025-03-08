// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

func addrKey(ch swarm.Address) string { return fmt.Sprintf("upload-chunk-%s", ch) }
func sessionKey(tag uint64) string    { return fmt.Sprintf("upload-session-%d", tag) }

const lockKeyNewSession string = "new_session"
const reportedEvent = "reportEnd"

// Report implements the storage.PushReporter by wrapping the internal reporter
// with a transaction.
func (db *DB) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {

	db.tagCache.Lock()
	defer db.tagCache.Unlock()

	update, ok := db.tagCache.updates[chunk.TagID()]
	if !ok {
		update = &upload.TagUpdate{}
		db.tagCache.updates[chunk.TagID()] = update
	}

	switch state {
	case storage.ChunkSent:
		update.Sent++
	case storage.ChunkStored:
		update.Stored++
		fallthrough
	case storage.ChunkSynced:
		update.Synced++
		db.tagCache.synced = append(db.tagCache.synced, chunkBatch{chunk.Address(), chunk.Stamp().BatchID()})
	}

	select {
	case db.tagCache.wakeup <- struct{}{}:
	default:
	}

	return nil
}

func (db *DB) reportWorker(ctx context.Context) {

	defer db.inFlight.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-db.quit:
			return
		case <-db.tagCache.wakeup:

			db.tagCache.Lock()
			for tag, t := range db.tagCache.updates {

				unlock := db.Lock(sessionKey(tag))

				if err := db.storage.Run(ctx, func(s transaction.Store) error {
					return upload.Report(s.IndexStore(), tag, t)
				}); err != nil {
					db.logger.Debug("report failed", "error", err)
				}

				unlock()
			}

			//reset
			db.tagCache.updates = make(map[uint64]*upload.TagUpdate)

			eg := errgroup.Group{}
			eg.SetLimit(runtime.NumCPU())

			synced := db.tagCache.synced[:]

			db.tagCache.Unlock()

			for _, s := range synced {
				eg.Go(func() error {
					unlock := db.Lock(addrKey(s.address))
					defer unlock()

					return db.storage.Run(ctx, func(st transaction.Store) error {
						return upload.Synced(st, s.address, s.batchID)
					})
				})
			}

			db.tagCache.Lock()
			db.tagCache.synced = db.tagCache.synced[len(synced):]
			db.tagCache.Unlock()

			if err := eg.Wait(); err != nil {
				db.logger.Debug("sync cleanup failed", "error", err)
			}

			db.events.Trigger(reportedEvent)

			// slowdown
			select {
			case <-ctx.Done():
				return
			case <-db.quit:
				return
			case <-time.After(time.Second * 5):
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
		pinningPutter internal.PutterCloserCleanerWithReference
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
				unlock := db.Lock(addrKey(chunk.Address())) // protect against multiple upload of same chunk
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
			unlock := db.Lock(sessionKey(tagID))
			defer unlock()

			return errors.Join(
				db.storage.Run(ctx, func(s transaction.Store) error {
					return uploadPutter.Close(s.IndexStore(), address)
				}),
				func() error {
					if pinningPutter != nil {
						pinErr := db.storage.Run(ctx, func(s transaction.Store) error {
							return pinningPutter.Close(s.IndexStore(), address)
						})
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
			unlock := db.Lock(sessionKey(tagID))
			defer unlock()

			return errors.Join(
				upload.Cleanup(db.storage, tagID),
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
	unlock := db.Lock(sessionKey(tagID))
	defer unlock()
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
