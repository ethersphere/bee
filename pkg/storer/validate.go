// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/sharky"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

//type dirFS struct {
//	basedir string
//}

//func (d *dirFS) Open(path string) (fs.File, error) {
//	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0644)
//}

// Validate ensures that all retrievalIndex chunks are correctly stored in sharky.
func Validate(ctx context.Context, basePath string, opts *Options) error {

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



	sharky, err := sharky.New(&dirFS{basedir: path.Join(basePath, sharkyPath)},
								sharkyNoOfShards, swarm.SocMaxChunkSize)
	if err != nil {
		return err
	}
	defer func() {
		if err := sharky.Close(); err != nil {
			logger.Error(err, "failed closing sharky")
		}
	}()

	logger.Info("performing chunk validation")
	validateWork(logger, store, sharky)

	return nil
}

func validateWork(logger log.Logger, store storage.Store, sharky *sharky.Store) {

	invalidCount := 0

	n := time.Now()
	defer func() {
		logger.Info("validation finished", "duration", time.Since(n), "invalid", invalidCount)
	}()

	iteratateItemsC := make(chan *chunkstore.RetrievalIndexItem)
	
	validChunk := func(item *chunkstore.RetrievalIndexItem, buf []byte) {
		err := sharky.Read(context.Background(), item.Location, buf)
		if err != nil {
			logger.Warning("invalid chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0), "location", item.Location, "error", err)
			return
		}

		ch := swarm.NewChunk(item.Address, buf)
		err = cac.Valid(ch)
		if err != nil && !soc.Valid(ch) {

			invalidCount++
			logger.Info("invalid cac/soc chunk ", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0), "err", err)

			h, err := cac.DoHash(buf[swarm.SpanSize:], buf[:swarm.SpanSize])
			if err != nil {
				logger.Error(err, "cac hash")
				return
			}

			computedAddr := swarm.NewAddress(h)

			err = cac.Valid(swarm.NewChunk(computedAddr, buf))
			if err != nil {
				logger.Info("computed chunk is also an invalid cac", "err", err)
				return
			}

			shardedEntry := chunkstore.RetrievalIndexItem{Address: computedAddr}
			err = store.Get(&shardedEntry)
			if err != nil {
				logger.Info("no shared entry found")
				return
			}

			logger.Info("retrieved chunk with shared slot", "shared_address", shardedEntry.Address, "shared_timestamp", time.Unix(int64(shardedEntry.Timestamp), 0))
		}
	}

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, swarm.SocMaxChunkSize)
			for item := range iteratateItemsC {
				validChunk(item, buf[:item.Location.Length])
			}
		}()
	}

	count := 0
	_ = chunkstore.Iterate(store, func(item *chunkstore.RetrievalIndexItem) error {
		iteratateItemsC <- item
		count++
		if count%100_000 == 0 {
			logger.Info("..still validating chunks", "count", count)
		}
		return nil
	})

	close(iteratateItemsC)

	wg.Wait()
}
