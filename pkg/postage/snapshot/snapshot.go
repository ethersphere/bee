// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package snapshot rebuilds the postage batch store from a pre-computed snapshot
// of postage contract events instead of replaying the whole contract history
// from the chain. A snapshot is NDJSON, one types.Log per line, sorted by block
// number, optionally gzip-compressed. It comes from a Source: the blob embedded
// in the binary, or a file the operator points at.
package snapshot

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
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

// maxLineBytes bounds one NDJSON line. A postage event log is a few hundred
// bytes; the limit only stops a garbage file from being read without end.
const maxLineBytes = 4 << 20

var (
	// ErrEmptySnapshot is returned when a strict snapshot holds no logs.
	ErrEmptySnapshot = errors.New("snapshot: no logs")
	// ErrContractMismatch is returned when a strict snapshot holds a log emitted
	// by a contract other than the configured postage contract.
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

// Open streams the file. A directory is rejected up front: reading one fails
// with an OS-specific error, which on Windows does not say what is wrong.
func (s fileSource) Open() (io.ReadCloser, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, &fs.PathError{Op: "open", Path: s.path, Err: syscall.EISDIR}
	}

	buffered := bufio.NewReader(file)
	magic, err := buffered.Peek(2)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = file.Close()
		return nil, err
	}
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gzipReader, err := gzip.NewReader(buffered)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		return &fileReader{Reader: gzipReader, gzip: gzipReader, file: file}, nil
	}
	return &fileReader{Reader: buffered, file: file}, nil
}

// fileReader streams a snapshot file, decompressing it when it is gzip.
type fileReader struct {
	io.Reader
	gzip *gzip.Reader // nil for plain NDJSON
	file *os.File
}

func (r *fileReader) Close() error {
	var err error
	if r.gzip != nil {
		err = r.gzip.Close()
	}
	return errors.Join(err, r.file.Close())
}

// entry is one decoded snapshot line.
type entry struct {
	line int // 1-based
	log  types.Log
}

// decode yields the logs in r, one per NDJSON line. Blank lines are skipped. A
// line that does not decode, or a read failure, ends the sequence with an error;
// decode errors wrap listener.ErrParseSnapshot.
//
// The snapshot is produced by ethersphere/batch-export, whose default slim
// encoding carries only the types.Log fields Bee reads today: address, topics,
// data, blockNumber, transactionHash (and logIndex). Any other field —
// BlockHash, TxIndex, Removed — decodes to its zero value here with no error.
// Before consuming a new types.Log field anywhere downstream of this package
// (FilterLogs callers, listener.processEvent, transaction.ParseEvent), extend
// SlimLog in batch-export's pkg/filestore and republish the snapshot first;
// otherwise the field is silently empty for snapshot-sourced logs.
func decode(r io.Reader) iter.Seq2[entry, error] {
	return func(yield func(entry, error) bool) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
		line := 0
		for scanner.Scan() {
			line++
			raw := bytes.TrimSpace(scanner.Bytes())
			if len(raw) == 0 {
				continue
			}
			var l types.Log
			if err := json.Unmarshal(raw, &l); err != nil {
				// A stream that breaks mid-line hands the partial line here first;
				// report the read error, not the line.
				if readErr := scanner.Err(); readErr != nil {
					yield(entry{line: line}, fmt.Errorf("read snapshot: %w", readErr))
					return
				}
				yield(entry{line: line}, fmt.Errorf("%w: line %d: %w", listener.ErrParseSnapshot, line, err))
				return
			}
			if !yield(entry{line: line, log: l}, nil) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			yield(entry{line: line}, fmt.Errorf("read snapshot: %w", err))
		}
	}
}

// Info describes a parsed snapshot.
type Info struct {
	Source   string
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
// block number. When strict, the snapshot must also hold at least one log, and
// every log must come from contract: a log from another contract would be
// filtered out during replay while the chain state still advanced past it,
// silently skipping history.
func Parse(logger log.Logger, src Source, contract common.Address, strict bool) (*SnapshotLogFilterer, Info, error) {
	reader, err := src.Open()
	if err != nil {
		return nil, Info{}, err
	}
	defer reader.Close()

	var (
		logs     []types.Log
		maxBlock uint64
	)
	for e, err := range decode(reader) {
		if err != nil {
			return nil, Info{}, err
		}
		// FilterLogs binary-searches by block number.
		if e.log.BlockNumber < maxBlock {
			return nil, Info{}, fmt.Errorf("%w: line %d: block %d after block %d, snapshot is not sorted by block number",
				listener.ErrParseSnapshot, e.line, e.log.BlockNumber, maxBlock)
		}
		if strict && e.log.Address != contract {
			return nil, Info{}, fmt.Errorf("%w: line %d has address %s, expected %s",
				ErrContractMismatch, e.line, e.log.Address.Hex(), contract.Hex())
		}
		maxBlock = e.log.BlockNumber
		logs = append(logs, e.log)
	}
	if strict && len(logs) == 0 {
		return nil, Info{}, ErrEmptySnapshot
	}

	filterer := &SnapshotLogFilterer{
		logger:   logger,
		logs:     logs,
		maxBlock: maxBlock,
	}
	return filterer, Info{Source: src.Name(), LogCount: len(logs), MaxBlock: maxBlock}, nil
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
