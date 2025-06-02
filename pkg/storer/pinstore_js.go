//go:build js
// +build js

package storer

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// NewCollection is the implementation of the PinStore.NewCollection method.
func (db *DB) NewCollection(ctx context.Context) (PutterSession, error) {
	var (
		pinningPutter internal.PutterCloserWithReference
		err           error
	)
	err = db.storage.Run(ctx, func(store transaction.Store) error {
		pinningPutter, err = pinstore.NewCollection(store.IndexStore())
		if err != nil {
			return fmt.Errorf("pinstore.NewCollection: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &putterSession{
		Putter: storage.PutterFunc(
			func(ctx context.Context, chunk swarm.Chunk) error {
				unlock := db.Lock(uploadsLock)
				defer unlock()
				return db.storage.Run(ctx, func(s transaction.Store) error {
					return pinningPutter.Put(ctx, s, chunk)
				})
			},
		),

		done: func(address swarm.Address) error {
			unlock := db.Lock(uploadsLock)
			defer unlock()
			return db.storage.Run(ctx, func(s transaction.Store) error {
				return pinningPutter.Close(s.IndexStore(), address)
			})
		},
		cleanup: func() error {
			unlock := db.Lock(uploadsLock)
			defer unlock()
			return pinningPutter.Cleanup(db.storage)
		},
	}, nil
}

// DeletePin is the implementation of the PinStore.DeletePin method.
func (db *DB) DeletePin(ctx context.Context, root swarm.Address) (err error) {

	unlock := db.Lock(uploadsLock)
	defer unlock()

	return pinstore.DeletePin(ctx, db.storage, root)
}

// Pins is the implementation of the PinStore.Pins method.
func (db *DB) Pins() (address []swarm.Address, err error) {

	return pinstore.Pins(db.storage.IndexStore())
}

// HasPin is the implementation of the PinStore.HasPin method.
func (db *DB) HasPin(root swarm.Address) (has bool, err error) {

	return pinstore.HasPin(db.storage.IndexStore(), root)
}
