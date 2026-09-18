// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"golang.org/x/sync/errgroup"
)

const (
	// backfillFlushSize is the number of entries held in memory before they are
	// written back. The reserve holds millions of bin items, so collecting them
	// all the way ReserveRepairer does would cost hundreds of megabytes on a
	// node that is still starting up.
	backfillFlushSize = 10_000
	// backfillTxSize is how many entries share one transaction inside a window.
	backfillTxSize = 500
)

// BackfillChunkBinItemLocation fills in ChunkBinItem.Location for entries that
// were written before the field existed.
//
// The sampler reads chunk data straight from sharky when a bin item carries a
// location, and falls back to a retrieval-index lookup when it does not. Bin
// items only gain a location when they are (re)written, so without this step an
// upgraded node keeps paying for the lookup on every chunk already in its
// reserve, and the hint is worthless until the reserve turns over on its own.
//
// Rewriting a bin item changes only the value: the key is (Bin, BinID), so the
// iteration this runs inside is unaffected and the entries can be flushed in
// windows rather than accumulated.
//
// The step is idempotent. Entries that already carry a location are skipped, and
// so are entries whose chunk is no longer in the chunkstore — those keep a zero
// location and the sampler keeps falling back for them, which is correct.
func BackfillChunkBinItemLocation(st transaction.Storage, logger log.Logger) func() error {
	return func() error {
		start := time.Now()

		var (
			seen    int
			filled  atomic.Int64
			missing atomic.Int64
			window  []*reserve.ChunkBinItem
		)

		// Every entry costs a random read of the retrieval index, which is the
		// slow part: single file order here would leave the disk idle most of
		// the time, so each window is spread over the same number of workers
		// ReserveRepairer uses.
		flush := func() error {
			if len(window) == 0 {
				return nil
			}

			var eg errgroup.Group
			eg.SetLimit(runtime.NumCPU())

			for i := 0; i < len(window); i += backfillTxSize {
				batch := window[i:min(i+backfillTxSize, len(window))]
				eg.Go(func() error {
					return st.Run(context.Background(), func(s transaction.Store) error {
						for _, item := range batch {
							rIdx := &chunkstore.RetrievalIndexItem{Address: item.Address}
							if err := s.IndexStore().Get(rIdx); err != nil {
								if errors.Is(err, storage.ErrNotFound) {
									missing.Add(1)
									continue
								}
								return err
							}

							item.Location = chunkstore.LocationToChunkLocation(rIdx.Location)
							if err := s.IndexStore().Put(item); err != nil {
								return err
							}
							filled.Add(1)
						}
						return nil
					})
				})
			}

			err := eg.Wait()
			window = window[:0]
			return err
		}

		err := st.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return new(reserve.ChunkBinItem) },
			},
			func(res storage.Result) (bool, error) {
				item := res.Entry.(*reserve.ChunkBinItem)
				seen++
				if !item.Location.IsZero() {
					return false, nil
				}

				window = append(window, item)
				if len(window) < backfillFlushSize {
					return false, nil
				}

				if err := flush(); err != nil {
					return true, err
				}
				logger.Info("backfilling chunk bin item locations", "seen", seen, "filled", filled.Load())
				return false, nil
			},
		)
		if err != nil {
			return err
		}

		if err := flush(); err != nil {
			return err
		}

		logger.Info(
			"chunk bin item locations backfilled",
			"seen", seen,
			"filled", filled.Load(),
			"missing_chunks", missing.Load(),
			"duration", time.Since(start),
		)

		return nil
	}
}
