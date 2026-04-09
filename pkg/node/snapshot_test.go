// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	archive "github.com/ethersphere/batch-archive"
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

type realSnapshotGetter struct{}

func (r realSnapshotGetter) GetBatchSnapshot() []byte {
	return archive.GetBatchSnapshot()
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
		filterer := node.NewSnapshotLogFilterer(log.Noop, getter)
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
		filterer := node.NewSnapshotLogFilterer(log.Noop, getter)
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
		filterer := node.NewSnapshotLogFilterer(log.Noop, getter)

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
		filterer := node.NewSnapshotLogFilterer(log.Noop, getter)

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
		filterer := node.NewSnapshotLogFilterer(log.Noop, getter)

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

func TestSnapshotLogFilterer_RealSnapshot(t *testing.T) {
	t.Parallel()

	getter := realSnapshotGetter{}
	filterer := node.NewSnapshotLogFilterer(log.Noop, getter)

	t.Run("block number", func(t *testing.T) {
		blockNumber, err := filterer.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Greater(t, blockNumber, uint64(0))
	})

	t.Run("filter range", func(t *testing.T) {
		// arbitrary range that should exist in the snapshot
		from := big.NewInt(20000000)
		to := big.NewInt(20001000)
		res, err := filterer.FilterLogs(context.Background(), ethereum.FilterQuery{
			FromBlock: from,
			ToBlock:   to,
		})
		require.NoError(t, err)
		for _, l := range res {
			assert.GreaterOrEqual(t, l.BlockNumber, from.Uint64())
			assert.LessOrEqual(t, l.BlockNumber, to.Uint64())
		}
	})

	t.Run("filter address mismatch", func(t *testing.T) {
		// random address that should not match the postage stamp contract
		addr := common.HexToAddress("0x1234567890123456789012345678901234567890")
		res, err := filterer.FilterLogs(context.Background(), ethereum.FilterQuery{
			Addresses: []common.Address{addr},
		})
		require.NoError(t, err)
		assert.Empty(t, res)
	})
}

func BenchmarkNewSnapshotLogFilterer_Load(b *testing.B) {
	getter := realSnapshotGetter{}

	for b.Loop() {
		filterer := node.NewSnapshotLogFilterer(log.Noop, getter)
		_, err := filterer.BlockNumber(context.Background())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSnapshotLogFilterer(b *testing.B) {
	getter := realSnapshotGetter{}
	filterer := node.NewSnapshotLogFilterer(log.Noop, getter)
	// ensure loaded
	if _, err := filterer.BlockNumber(context.Background()); err != nil {
		b.Fatal(err)
	}

	b.Run("FilterLogs", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			from := big.NewInt(20000000)
			to := big.NewInt(20001000)
			_, err := filterer.FilterLogs(context.Background(), ethereum.FilterQuery{
				FromBlock: from,
				ToBlock:   to,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
