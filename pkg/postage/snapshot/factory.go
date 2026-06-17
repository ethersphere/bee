// Copyright 2025 The Swarm Authors. All rights reserved.
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

// New assembles the postage-snapshot inputs the batch service needs to rebuild
// the store from the embedded snapshot: a listener that replays the snapshot's
// logs, the block the replay starts from, and the block height it reaches. The
// snapshot is parsed eagerly to resolve that height, so a corrupt snapshot
// surfaces here as an error and the caller can fall back to a full chain rebuild.
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

	resumeBlock, err := filterer.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("read postage snapshot: %w", err)
	}

	eventListener := listener.New(syncingStopped, logger, filterer, contractAddress, contractABI, blockTime, stallingTimeout, backoffTimeout)

	return &batchservice.Snapshot{
		Listener:    eventListener,
		StartBlock:  startBlock,
		ResumeBlock: resumeBlock,
	}, nil
}
