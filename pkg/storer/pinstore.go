// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
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
		Putter: putterWithMetrics{
			storage.PutterFunc(
				func(ctx context.Context, chunk swarm.Chunk) error {
					unlock := db.Lock(uploadsLock)
					defer unlock()
					return db.storage.Run(ctx, func(s transaction.Store) error {
						return pinningPutter.Put(ctx, s, chunk)
					})
				},
			),
			db.metrics,
			"pinstore",
		},
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
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("pinstore", "DeletePin").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("pinstore", "DeletePin", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("pinstore", "DeletePin", "failure").Inc()
		}
	}()

	unlock := db.Lock(uploadsLock)
	defer unlock()

	return pinstore.DeletePin(ctx, db.storage, root)
}

// Pins is the implementation of the PinStore.Pins method.
func (db *DB) Pins() (address []swarm.Address, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("pinstore", "Pins").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("pinstore", "Pins", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("pinstore", "Pins", "failure").Inc()
		}
	}()

	return pinstore.Pins(db.storage.IndexStore())
}

// HasPin is the implementation of the PinStore.HasPin method.
func (db *DB) HasPin(root swarm.Address) (has bool, err error) {
	dur := captureDuration(time.Now())
	defer func() {
		db.metrics.MethodCallsDuration.WithLabelValues("pinstore", "HasPin").Observe(dur())
		if err == nil {
			db.metrics.MethodCalls.WithLabelValues("pinstore", "HasPin", "success").Inc()
		} else {
			db.metrics.MethodCalls.WithLabelValues("pinstore", "HasPin", "failure").Inc()
		}
	}()

	return pinstore.HasPin(db.storage.IndexStore(), root)
}

func (db *DB) IteratePinCollection(root swarm.Address, iterateFn func(swarm.Address) (bool, error)) error {
	return pinstore.IterateCollection(db.storage.IndexStore(), root, iterateFn)
}
