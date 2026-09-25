// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
)

// SnapshotInfo describes a loaded snapshot file.
type SnapshotInfo struct {
	LogCount int
	MaxBlock uint64
}

// New builds the inputs the batch service needs to rebuild the store from the
// embedded snapshot: a listener that replays the snapshot's logs and the block to
// start from. The snapshot is parsed eagerly here so a corrupt one fails fast and
// the caller can fall back to a full chain rebuild.
func New(
	ctx context.Context,
	logger log.Logger,
	getter SnapshotGetter,
	syncingStopped *syncutil.Signaler,
	contractAddress common.Address,
	contractABI abi.ABI,
	blockTime time.Duration,
	stallingTimeout time.Duration,
	backoffTimeout time.Duration,
	startBlock uint64,
) (*batchservice.Snapshot, error) {
	filterer := NewSnapshotLogFilterer(logger, getter)

	// Parse the snapshot now so a corrupt one fails fast here; left to the
	// listener it would stall until the sync timeout before falling back.
	if _, err := filterer.BlockNumber(ctx); err != nil {
		return nil, fmt.Errorf("read postage snapshot: %w", err)
	}

	return newSnapshot(logger, filterer, syncingStopped, contractAddress, contractABI, blockTime, stallingTimeout, backoffTimeout, startBlock), nil
}

// NewFromFile is like New, but reads the snapshot from the file at path and
// validates it strictly, since the operator asked for this file explicitly: it
// must hold at least one log, every log must come from contractAddress, and it
// must reach far enough past startBlock for the replay to make progress.
func NewFromFile(
	ctx context.Context,
	logger log.Logger,
	path string,
	syncingStopped *syncutil.Signaler,
	contractAddress common.Address,
	contractABI abi.ABI,
	blockTime time.Duration,
	stallingTimeout time.Duration,
	backoffTimeout time.Duration,
	startBlock uint64,
) (*batchservice.Snapshot, SnapshotInfo, error) {
	getter, err := readFile(path)
	if err != nil {
		return nil, SnapshotInfo{}, err
	}

	filterer := NewSnapshotLogFilterer(logger, getter)
	maxBlock, err := filterer.BlockNumber(ctx)
	if err != nil {
		return nil, SnapshotInfo{}, fmt.Errorf("read postage snapshot: %w", err)
	}
	if len(filterer.loadedLogs) == 0 {
		return nil, SnapshotInfo{}, ErrEmptySnapshot
	}
	// A log from another contract would be filtered out during replay while the
	// chain state still advanced past it, silently skipping history.
	if err := filterer.checkContract(contractAddress); err != nil {
		return nil, SnapshotInfo{}, err
	}
	// The replay starts at startBlock+1; below that the listener would wait for
	// the stalling timeout and then shut the node down.
	if target, ok := listener.SyncTarget(maxBlock); !ok || target < startBlock+1 {
		return nil, SnapshotInfo{}, fmt.Errorf("%w: max block %d, start block %d", ErrBlockHeightTooLow, maxBlock, startBlock)
	}

	info := SnapshotInfo{LogCount: len(filterer.loadedLogs), MaxBlock: maxBlock}
	return newSnapshot(logger, filterer, syncingStopped, contractAddress, contractABI, blockTime, stallingTimeout, backoffTimeout, startBlock), info, nil
}

// newSnapshot wraps a loaded filterer in the listener that replays it.
func newSnapshot(
	logger log.Logger,
	filterer *SnapshotLogFilterer,
	syncingStopped *syncutil.Signaler,
	contractAddress common.Address,
	contractABI abi.ABI,
	blockTime time.Duration,
	stallingTimeout time.Duration,
	backoffTimeout time.Duration,
	startBlock uint64,
) *batchservice.Snapshot {
	eventListener := listener.New(syncingStopped, logger, filterer, contractAddress, contractABI, blockTime, stallingTimeout, backoffTimeout, listener.DefaultBlockPage)

	return &batchservice.Snapshot{
		Listener:   eventListener,
		StartBlock: startBlock,
	}
}
