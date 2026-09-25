// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive_test

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot"
	"github.com/ethersphere/bee/v2/pkg/postage/snapshot/archive"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
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

	filterer, info, err := snapshot.Parse(log.Noop, snapshot.Embedded(getter), common.Address{}, false)
	if err != nil {
		t.Fatalf("embedded batch snapshot failed to parse: %v", err)
	}
	if info.LogCount == 0 {
		t.Fatal("embedded batch snapshot has no logs")
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

func BenchmarkParse(b *testing.B) {
	src := snapshot.Embedded(archive.Getter{})

	for b.Loop() {
		if _, _, err := snapshot.Parse(log.Noop, src, common.Address{}, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSnapshotLogFilterer(b *testing.B) {
	filterer, _, err := snapshot.Parse(log.Noop, snapshot.Embedded(archive.Getter{}), common.Address{}, false)
	if err != nil {
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

// TestLoadFile_RealSnapshot feeds the embedded blob through the operator file
// path, strictly, against the mainnet contract and start block it was exported
// for. It proves the strict validation accepts a real batch-export file: the
// slim line format, the sort order, the contract address on every line, and a
// max block far enough past the contract start block.
func TestLoadFile_RealSnapshot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "export.ndjson.gzip")
	if err := os.WriteFile(path, archive.Getter{}.GetBatchSnapshot(), 0o600); err != nil {
		t.Fatal(err)
	}

	snap, info, err := snapshot.Load(log.Noop, snapshot.File(path), snapshot.Config{
		Contract:        chaincfg.Mainnet.PostageStampAddress,
		ABI:             abiutil.MustParseABI(chaincfg.Mainnet.PostageStampABI),
		StartBlock:      chaincfg.Mainnet.PostageStampStartBlock,
		BlockTime:       time.Second,
		StallingTimeout: time.Minute,
		BackoffTimeout:  time.Second,
		Strict:          true,
	})
	if err != nil {
		t.Fatalf("embedded snapshot rejected by the file path: %v", err)
	}
	t.Cleanup(func() { _ = snap.Listener.Close() })

	_, embeddedInfo, err := snapshot.Parse(log.Noop, snapshot.Embedded(archive.Getter{}), common.Address{}, false)
	require.NoError(t, err)
	assert.Equal(t, "file", info.Source)
	assert.Equal(t, embeddedInfo.LogCount, info.LogCount)
	assert.Equal(t, embeddedInfo.MaxBlock, info.MaxBlock)
}
