// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"

	"slices"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	archive "github.com/ethersphere/batch-archive"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
)

var _ listener.BlockHeightContractFilterer = (*SnapshotLogFilterer)(nil)

type SnapshotGetter interface {
	GetBatchSnapshot() []byte
}

type archiveSnapshotGetter struct{}

func (a archiveSnapshotGetter) GetBatchSnapshot() []byte {
	return archive.GetBatchSnapshot()
}

type SnapshotLogFilterer struct {
	logger         log.Logger
	loadedLogs     []types.Log
	maxBlockHeight uint64
	initOnce       sync.Once
	getter         SnapshotGetter
}

func NewSnapshotLogFilterer(logger log.Logger, getter SnapshotGetter) *SnapshotLogFilterer {
	return &SnapshotLogFilterer{
		logger: logger,
		getter: getter,
	}
}

func (f *SnapshotLogFilterer) GetBatchSnapshot() []byte {
	return f.getter.GetBatchSnapshot()
}

// loadSnapshot is responsible for loading and processing the snapshot data.
// It is intended to be called exactly once by initOnce.Do.
func (f *SnapshotLogFilterer) loadSnapshot() error {
	f.logger.Info("loading batch snapshot")
	data := f.getter.GetBatchSnapshot()
	dataReader := bytes.NewReader(data)
	gzipReader, err := gzip.NewReader(dataReader)
	if err != nil {
		f.logger.Error(err, "failed to create gzip reader for batch import")
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	if err := f.parseLogs(gzipReader); err != nil {
		f.logger.Error(err, "failed to parse logs from snapshot")
		return err
	}

	f.logger.Info("batch snapshot loaded successfully", "log_count", len(f.loadedLogs), "max_block_height", f.maxBlockHeight)
	return nil
}

func (f *SnapshotLogFilterer) parseLogs(reader io.Reader) error {
	var parsedLogs []types.Log
	var currentMaxBlockHeight uint64

	decoder := json.NewDecoder(reader)
	for {
		var logEntry types.Log
		if err := decoder.Decode(&logEntry); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("%w: failed to decode log event at position %d: %w", listener.ErrParseSnapshot, len(parsedLogs), err)
		}

		// Validate sorting order (required for binary search in FilterLogs)
		if logEntry.BlockNumber < currentMaxBlockHeight {
			return fmt.Errorf("%w: snapshot data is not sorted by block number at index %d (block %d < %d)",
				listener.ErrParseSnapshot, len(parsedLogs), logEntry.BlockNumber, currentMaxBlockHeight)
		}

		if logEntry.BlockNumber > currentMaxBlockHeight {
			currentMaxBlockHeight = logEntry.BlockNumber
		}
		parsedLogs = append(parsedLogs, logEntry)
	}

	f.loadedLogs = parsedLogs
	f.maxBlockHeight = currentMaxBlockHeight
	return nil
}

// ensureLoaded calls loadSnapshot via sync.Once to ensure thread-safe, one-time initialization.
func (f *SnapshotLogFilterer) ensureLoaded() error {
	var err error
	f.initOnce.Do(func() {
		err = f.loadSnapshot()
	})
	return err
}

func (f *SnapshotLogFilterer) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	if err := f.ensureLoaded(); err != nil {
		return nil, fmt.Errorf("failed to ensure snapshot was loaded for FilterLogs: %w", err)
	}

	f.logger.Debug("filtering pre-loaded logs", "total_logs", len(f.loadedLogs), "query_from_block", query.FromBlock, "query_to_block", query.ToBlock, "query_addresses_count", len(query.Addresses), "query_topics_count", len(query.Topics))

	filtered := make([]types.Log, 0)

	startIndex := 0
	if query.FromBlock != nil {
		fromBlockNum := query.FromBlock.Uint64()
		startIndex = sort.Search(len(f.loadedLogs), func(i int) bool {
			return f.loadedLogs[i].BlockNumber >= fromBlockNum
		})
	}

	scannedCount := 0
	for i := startIndex; i < len(f.loadedLogs); i++ {
		logEntry := f.loadedLogs[i]
		scannedCount++

		if query.ToBlock != nil && logEntry.BlockNumber > query.ToBlock.Uint64() {
			break
		}

		if len(query.Addresses) > 0 && !slices.Contains(query.Addresses, logEntry.Address) {
			continue
		}

		if len(query.Topics) > 0 {
			match := true
			for topicIndex, topicCriteria := range query.Topics {
				if len(topicCriteria) == 0 {
					continue
				}
				if topicIndex >= len(logEntry.Topics) {
					match = false
					break
				}

				if !slices.Contains(topicCriteria, logEntry.Topics[topicIndex]) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		filtered = append(filtered, logEntry)
	}

	f.logger.Debug("filtered logs complete", "input_log_count", len(f.loadedLogs), "potential_logs_in_block_range", scannedCount, "output_count", len(filtered))
	return filtered, nil
}

func (f *SnapshotLogFilterer) BlockNumber(_ context.Context) (uint64, error) {
	if err := f.ensureLoaded(); err != nil {
		return 0, fmt.Errorf("failed to ensure snapshot was loaded for BlockNumber: %w", err)
	}
	return f.maxBlockHeight, nil
}
