package node

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	archive "github.com/ethersphere/batch-archive"
	"github.com/ethersphere/bee/v2/pkg/log"
)

type SnapshotBlockHeightContractFilterer struct {
	logger         log.Logger
	loadedLogs     []types.Log
	maxBlockHeight uint64
	isLoaded       bool
}

func NewSnapshotBlockHeightContractFilterer(logger log.Logger) (*SnapshotBlockHeightContractFilterer, error) {
	f := &SnapshotBlockHeightContractFilterer{
		logger: logger,
	}

	if err := f.loadAndProcessSnapshot(); err != nil {
		return nil, fmt.Errorf("failed to load and process snapshot during initialization: %w", err)
	}

	return f, nil
}

func (f *SnapshotBlockHeightContractFilterer) loadAndProcessSnapshot() error {
	f.logger.Info("loading batch snapshot during construction")
	data := archive.GetBatchSnapshot(true)

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

	f.isLoaded = true
	f.logger.Info("batch snapshot loaded successfully during construction", "log_count", len(f.loadedLogs), "max_block_height", f.maxBlockHeight)
	return nil
}

func (f *SnapshotBlockHeightContractFilterer) parseLogs(reader io.Reader) error {
	var parsedLogs []types.Log
	var currentMaxBlockHeight uint64
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var logEntry types.Log
		if err := json.Unmarshal(line, &logEntry); err != nil {
			f.logger.Warning("failed to unmarshal log event, skipping line", "error", err, "line_snippet", string(line[:min(len(line), 100)]))
			continue
		}

		if logEntry.BlockNumber > currentMaxBlockHeight {
			currentMaxBlockHeight = logEntry.BlockNumber
		}
		parsedLogs = append(parsedLogs, logEntry)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning batch import data: %w", err)
	}

	f.loadedLogs = parsedLogs
	f.maxBlockHeight = currentMaxBlockHeight
	f.isLoaded = true
	return nil
}

func (f *SnapshotBlockHeightContractFilterer) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	f.logger.Debug("filtering pre-loaded logs", "total_logs", len(f.loadedLogs), "query", query)
	
	var filtered []types.Log
	
	for _, log := range f.loadedLogs {
		// Filter by block range
		if query.FromBlock != nil && log.BlockNumber < query.FromBlock.Uint64() {
			continue
		}
		if query.ToBlock != nil && log.BlockNumber > query.ToBlock.Uint64() {
			continue
		}
		
		// Filter by addresses
		if len(query.Addresses) > 0 {
			addressMatch := false
			for _, addr := range query.Addresses {
				if log.Address == addr {
					addressMatch = true
					break
				}
			}
			if !addressMatch {
				continue
			}
		}
		
		// Filter by topics
		if len(query.Topics) > 0 {
			topicMatch := true
			
			// We have a max of 4 topics in Ethereum logs
			for i := 0; i < len(query.Topics) && i < 4; i++ {
				// Skip if no filter for this topic position or empty filter array
				if i >= len(query.Topics) || len(query.Topics[i]) == 0 {
					continue
				}
				
				// If we're filtering for this topic position but log doesn't have enough topics
				if i >= len(log.Topics) {
					topicMatch = false
					break
				}
				
				// Check if this topic matches any in the filter array for this position
				hasMatch := false
				for _, topic := range query.Topics[i] {
					if log.Topics[i] == topic {
						hasMatch = true
						break
					}
				}
				
				if !hasMatch {
					topicMatch = false
					break
				}
			}
			
			if !topicMatch {
				continue
			}
		}
		
		// If we got here, log passed all filters
		filtered = append(filtered, log)
	}
	
	f.logger.Debug("filtered logs", "input_count", len(f.loadedLogs), "output_count", len(filtered))
	return filtered, nil
}

func (f *SnapshotBlockHeightContractFilterer) BlockNumber(_ context.Context) (uint64, error) {
	return f.maxBlockHeight, nil
}
