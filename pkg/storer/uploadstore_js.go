//go:build js
// +build js

package storer

import (
	"context"
	"errors"
	"fmt"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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
		Putter: storage.PutterFunc(func(ctx context.Context, chunk swarm.Chunk) error {
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
		done: func(address swarm.Address) error {
			defer db.events.Trigger(subscribePushEventKey)
			unlock := db.Lock(uploadsLock)
			defer unlock()

			return errors.Join(
				db.storage.Run(ctx, func(s transaction.Store) error {
					return uploadPutter.Close(s.IndexStore(), address)
				}),
				func() error {
					if pinningPutter != nil {
						return db.storage.Run(ctx, func(s transaction.Store) error {
							pinErr := pinningPutter.Close(s.IndexStore(), address)
							if errors.Is(pinErr, pinstore.ErrDuplicatePinCollection) {
								pinErr = pinningPutter.Cleanup(db.storage)
							}
							return pinErr
						})
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
