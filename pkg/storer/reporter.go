// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Report implements the storage.PushReporter by wrapping the internal reporter
// with a transaction.
func (db *DB) Report(ctx context.Context, chunk swarm.Chunk, state storage.ChunkState) error {
	// only allow one report per tag
	db.lock.Lock(fmt.Sprintf("%d", chunk.TagID()))
	defer db.lock.Unlock(fmt.Sprintf("%d", chunk.TagID()))

	txnRepo, commit, rollback := db.repo.NewTx(ctx)
	reporter := upload.NewPushReporter(txnRepo)

	err := reporter.Report(ctx, chunk, state)
	if err != nil {
		return fmt.Errorf("reporter.Report: %w", errors.Join(err, rollback()))
	}

	return commit()
}
