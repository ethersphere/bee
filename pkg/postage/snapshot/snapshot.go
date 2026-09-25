// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package snapshot rebuilds the postage batch store from a pre-computed snapshot
// of postage contract events instead of replaying the whole contract history
// from the chain. A snapshot is NDJSON, one types.Log per line, sorted by block
// number, optionally gzip-compressed.
package snapshot

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"sort"
	"syscall"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
)

// blockPage is the number of blocks per FilterLogs call during replay; the
// snapshot is served from memory, so pages can be large.
const blockPage = uint64(50000)

var (
	// ErrParseSnapshot is returned when a snapshot does not decode as sorted
	// NDJSON of logs.
	ErrParseSnapshot = errors.New("failed to parse snapshot data")
	// ErrEmptySnapshot is returned when a strict snapshot holds no logs.
	ErrEmptySnapshot = errors.New("snapshot: no logs")
	// ErrContractMismatch is returned when a strict snapshot holds a log from a
	// contract other than the configured postage contract.
	ErrContractMismatch = errors.New("snapshot: log from unexpected contract")
	// ErrBlockHeightTooLow is returned when a strict snapshot does not reach far
	// enough past the start block for the replay to make progress.
	ErrBlockHeightTooLow = errors.New("snapshot: max block not ahead of start block")
)

// SnapshotGetter provides the snapshot blob embedded in the binary.
type SnapshotGetter interface {
	GetBatchSnapshot() []byte
}

// Source is where a snapshot is read from.
type Source interface {
	// Name identifies the kind of source in logs: "embedded" or "file".
	Name() string
	// Open returns the snapshot as a stream of NDJSON, already decompressed.
	Open() (io.ReadCloser, error)
}

// Embedded returns the source backed by the gzip blob embedded in the binary.
func Embedded(getter SnapshotGetter) Source {
	return embeddedSource{getter: getter}
}

type embeddedSource struct {
	getter SnapshotGetter
}

func (embeddedSource) Name() string { return "embedded" }

func (s embeddedSource) Open() (io.ReadCloser, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(s.getter.GetBatchSnapshot()))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	return gzipReader, nil
}

// File returns the source backed by the snapshot file at path. The content
// decides the format, not the file name: gzip (possibly multi-member) is
// recognized by its magic bytes, anything else is read as plain NDJSON.
func File(path string) Source {
	return fileSource{path: path}
}

type fileSource struct {
	path string
}

func (fileSource) Name() string { return "file" }

func (s fileSource) Open() (io.ReadCloser, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	reader, err := snapshotReader(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return readCloser{reader, file}, nil
}

// snapshotReader returns a reader that yields the file's content as plain
// NDJSON.
func snapshotReader(file *os.File) (io.Reader, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	// Reading a directory fails with an OS-specific error that on Windows does
	// not say what is wrong, so reject it up front.
	if info.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: file.Name(), Err: syscall.EISDIR}
	}

	buffered := bufio.NewReader(file)
	magic, err := buffered.Peek(2)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gzipReader, err := gzip.NewReader(buffered)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		return gzipReader, nil
	}
	return buffered, nil
}

// readCloser pairs a decoding reader with the file it reads from.
type readCloser struct {
	io.Reader
	io.Closer
}

// Info describes a parsed snapshot.
type Info struct {
	LogCount int
	MaxBlock uint64
}

// SnapshotLogFilterer serves a parsed snapshot to the postage listener as if it
// were a chain backend: BlockNumber is the snapshot's max block, and FilterLogs
// answers from the in-memory, block-sorted logs.
type SnapshotLogFilterer struct {
	logger   log.Logger
	logs     []types.Log // sorted by block number
	maxBlock uint64
}

var _ listener.BlockHeightContractFilterer = (*SnapshotLogFilterer)(nil)

// Parse reads src to the end and indexes its logs, which must be sorted by
// block number. Blank lines are skipped.
//
// The snapshot comes from ethersphere/batch-export in its slim encoding: only
// address, topics, data, blockNumber, transactionHash and logIndex are set,
// every other types.Log field decodes to its zero value without error. Extend
// SlimLog in batch-export before reading a new field from snapshot logs.
func Parse(logger log.Logger, src Source) (*SnapshotLogFilterer, Info, error) {
	reader, err := src.Open()
	if err != nil {
		return nil, Info{}, err
	}
	defer reader.Close()

	var (
		logs     []types.Log
		maxBlock uint64
		line     int
		parseErr error
	)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var l types.Log
		if err := l.UnmarshalJSON(raw); err != nil {
			parseErr = fmt.Errorf("%w: line %d: %w", ErrParseSnapshot, line, err)
			break
		}
		// FilterLogs binary-searches by block number.
		if l.BlockNumber < maxBlock {
			parseErr = fmt.Errorf("%w: line %d: block %d after block %d, snapshot is not sorted by block number", ErrParseSnapshot, line, l.BlockNumber, maxBlock)
			break
		}
		maxBlock = l.BlockNumber
		logs = append(logs, l)
	}
	// A stream that breaks mid-line hands the partial line to the loop before
	// the scanner reports the read error, so the read error is checked first.
	if err := scanner.Err(); err != nil {
		return nil, Info{}, fmt.Errorf("read snapshot: %w", err)
	}
	if parseErr != nil {
		return nil, Info{}, parseErr
	}

	filterer := &SnapshotLogFilterer{logger: logger, logs: logs, maxBlock: maxBlock}
	return filterer, Info{LogCount: len(logs), MaxBlock: maxBlock}, nil
}

// checkContract reports ErrContractMismatch for the first log not emitted by
// contract. A log from another contract would be filtered out during replay
// while the chain state still advanced past it, silently skipping history.
func (f *SnapshotLogFilterer) checkContract(contract common.Address) error {
	for i, l := range f.logs {
		if l.Address != contract {
			return fmt.Errorf("%w: log %d has address %s, expected %s", ErrContractMismatch, i+1, l.Address.Hex(), contract.Hex())
		}
	}
	return nil
}

func (f *SnapshotLogFilterer) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	f.logger.Debug("filtering pre-loaded logs", "total_logs", len(f.logs), "query_from_block", query.FromBlock, "query_to_block", query.ToBlock, "query_addresses_count", len(query.Addresses), "query_topics_count", len(query.Topics))

	filtered := make([]types.Log, 0)

	startIndex := 0
	if query.FromBlock != nil {
		fromBlockNum := query.FromBlock.Uint64()
		startIndex = sort.Search(len(f.logs), func(i int) bool {
			return f.logs[i].BlockNumber >= fromBlockNum
		})
	}

	scannedCount := 0
	for i := startIndex; i < len(f.logs); i++ {
		logEntry := f.logs[i]
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

	f.logger.Debug("filtered logs complete", "input_log_count", len(f.logs), "potential_logs_in_block_range", scannedCount, "output_count", len(filtered))
	return filtered, nil
}

func (f *SnapshotLogFilterer) BlockNumber(_ context.Context) (uint64, error) {
	return f.maxBlock, nil
}
