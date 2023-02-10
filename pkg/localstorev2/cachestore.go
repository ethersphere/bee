// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// Lookup is the implementation of the CacheStore.Lookup method.
func (db *DB) Lookup() storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
		txnRepo, commit, rollback := db.repo.NewTx(ctx)
		ch, err := db.cacheObj.Getter(txnRepo).Get(ctx, address)
		switch {
		case err == nil:
			return ch, commit()
		case errors.Is(err, storage.ErrNotFound):
			// here we would ideally have nothing to do but just to return this
			// error to the client. The commit is mainly done to end the txn.
			return nil, multierror.Append(err, commit()).ErrorOrNil()
		}
		// if we are here, it means there was some unexpected error, in which
		// case we need to rollback any changes that were already made.
		return nil, multierror.Append(err, rollback()).ErrorOrNil()
	})
}

// Cache is the implementation of the CacheStore.Cache method.
func (db *DB) Cache() storage.Putter {
	return storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
		txnRepo, commit, rollback := db.repo.NewTx(ctx)
		err := db.cacheObj.Putter(txnRepo).Put(ctx, ch)
		if err != nil {
			return multierror.Append(err, rollback()).ErrorOrNil()
		}
		return multierror.Append(err, commit()).ErrorOrNil()
	})
}
