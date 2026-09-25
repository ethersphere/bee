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

func makeSnapshotData(logs []types.Log) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := json.NewEncoder(gz)
	for _, l := range logs {
		_ = enc.Encode(l)
	}
	gz.Close()
	return buf.Bytes()
}

func TestNewSnapshotLogFilterer(t *testing.T) {
	t.Parallel()
	t.Run("invalid gzip", func(t *testing.T) {
		t.Parallel()
		getter := newMockSnapshotGetter([]byte("not-gzip"))
		filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)
		_, err := filterer.BlockNumber(context.Background())
		assert.Error(t, err)
	})

	t.Run("invalid log entry", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, err := gz.Write([]byte("not-a-log-entry"))
		require.NoError(t, err)
		gz.Close()
		getter := newMockSnapshotGetter(buf.Bytes())
		filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)
		_, err = filterer.BlockNumber(context.Background())
		assert.ErrorIs(t, err, listener.ErrParseSnapshot)
	})

	t.Run("non-sorted", func(t *testing.T) {
		t.Parallel()
		logs := []types.Log{
			{BlockNumber: 1, Topics: []common.Hash{}},
			{BlockNumber: 3, Topics: []common.Hash{}},
			{BlockNumber: 2, Topics: []common.Hash{}},
		}
		getter := newMockSnapshotGetter(makeSnapshotData(logs))
		filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)

		_, err := filterer.BlockNumber(context.Background())
		assert.ErrorIs(t, err, listener.ErrParseSnapshot)
	})

	t.Run("get block number", func(t *testing.T) {
		t.Parallel()
		logs := []types.Log{
			{BlockNumber: 1, Topics: []common.Hash{}},
			{BlockNumber: 2, Topics: []common.Hash{}},
			{BlockNumber: 2, Topics: []common.Hash{}},
			{BlockNumber: 3, Topics: []common.Hash{}},
		}
		getter := newMockSnapshotGetter(makeSnapshotData(logs))
		filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)

		blockNumber, err := filterer.BlockNumber(context.Background())
		assert.NoError(t, err)
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
		getter := newMockSnapshotGetter(makeSnapshotData(logs))
		filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)

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

func TestNew(t *testing.T) {
	t.Parallel()

	logs := []types.Log{
		{BlockNumber: 1, Topics: []common.Hash{}},
		{BlockNumber: 5, Topics: []common.Hash{}},
	}
	getter := newMockSnapshotGetter(makeSnapshotData(logs))

	snap, err := snapshot.New(context.Background(), log.Noop, getter, nil,
		common.Address{}, abi.ABI{}, time.Second, time.Second, time.Second, 100)
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, uint64(100), snap.StartBlock)
	assert.NotNil(t, snap.Listener)

	t.Run("corrupt snapshot returns an error", func(t *testing.T) {
		t.Parallel()
		_, err := snapshot.New(context.Background(), log.Noop, newMockSnapshotGetter([]byte("not-gzip")), nil,
			common.Address{}, abi.ABI{}, time.Second, time.Second, time.Second, 100)
		assert.Error(t, err)
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

	// Logs span up to maxBlock under an arbitrary address. The listener is built
	// with the zero contract address, so FilterLogs filters them all out and only
	// the per-page UpdateBlockNumber(to) advances the chain state — isolating the
	// resume point from event processing.
	logs := []types.Log{
		{BlockNumber: 10, Address: common.HexToAddress("0x1"), Topics: []common.Hash{}},
		{BlockNumber: maxBlock, Address: common.HexToAddress("0x1"), Topics: []common.Hash{}},
	}
	filterer := snapshot.NewSnapshotLogFilterer(log.Noop, newMockSnapshotGetter(makeSnapshotData(logs)))

	l := listener.New(nil, log.Noop, filterer, common.Address{}, abi.ABI{}, time.Second, time.Minute, time.Second, listener.DefaultBlockPage)
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

// blockRecorder is a postage.EventUpdater that reports every block the listener
// commits via UpdateBlockNumber and ignores everything else.
type blockRecorder struct{ blocks chan uint64 }

func (r *blockRecorder) UpdateBlockNumber(blockNumber uint64) error {
	r.blocks <- blockNumber
	return nil
}

func (r *blockRecorder) Create(_, _ []byte, _, _ *big.Int, _, _ uint8, _ bool, _ common.Hash) error {
	return nil
}
func (r *blockRecorder) TopUp(_ []byte, _, _ *big.Int, _ common.Hash) error             { return nil }
func (r *blockRecorder) UpdateDepth(_ []byte, _ uint8, _ *big.Int, _ common.Hash) error { return nil }
func (r *blockRecorder) UpdatePrice(_ *big.Int, _ common.Hash) error                    { return nil }
func (r *blockRecorder) Start(_ context.Context, _ uint64) error                        { return nil }
func (r *blockRecorder) TransactionStart() error                                        { return nil }
func (r *blockRecorder) TransactionEnd() error                                          { return nil }

var (
	fileContract    = common.HexToAddress("0x45A1502382541Cd610CC9068e88727426b696293")
	fileContractABI = abiutil.MustParseABI(chaincfg.Testnet.PostageStampABI)
	priceTopic      = fileContractABI.Events["PriceUpdate"].ID
)

// writeSnapshotFile writes data to a file in a fresh temp dir and returns its path.
func writeSnapshotFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.ndjson.gz")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// gzipRaw compresses raw NDJSON text into a single gzip member.
func gzipRaw(t *testing.T, raw string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
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

func newFromFile(path string, startBlock uint64) (*batchservice.Snapshot, snapshot.SnapshotInfo, error) {
	return snapshot.NewFromFile(context.Background(), log.Noop, path, nil,
		fileContract, fileContractABI, time.Second, time.Minute, time.Second, startBlock)
}

func TestNewFromFile(t *testing.T) {
	t.Parallel()

	const startBlock = uint64(100)

	t.Run("valid file replays its events", func(t *testing.T) {
		t.Parallel()
		path := writeSnapshotFile(t, makeSnapshotData([]types.Log{
			priceLog(110, 42),
			priceLog(120, 43),
		}))

		snap, info, err := newFromFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.SnapshotInfo{LogCount: 2, MaxBlock: 120}, info)
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
		path := writeSnapshotFile(t, gzipRaw(t, line))

		_, info, err := newFromFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.SnapshotInfo{LogCount: 1, MaxBlock: 120}, info)
	})

	t.Run("multi-member gzip", func(t *testing.T) {
		t.Parallel()
		// The second member starts on the block the first one ended on.
		data := append(
			makeSnapshotData([]types.Log{priceLog(105, 1), priceLog(110, 2)}),
			makeSnapshotData([]types.Log{priceLog(110, 3), priceLog(130, 4)})...,
		)
		path := writeSnapshotFile(t, data)

		_, info, err := newFromFile(path, startBlock)
		require.NoError(t, err)
		assert.Equal(t, snapshot.SnapshotInfo{LogCount: 4, MaxBlock: 130}, info)
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, _, err := newFromFile(filepath.Join(t.TempDir(), "missing.gz"), startBlock)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("directory path", func(t *testing.T) {
		t.Parallel()
		_, _, err := newFromFile(t.TempDir(), startBlock)
		require.ErrorIs(t, err, syscall.EISDIR)
		// Rejected before any read, so the error is the same on every OS; on
		// Windows reading a directory handle fails with an unrelated error.
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, "open", pathErr.Op)
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()
		_, _, err := newFromFile(writeSnapshotFile(t, nil), startBlock)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("not gzip", func(t *testing.T) {
		t.Parallel()
		_, _, err := newFromFile(writeSnapshotFile(t, []byte(`{"blockNumber":"0x1"}`+"\n")), startBlock)
		assert.ErrorIs(t, err, gzip.ErrHeader)
	})

	t.Run("gzip with non-JSON lines", func(t *testing.T) {
		t.Parallel()
		_, _, err := newFromFile(writeSnapshotFile(t, gzipRaw(t, "not-a-log-entry\n")), startBlock)
		assert.ErrorIs(t, err, listener.ErrParseSnapshot)
	})

	t.Run("gzip with no logs", func(t *testing.T) {
		t.Parallel()
		_, _, err := newFromFile(writeSnapshotFile(t, makeSnapshotData(nil)), startBlock)
		assert.ErrorIs(t, err, snapshot.ErrEmptySnapshot)
	})

	t.Run("contract mismatch on line 3", func(t *testing.T) {
		t.Parallel()
		other := common.HexToAddress("0x1234567890123456789012345678901234567890")
		bad := priceLog(115, 3)
		bad.Address = other
		path := writeSnapshotFile(t, makeSnapshotData([]types.Log{priceLog(105, 1), priceLog(110, 2), bad, priceLog(120, 4)}))

		_, _, err := newFromFile(path, startBlock)
		require.ErrorIs(t, err, snapshot.ErrContractMismatch)
		assert.ErrorContains(t, err, "line 3")
		assert.ErrorContains(t, err, other.Hex())
		assert.ErrorContains(t, err, fileContract.Hex())
	})

	t.Run("max block at start block", func(t *testing.T) {
		t.Parallel()
		// Head 100 trims to 95, below the first replayed block 101.
		path := writeSnapshotFile(t, makeSnapshotData([]types.Log{priceLog(90, 1), priceLog(startBlock, 2)}))
		_, _, err := newFromFile(path, startBlock)
		assert.ErrorIs(t, err, snapshot.ErrBlockHeightTooLow)
	})

	t.Run("max block below tail", func(t *testing.T) {
		t.Parallel()
		path := writeSnapshotFile(t, makeSnapshotData([]types.Log{priceLog(2, 1)}))
		_, _, err := newFromFile(path, 0)
		assert.ErrorIs(t, err, snapshot.ErrBlockHeightTooLow)
	})
}

// priceRecorder is a postage.EventUpdater that records every price update.
type priceRecorder struct {
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

func (r *priceRecorder) Create(_, _ []byte, _, _ *big.Int, _, _ uint8, _ bool, _ common.Hash) error {
	return nil
}
func (r *priceRecorder) TopUp(_ []byte, _, _ *big.Int, _ common.Hash) error             { return nil }
func (r *priceRecorder) UpdateDepth(_ []byte, _ uint8, _ *big.Int, _ common.Hash) error { return nil }
func (r *priceRecorder) UpdateBlockNumber(_ uint64) error                               { return nil }
func (r *priceRecorder) Start(_ context.Context, _ uint64) error                        { return nil }
func (r *priceRecorder) TransactionStart() error                                        { return nil }
func (r *priceRecorder) TransactionEnd() error                                          { return nil }
