// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot/archive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSnapshotLogFilterer_RealSnapshot parses the snapshot blob actually embedded
// in the binary. It guards against a missing, empty, or unparseable embed (e.g. a
// bad batch-archive bump), which would otherwise only surface at runtime as a
// stalled postage sync.
func TestSnapshotLogFilterer_RealSnapshot(t *testing.T) {
	t.Parallel()

	getter := archive.Getter{}

	// Sanity, fail-fast before the filter subtests run against the filterer: the
	// embed must carry data, parse cleanly, and contain logs. Otherwise a bad
	// batch-archive bump only surfaces at runtime as a stalled postage sync.
	require.NotEmpty(t, getter.GetBatchSnapshot(), "embedded batch snapshot is empty")

	filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)

	maxBlock, err := filterer.BlockNumber(context.Background())
	if err != nil {
		t.Fatalf("embedded batch snapshot failed to parse: %v", err)
	}
	if maxBlock == 0 {
		t.Fatal("embedded batch snapshot has no logs (max block height 0)")
	}

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
	getter := archive.Getter{}

	for b.Loop() {
		filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)
		_, err := filterer.BlockNumber(context.Background())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSnapshotLogFilterer(b *testing.B) {
	getter := archive.Getter{}
	filterer := snapshot.NewSnapshotLogFilterer(log.Noop, getter)
	// ensure loaded
	if _, err := filterer.BlockNumber(context.Background()); err != nil {
		b.Fatal(err)
	}

	b.Run("FilterLogs", func(b *testing.B) {
		for b.Loop() {
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
