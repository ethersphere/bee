// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"time"

	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/swarm"
)

// NewCollection is the implementation of the PinStore.NewCollection method.
func (db *DB) NewCollection(ctx context.Context) (PutterSession, error) {
	txnRepo, commit, rollback := db.repo.NewTx(ctx)
	pinningPutter := pinstore.NewCollection(txnRepo)

	return &putterSession{
		Putter: putterWithMetrics{
			pinningPutter,
			db.metrics,
			"pinstore",
		},
		done: func(address swarm.Address) error {
			return errors.Join(
				pinningPutter.Close(address),
				commit(),
			)
		},
		cleanup: func() error {
			if err := rollback(); err != nil {
				return fmt.Errorf("pinstore.NewCollection: %w", err)
			}
			return nil
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

	txnRepo, commit, rollback := db.repo.NewTx(ctx)
	if err := pinstore.DeletePin(ctx, txnRepo, root); err != nil {
		return fmt.Errorf("pinstore.DeletePin: %w", errors.Join(err, rollback()))
	}
	return commit()
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

	return pinstore.Pins(db.repo.IndexStore())
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

	return pinstore.HasPin(db.repo.IndexStore(), root)
}

func (db *DB) IteratePinCollection(root swarm.Address, iterateFn func(swarm.Address) (bool, error)) error {
	return pinstore.IterateCollection(db.repo.IndexStore(), root, iterateFn)
}
