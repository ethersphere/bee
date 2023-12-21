// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

// FixSharky removes all invalid RetrievalIndexItems which are not properly read from sharky.
func FixSharky(ctx context.Context, basePath string, opts *Options, repair, validate bool) error {
	logger := opts.Logger

	store, err := initStore(basePath, opts)
	if err != nil {
		return fmt.Errorf("failed creating levelDB index store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error(err, "failed closing store")
		}
	}()

	sharkyRecover, err := sharky.NewRecovery(path.Join(basePath, sharkyPath), sharkyNoOfShards, swarm.SocMaxChunkSize, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := sharkyRecover.Close(); err != nil {
			logger.Error(err, "failed closing sharky recovery")
		}
	}()

	logger.Info("locating invalid chunks")
	invalids := validateWork(logger, store, sharkyRecover.Read)

	if len(invalids) > 0 && repair {
		logger.Info("removing invalid chunks", "count", len(invalids))

		n := time.Now()

		batch, err := store.Batch(ctx)
		if err != nil {
			return err
		}

		for _, invalid := range invalids {

			item := &chunkstore.RetrievalIndexItem{Address: *invalid}

			if err := batch.Delete(item); err != nil {
				return fmt.Errorf("store delete: %w", err)
			}
			
			logger.Info("removing", "reference", *invalid)
		}

		if err := batch.Commit(); err != nil {
			return err
		}

		logger.Info("removal finished", "duration", time.Since(n))

		if validate {
			logger.Info("performing chunk re-validation after removal")
			_ = validateWork(logger, store, sharkyRecover.Read)
		}
	}

	return nil
}
