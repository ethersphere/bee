// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSnapshotGetter struct {
	data []byte
}

func newMockSnapshotGetter(data []byte) mockSnapshotGetter {
	return mockSnapshotGetter{data}
}

func (m mockSnapshotGetter) GetBatchSnapshot() []byte {
	return m.data
}

func embedded(data []byte) snapshot.Source {
	return snapshot.Embedded(newMockSnapshotGetter(data))
}

// makeSnapshotData encodes logs as gzip NDJSON, the embedded snapshot format.
func makeSnapshotData(logs []types.Log) []byte {
	return gzipBytes(makeNDJSON(logs))
}

func makeNDJSON(logs []types.Log) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, l := range logs {
		_ = enc.Encode(l)
	}
	return buf.Bytes()
}

func gzipBytes(data []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(data)
	_ = gz.Close()
	return buf.Bytes()
}

// parse parses src expecting logs from the zero address, which is what logs
// built without an Address carry.
func parse(src snapshot.Source) (*snapshot.SnapshotLogFilterer, snapshot.Info, error) {
	return snapshot.Parse(log.Noop, src, common.Address{})
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("invalid gzip", func(t *testing.T) {
		t.Parallel()
		_, _, err := parse(embedded([]byte("not-gzip")))
		assert.Error(t, err)
	})

	t.Run("invalid log entry", func(t *testing.T) {
		t.Parallel()
		_, _, err := parse(embedded(gzipBytes([]byte("not-a-log-entry"))))
		assert.ErrorIs(t, err, snapshot.ErrParseSnapshot)
	})

	t.Run("non-sorted", func(t *testing.T) {
		t.Parallel()
		logs := []types.Log{
			{BlockNumber: 1, Topics: []common.Hash{}},
			{BlockNumber: 3, Topics: []common.Hash{}},
			{BlockNumber: 2, Topics: []common.Hash{}},
		}
		_, _, err := parse(embedded(makeSnapshotData(logs)))
		require.ErrorIs(t, err, snapshot.ErrParseSnapshot)
		assert.ErrorContains(t, err, "line 3")
	})

	t.Run("parse error names the line", func(t *testing.T) {
		t.Parallel()
		raw := string(makeNDJSON([]types.Log{{BlockNumber: 1, Topics: []common.Hash{}}})) + "garbage\n"
		_, _, err := parse(embedded(gzipBytes([]byte(raw))))
		require.ErrorIs(t, err, snapshot.ErrParseSnapshot)
		assert.ErrorContains(t, err, "line 2")
	})

	t.Run("blank lines are skipped", func(t *testing.T) {
		t.Parallel()
		raw := "\n" + string(makeNDJSON([]types.Log{{BlockNumber: 1, Topics: []common.Hash{}}})) + "\n\n"
		_, info, err := parse(embedded(gzipBytes([]byte(raw))))
		require.NoError(t, err)
		assert.Equal(t, 1, info.LogCount)
	})

	t.Run("contract mismatch names the line", func(t *testing.T) {
		t.Parallel()
		other := common.HexToAddress("0x1234567890123456789012345678901234567890")
		// A blank line before the foreign log puts it on file line 4, not log 3.
		raw := string(makeNDJSON([]types.Log{{BlockNumber: 1, Topics: []common.Hash{}}, {BlockNumber: 2, Topics: []common.Hash{}}})) +
			"\n" + string(makeNDJSON([]types.Log{{BlockNumber: 3, Address: other, Topics: []common.Hash{}}}))
		_, _, err := parse(embedded(gzipBytes([]byte(raw))))
		require.ErrorIs(t, err, snapshot.ErrContractMismatch)
		assert.ErrorContains(t, err, "line 4")
		assert.ErrorContains(t, err, other.Hex())
	})

	// The contract is checked while decoding, so a snapshot for another network
	// is rejected at its first line, before the rest is read.
	t.Run("contract mismatch stops at the first line", func(t *testing.T) {
		t.Parallel()
		other := common.HexToAddress("0x1234567890123456789012345678901234567890")
		raw := string(makeNDJSON([]types.Log{{BlockNumber: 1, Address: other, Topics: []common.Hash{}}})) + "garbage\n"
		_, _, err := parse(embedded(gzipBytes([]byte(raw))))
		require.ErrorIs(t, err, snapshot.ErrContractMismatch)
		assert.ErrorContains(t, err, "line 1")
	})

	t.Run("info and block number", func(t *testing.T) {
		t.Parallel()
		logs := []types.Log{
			{BlockNumber: 1, Topics: []common.Hash{}},
			{BlockNumber: 2, Topics: []common.Hash{}},
			{BlockNumber: 2, Topics: []common.Hash{}},
			{BlockNumber: 3, Topics: []common.Hash{}},
		}
		filterer, info, err := parse(embedded(makeSnapshotData(logs)))
		require.NoError(t, err)
		assert.Equal(t, snapshot.Info{LogCount: 4, MaxBlock: 3}, info)

		blockNumber, err := filterer.BlockNumber(context.Background())
		require.NoError(t, err)
		assert.Equal(t, uint64(3), blockNumber)
	})

	t.Run("filter", func(t *testing.T) {
		t.Parallel()
		logs := []types.Log{
			{BlockNumber: 1, Address: common.HexToAddress("0x1"), TxHash: common.HexToHash("0x1"), Topics: []common.Hash{common.HexToHash("0xa1")}},
			{BlockNumber: 2, Address: common.HexToAddress("0x2"), TxHash: common.HexToHash("0x2"), Topics: []common.Hash{common.HexToHash("0xa1")}},
			{BlockNumber: 3, Address: common.HexToAddress("0x3"), TxHash: common.HexToHash("0x3"), Topics: []common.Hash{common.HexToHash("0xa3")}},
			{BlockNumber: 4, Address: common.HexToAddress("0x4"), TxHash: common.HexToHash("0x4"), Topics: []common.Hash{common.HexToHash("0xa4")}},
			{BlockNumber: 5, Address: common.HexToAddress("0x4"), TxHash: common.HexToHash("0x4"), Topics: []common.Hash{common.HexToHash("0xa4"), common.HexToHash("0xa5")}},
		}
		// Logs from several contracts, to test address filtering; Parse would
		// reject them.
		filterer := snapshot.NewFilterer(logs)

		res, err := filterer.FilterLogs(context.Background(), ethereum.FilterQuery{
			FromBlock: big.NewInt(2),
			ToBlock:   big.NewInt(3),
		})
		require.NoError(t, err)
		require.Len(t, res, 2)
		assert.Equal(t, uint64(2), res[0].BlockNumber)
		assert.Equal(t, uint64(3), res[1].BlockNumber)

		res, err = filterer.FilterLogs(context.Background(), ethereum.FilterQuery{
			Addresses: []common.Address{common.HexToAddress("0x3"), common.HexToAddress("0x4")},
		})
		require.NoError(t, err)
		require.Len(t, res, 3)
		assert.Equal(t, 0, res[0].Address.Cmp(common.HexToAddress("0x3")))
		assert.Equal(t, 0, res[1].Address.Cmp(common.HexToAddress("0x4")))
		assert.Equal(t, 0, res[2].Address.Cmp(common.HexToAddress("0x4")))

		res, err = filterer.FilterLogs(context.Background(), ethereum.FilterQuery{})
		require.NoError(t, err)
		require.Len(t, res, 5)

		res, err = filterer.FilterLogs(context.Background(), ethereum.FilterQuery{
			Topics: [][]common.Hash{},
		})
		require.NoError(t, err)
		require.Len(t, res, 5)

		res, err = filterer.FilterLogs(context.Background(), ethereum.FilterQuery{
			Topics: [][]common.Hash{
				{common.HexToHash("0xa1"), common.HexToHash("0xa4"), common.HexToHash("0xa5")},
			},
		})
		require.NoError(t, err)
		require.Len(t, res, 4)
		assert.Equal(t, 0, res[0].Topics[0].Cmp(common.HexToHash("0xa1")))
		assert.Equal(t, 0, res[1].Topics[0].Cmp(common.HexToHash("0xa1")))
		assert.Equal(t, 0, res[2].Topics[0].Cmp(common.HexToHash("0xa4")))
		assert.Equal(t, 0, res[3].Topics[0].Cmp(common.HexToHash("0xa4")))
	})
}

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("embedded", func(t *testing.T) {
		t.Parallel()
		logs := []types.Log{
			{BlockNumber: 1, Topics: []common.Hash{}},
			{BlockNumber: 110, Topics: []common.Hash{}},
		}
		snap, info, err := snapshot.Load(log.Noop, embedded(makeSnapshotData(logs)), snapshot.Config{
			ABI:        abi.ABI{},
			StartBlock: 100,
		})
		require.NoError(t, err)
		require.NotNil(t, snap)
		t.Cleanup(func() { _ = snap.Listener.Close() })
		assert.Equal(t, uint64(100), snap.StartBlock)
		assert.NotNil(t, snap.Listener)
		assert.Equal(t, snapshot.Info{LogCount: 2, MaxBlock: 110}, info)
	})

	// The embedded snapshot gets the same checks as a file; a bad blob falls
	// back to chain sync instead of stalling the listener or skipping history.
	t.Run("embedded is checked too", func(t *testing.T) {
		t.Parallel()
		other := priceLog(110, 1)
		other.Address = common.HexToAddress("0x1234567890123456789012345678901234567890")
		_, _, err := snapshot.Load(log.Noop, embedded(makeSnapshotData([]types.Log{other})), snapshot.Config{
			Contract:   fileContract,
			ABI:        fileContractABI,
			StartBlock: 100,
		})
		assert.ErrorIs(t, err, snapshot.ErrContractMismatch)

		_, _, err = snapshot.Load(log.Noop, embedded(makeSnapshotData(nil)), snapshot.Config{ABI: abi.ABI{}})
		assert.ErrorIs(t, err, snapshot.ErrEmptySnapshot)
	})

	t.Run("corrupt embedded snapshot returns an error", func(t *testing.T) {
		t.Parallel()
		_, _, err := snapshot.Load(log.Noop, embedded([]byte("not-gzip")), snapshot.Config{ABI: abi.ABI{}})
		assert.Error(t, err)
	})
}

var (
	fileContract    = chaincfg.Mainnet.PostageStampAddress
	fileContractABI = abiutil.MustParseABI(chaincfg.Mainnet.PostageStampABI)
	priceTopic      = fileContractABI.Events["PriceUpdate"].ID
)

func writeSnapshotFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// priceLog is a PriceUpdate event from the file contract at the given block.
func priceLog(block uint64, price int64) types.Log {
	return types.Log{
		Address:     fileContract,
		Topics:      []common.Hash{priceTopic},
		Data:        common.LeftPadBytes(big.NewInt(price).Bytes(), 32),
		BlockNumber: block,
		TxHash:      common.BigToHash(big.NewInt(int64(block))),
	}
}

func loadFile(path string, startBlock uint64) (*batchservice.Snapshot, snapshot.Info, error) {
	return snapshot.Load(log.Noop, snapshot.File(path), snapshot.Config{
		Contract:        fileContract,
		ABI:             fileContractABI,
		StartBlock:      startBlock,
		BlockTime:       time.Second,
		StallingTimeout: time.Minute,
		BackoffTimeout:  time.Second,
	})
}

func TestLoadFile(t *testing.T) {
	t.Parallel()

	const startBlock = uint64(100)

	t.Run("valid file replays its events", func(t *testing.T) {
		t.Parallel()
		path := writeSnapshotFile(t, "snapshot.ndjson.gz", makeSnapshotData([]types.Log{
			priceLog(110, 42),
			priceLog(120, 43),
		}))

		snap, info, err := loadFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.Info{LogCount: 2, MaxBlock: 120}, info)
		assert.Equal(t, startBlock, snap.StartBlock)

		rec := &priceRecorder{}
		require.NoError(t, <-snap.Listener.Listen(context.Background(), snap.StartBlock+1, rec))
		// Close waits for the in-flight page to be processed, so the recorder is
		// complete once it returns.
		require.NoError(t, snap.Listener.Close())
		// Head 120 trims to 115, so only the block-110 event is replayed; the
		// block-120 one is left for live sync.
		assert.Equal(t, []int64{42}, rec.got())
	})

	t.Run("real batch-export line format", func(t *testing.T) {
		t.Parallel()
		// Shape of a line written by batch-export: hex quantities, lowercase
		// address, no blockHash or transactionIndex.
		line := `{"address":"0x45a1502382541cd610cc9068e88727426b696293","topics":["` + priceTopic.Hex() + `"],` +
			`"data":"0x` + common.Bytes2Hex(common.LeftPadBytes([]byte{7}, 32)) + `",` +
			`"blockNumber":"0x78","transactionHash":"0x9b1a200b3b9c757e88fe4579c87d6dd27ec781284a69163a6436ce5d29a9baaa","logIndex":"0x0"}` + "\n"
		path := writeSnapshotFile(t, "snapshot.ndjson.gz", gzipBytes([]byte(line)))

		_, info, err := loadFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.Info{LogCount: 1, MaxBlock: 120}, info)
	})

	t.Run("multi-member gzip", func(t *testing.T) {
		t.Parallel()
		// The second member starts on the block the first one ended on.
		data := append(
			makeSnapshotData([]types.Log{priceLog(105, 1), priceLog(110, 2)}),
			makeSnapshotData([]types.Log{priceLog(110, 3), priceLog(130, 4)})...,
		)
		path := writeSnapshotFile(t, "snapshot.ndjson.gz", data)

		_, info, err := loadFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.Info{LogCount: 4, MaxBlock: 130}, info)
	})

	t.Run("plain NDJSON", func(t *testing.T) {
		t.Parallel()
		path := writeSnapshotFile(t, "snapshot.ndjson", makeNDJSON([]types.Log{priceLog(110, 1), priceLog(120, 2)}))

		_, info, err := loadFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.Info{LogCount: 2, MaxBlock: 120}, info)
	})

	t.Run("plain NDJSON named .gz", func(t *testing.T) {
		t.Parallel()
		path := writeSnapshotFile(t, "snapshot.gz", makeNDJSON([]types.Log{priceLog(110, 1), priceLog(120, 2)}))

		_, info, err := loadFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.Info{LogCount: 2, MaxBlock: 120}, info)
	})

	t.Run("gzip named .ndjson", func(t *testing.T) {
		t.Parallel()
		path := writeSnapshotFile(t, "snapshot.ndjson", makeSnapshotData([]types.Log{priceLog(110, 1), priceLog(120, 2)}))

		_, info, err := loadFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.Info{LogCount: 2, MaxBlock: 120}, info)
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, _, err := loadFile(filepath.Join(t.TempDir(), "missing.gz"), startBlock)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("directory path", func(t *testing.T) {
		t.Parallel()
		_, _, err := loadFile(t.TempDir(), startBlock)
		require.ErrorIs(t, err, syscall.EISDIR)
		// Rejected before any read; see fileSource.Open.
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, "open", pathErr.Op)
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()
		_, _, err := loadFile(writeSnapshotFile(t, "snapshot.ndjson", nil), startBlock)
		assert.ErrorIs(t, err, snapshot.ErrEmptySnapshot)
	})

	t.Run("plain non-JSON", func(t *testing.T) {
		t.Parallel()
		_, _, err := loadFile(writeSnapshotFile(t, "snapshot.ndjson", []byte("not-a-log-entry\n")), startBlock)
		assert.ErrorIs(t, err, snapshot.ErrParseSnapshot)
	})

	t.Run("gzip with non-JSON lines", func(t *testing.T) {
		t.Parallel()
		_, _, err := loadFile(writeSnapshotFile(t, "snapshot.ndjson.gz", gzipBytes([]byte("not-a-log-entry\n"))), startBlock)
		assert.ErrorIs(t, err, snapshot.ErrParseSnapshot)
	})

	t.Run("truncated gzip", func(t *testing.T) {
		t.Parallel()
		data := makeSnapshotData([]types.Log{priceLog(110, 1), priceLog(120, 2)})
		_, _, err := loadFile(writeSnapshotFile(t, "snapshot.ndjson.gz", data[:len(data)/2]), startBlock)
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("gzip with no logs", func(t *testing.T) {
		t.Parallel()
		_, _, err := loadFile(writeSnapshotFile(t, "snapshot.ndjson.gz", makeSnapshotData(nil)), startBlock)
		assert.ErrorIs(t, err, snapshot.ErrEmptySnapshot)
	})

	t.Run("contract mismatch on line 3", func(t *testing.T) {
		t.Parallel()
		other := common.HexToAddress("0x1234567890123456789012345678901234567890")
		bad := priceLog(115, 3)
		bad.Address = other
		path := writeSnapshotFile(t, "snapshot.ndjson.gz", makeSnapshotData([]types.Log{priceLog(105, 1), priceLog(110, 2), bad, priceLog(120, 4)}))

		_, _, err := loadFile(path, startBlock)
		require.ErrorIs(t, err, snapshot.ErrContractMismatch)
		assert.ErrorContains(t, err, "line 3")
		assert.ErrorContains(t, err, other.Hex())
		assert.ErrorContains(t, err, fileContract.Hex())
	})

	t.Run("max block at start block", func(t *testing.T) {
		t.Parallel()
		// Head 100 trims to 95, below the first replayed block 101.
		path := writeSnapshotFile(t, "snapshot.ndjson.gz", makeSnapshotData([]types.Log{priceLog(90, 1), priceLog(startBlock, 2)}))
		_, _, err := loadFile(path, startBlock)
		assert.ErrorIs(t, err, snapshot.ErrBlockHeightTooLow)
	})

	t.Run("max block below tail", func(t *testing.T) {
		t.Parallel()
		path := writeSnapshotFile(t, "snapshot.ndjson.gz", makeSnapshotData([]types.Log{priceLog(2, 1)}))
		_, _, err := loadFile(path, 0)
		assert.ErrorIs(t, err, snapshot.ErrBlockHeightTooLow)
	})
}

// TestReplayStopsBelowMaxBlock runs the real event listener over the snapshot
// filterer and asserts the replay advances the chain state only to the trimmed
// tip — strictly below the snapshot's max block. The listener trims tailSize
// blocks off the tip (rounded to a batchFactor multiple) for reorg safety, so
// the last few blocks of the snapshot are deliberately left for live RPC sync to
// re-fetch. This is the invariant that keeps the snapshot->RPC handoff gap-free
// (#5495): live sync resumes from cs.Block+1, and cs.Block here stays below the
// tip.
func TestReplayStopsBelowMaxBlock(t *testing.T) {
	t.Parallel()

	const maxBlock = uint64(5000)

	// Logs span up to maxBlock with no topics, so FilterLogs filters them all
	// out and only the per-page UpdateBlockNumber(to) advances the chain state —
	// isolating the resume point from event processing.
	logs := []types.Log{
		{BlockNumber: 10, Topics: []common.Hash{}},
		{BlockNumber: maxBlock, Topics: []common.Hash{}},
	}
	filterer, _, err := parse(embedded(makeSnapshotData(logs)))
	require.NoError(t, err)

	// The page size Load uses, so one page covers the whole snapshot.
	l := listener.New(nil, log.Noop, filterer, common.Address{}, abi.ABI{}, time.Second, time.Minute, time.Second, snapshot.BlockPage)
	t.Cleanup(func() { _ = l.Close() })

	rec := &blockRecorder{blocks: make(chan uint64, 8)}
	if err := <-l.Listen(context.Background(), 0, rec); err != nil {
		t.Fatalf("listen: %v", err)
	}

	var got uint64
	select {
	case got = <-rec.blocks:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the replay to advance the chain state")
	}

	if got >= maxBlock {
		t.Fatalf("replay advanced chain state to %d; it must stop below the snapshot max block %d so live sync re-fetches the tail", got, maxBlock)
	}
	if maxBlock-got > 16 {
		t.Fatalf("replay stopped %d blocks below max block %d; expected within the reorg-safety trim", maxBlock-got, maxBlock)
	}
}

// noopUpdater is a postage.EventUpdater that ignores everything; recorders
// embed it and override what they record.
type noopUpdater struct{}

func (noopUpdater) Create(_, _ []byte, _, _ *big.Int, _, _ uint8, _ bool, _ common.Hash) error {
	return nil
}
func (noopUpdater) TopUp(_ []byte, _, _ *big.Int, _ common.Hash) error             { return nil }
func (noopUpdater) UpdateDepth(_ []byte, _ uint8, _ *big.Int, _ common.Hash) error { return nil }
func (noopUpdater) UpdatePrice(_ *big.Int, _ common.Hash) error                    { return nil }
func (noopUpdater) UpdateBlockNumber(_ uint64) error                               { return nil }
func (noopUpdater) Start(_ context.Context, _ uint64) error                        { return nil }
func (noopUpdater) TransactionStart() error                                        { return nil }
func (noopUpdater) TransactionEnd() error                                          { return nil }

// blockRecorder reports every block the listener commits via UpdateBlockNumber.
type blockRecorder struct {
	noopUpdater
	blocks chan uint64
}

func (r *blockRecorder) UpdateBlockNumber(blockNumber uint64) error {
	r.blocks <- blockNumber
	return nil
}

// priceRecorder records every price update.
type priceRecorder struct {
	noopUpdater
	mu     sync.Mutex
	prices []int64
}

func (r *priceRecorder) got() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.prices
}

func (r *priceRecorder) UpdatePrice(price *big.Int, _ common.Hash) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prices = append(r.prices, price.Int64())
	return nil
}
