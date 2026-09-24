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

	eventListener := listener.New(syncingStopped, logger, filterer, contractAddress, contractABI, blockTime, stallingTimeout, backoffTimeout)

	return &batchservice.Snapshot{
		Listener:   eventListener,
		StartBlock: startBlock,
	}, nil
}
