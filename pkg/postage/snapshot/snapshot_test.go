// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot"
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

	l := listener.New(nil, log.Noop, filterer, common.Address{}, abi.ABI{}, time.Second, time.Minute, time.Second)
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
