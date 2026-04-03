// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/soc"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	sharkyDirtyFileName = ".DIRTY"
)

func sharkyRecovery(ctx context.Context, sharkyBasePath string, store storage.Store, opts *Options) (closerFn, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := opts.Logger.WithName(loggerName).Register()
	dirtyFilePath := filepath.Join(sharkyBasePath, sharkyDirtyFileName)

	closer := func() error { return os.Remove(dirtyFilePath) }

	if _, err := os.Stat(dirtyFilePath); errors.Is(err, fs.ErrNotExist) {
		return closer, os.WriteFile(dirtyFilePath, []byte{}, 0644)
	}

	logger.Info("localstore sharky .DIRTY file exists: starting recovery due to previous dirty exit")
	defer func(t time.Time) {
		logger.Info("localstore sharky recovery finished", "time", time.Since(t))
	}(time.Now())

	sharkyRecover, err := sharky.NewRecovery(sharkyBasePath, sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return closer, err
	}

	defer func() {
		if err := sharkyRecover.Close(); err != nil {
			logger.Error(err, "failed closing sharky recovery")
		}
	}()

	if err := validateAndAddLocations(ctx, store, sharkyRecover, logger); err != nil {
		return closer, err
	}

	return closer, nil
}

// validateAndAddLocations iterates every chunk index entry, reads its data from
// Sharky, and validates the content hash. Valid chunks are registered with the
// recovery so their slots are preserved. Corrupted entries (unreadable data or
// hash mismatch) are logged, excluded from the recovery bitmap, and deleted from
// the index store so the node starts clean without serving invalid data.
// If a corrupted index entry cannot be deleted, an error is returned and the
// node startup is aborted to prevent serving or operating on corrupt state.
func validateAndAddLocations(ctx context.Context, store storage.Store, sharkyRecover *sharky.Recovery, logger log.Logger) error {
	var corrupted []*chunkstore.RetrievalIndexItem

	buf := make([]byte, swarm.SocMaxChunkSize)

	err := chunkstore.IterateItems(store, func(item *chunkstore.RetrievalIndexItem) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := sharkyRecover.Read(ctx, item.Location, buf[:item.Location.Length]); err != nil {
			logger.Warning("recovery: unreadable chunk, marking corrupted", "address", item.Address, "err", err)
			corrupted = append(corrupted, item)
			return nil
		}

		ch := swarm.NewChunk(item.Address, buf[:item.Location.Length])
		if !cac.Valid(ch) && !soc.Valid(ch) {
			logger.Warning("recovery: invalid chunk hash, marking corrupted", "address", item.Address)
			corrupted = append(corrupted, item)
			return nil
		}

		return sharkyRecover.Add(item.Location)
	})
	if err != nil {
		return err
	}

	if err := sharkyRecover.Save(); err != nil {
		return err
	}

	for _, item := range corrupted {
		if err := store.Delete(item); err != nil {
			logger.Error(err, "recovery: failed deleting corrupted chunk index entry", "address", item.Address)
			return fmt.Errorf("recovery: failed deleting corrupted chunk index entry %s: %w", item.Address, err)
		}
	}

	if len(corrupted) > 0 {
		logger.Warning("recovery: removed corrupted chunk index entries", "count", len(corrupted))
	}

	return nil
}
