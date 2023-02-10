// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"

	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// NewCollection is the implementation of the PinStore.NewCollection method.
func (db *DB) NewCollection(ctx context.Context) (PutterSession, error) {
	txnRepo, commit, rollback := db.repo.NewTx(ctx)
	pinningPutter := pinstore.NewCollection(txnRepo)

	return &putterSession{
		Putter: pinningPutter,
		done: func(address swarm.Address) error {
			return multierror.Append(
				pinningPutter.Close(address),
				commit(),
			).ErrorOrNil()
		},
		cleanup: func() error { return rollback() },
	}, nil
}

// DeletePin is the implementation of the PinStore.DeletePin method.
func (db *DB) DeletePin(ctx context.Context, root swarm.Address) error {
	txnRepo, commit, rollback := db.repo.NewTx(ctx)

	err := pinstore.DeletePin(ctx, txnRepo, root)
	if err != nil {
		return multierror.Append(err, rollback()).ErrorOrNil()
	}

	return commit()
}

// Pins is the implementation of the PinStore.Pins method.
func (db *DB) Pins() ([]swarm.Address, error) {
	return pinstore.Pins(db.repo.IndexStore())
}

// HasPin is the implementation of the PinStore.HasPin method.
func (db *DB) HasPin(root swarm.Address) (bool, error) {
	return pinstore.HasPin(db.repo.IndexStore(), root)
}
