// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
//	"errors"
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
	_ = validateWork(logger, store, sharky.Read)

	return nil
}

func validateWork(logger log.Logger, store storage.Store, readFn func(context.Context, sharky.Location, []byte) error) []*swarm.Address {

	total := 0
	socCount := 0
	invalidCount := 0
	var (
		muChunks = &sync.Mutex{}
		chunks  = make([]*swarm.Address, 0)
	)

	n := time.Now()
	defer func() {
		logger.Info("validation finished", "duration", time.Since(n), "invalid", invalidCount, "soc", socCount, "total", total)
	}()

	iteratateItemsC := make(chan *chunkstore.RetrievalIndexItem)

	validChunk := func(item *chunkstore.RetrievalIndexItem, buf []byte) (bool, error) {
		err := readFn(context.Background(), item.Location, buf)
		if err != nil {
			logger.Warning("invalid chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0), "location", item.Location, "error", err)
			return false, err
		}

		ch := swarm.NewChunk(item.Address, buf)
		err = cac.Valid(ch)
		if err != nil {
			if soc.Valid(ch) {
				socCount++
				logger.Debug("found soc chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0))
				return true, nil
			} else {
				invalidCount++
				logger.Warning("invalid cac/soc chunk", "address", item.Address, "timestamp", time.Unix(int64(item.Timestamp), 0), "err", err)

				h, err := cac.DoHash(buf[swarm.SpanSize:], buf[:swarm.SpanSize])
				if err != nil {
					logger.Error(err, "cac hash")
					return false, err
				}

				computedAddr := swarm.NewAddress(h)

				err = cac.Valid(swarm.NewChunk(computedAddr, buf))
				if err != nil {
					logger.Warning("computed chunk is also an invalid cac", "err", err)
					return false, err
				}

				sharedEntry := chunkstore.RetrievalIndexItem{Address: computedAddr}
				err = store.Get(&sharedEntry)
				if err != nil {
					logger.Warning("no shared entry found")
					return false, nil
				}

				logger.Warning("retrieved chunk with shared slot", "shared_address", sharedEntry.Address, "shared_timestamp", time.Unix(int64(sharedEntry.Timestamp), 0))
				return false, nil
			}
		} else {
			return true, nil
		}
	}

	s := time.Now()

	_ = chunkstore.Iterate(store, func(item *chunkstore.RetrievalIndexItem) error {
		total++
		if total%10_000_000 == 0 {
			logger.Info("..still counting chunks", "total", total)
		}
		return nil
	})
	logger.Info("validation count finished", "duration", time.Since(s), "total", total)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, swarm.SocMaxChunkSize)
			for item := range iteratateItemsC {
				valid, err := validChunk(item, buf[:item.Location.Length])
				if !valid && err == nil {
					muChunks.Lock()
					chunks = append(chunks, &item.Address)
					muChunks.Unlock()
				}
			}
		}()
	}

	count := 0
	_ = chunkstore.Iterate(store, func(item *chunkstore.RetrievalIndexItem) error {
		iteratateItemsC <- item
		count++
		if count%100_000 == 0 {
			logger.Info("..still validating chunks", "count", count, "invalid", invalidCount, "soc", socCount, "total", total, "percent", fmt.Sprintf("%.2f", (float64(count)*100.0)/float64(total)))
		}
		// ToDo: Hack for testing!
		//if len(chunks) > 0 {
		//	logger.Warning("HACK: found an invalid", "count", len(chunks))
		//	return errors.New("found (at least) one invalid")
		//}
		return nil
	})

	close(iteratateItemsC)

	wg.Wait()
	
	return chunks
}
