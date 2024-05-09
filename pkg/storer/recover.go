// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ethersphere/bee/v2/pkg/sharky"
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

	c := chunkstore.IterateLocations(ctx, store)

	if err := addLocations(c, sharkyRecover); err != nil {
		return closer, err
	}

	return closer, nil
}

func addLocations(locationResultC <-chan chunkstore.LocationResult, sharkyRecover *sharky.Recovery) error {
	for res := range locationResultC {
		if res.Err != nil {
			return res.Err
		}

		if err := sharkyRecover.Add(res.Location); err != nil {
			return err
		}
	}

	if err := sharkyRecover.Save(); err != nil {
		return err
	}

	return nil
}
