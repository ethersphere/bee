// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/postage/listener"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
)

// Config holds what Load needs to build the replay listener and to validate a
// strict source.
type Config struct {
	// Contract is the postage stamp contract whose events the snapshot holds.
	Contract common.Address
	ABI      abi.ABI
	// StartBlock is the block the replay starts after (the postage contract
	// start block).
	StartBlock      uint64
	BlockTime       time.Duration
	StallingTimeout time.Duration
	BackoffTimeout  time.Duration
	SyncingStopped  *syncutil.Signaler
	// Strict rejects a snapshot that is empty, holds logs from another contract,
	// or does not reach far enough past StartBlock for the replay to make
	// progress. It is set for a source the operator asked for explicitly.
	Strict bool
}

// Load parses src and wraps it in the listener that replays it into the batch
// store. The snapshot is parsed eagerly so a corrupt one fails here, not after
// the sync timeout; the source is closed before Load returns.
func Load(logger log.Logger, src Source, cfg Config) (*batchservice.Snapshot, Info, error) {
	logger.Info("loading batch snapshot", "source", src.Name())

	filterer, info, err := Parse(logger, src, cfg.Contract, cfg.Strict)
	if err != nil {
		return nil, Info{}, err
	}
	if cfg.Strict {
		// The replay starts at StartBlock+1; below that the listener would wait
		// for the stalling timeout and then shut the node down.
		if target, ok := listener.SyncTarget(info.MaxBlock); !ok || target < cfg.StartBlock+1 {
			return nil, Info{}, fmt.Errorf("%w: max block %d, start block %d", ErrBlockHeightTooLow, info.MaxBlock, cfg.StartBlock)
		}
	}

	eventListener := listener.New(cfg.SyncingStopped, logger, filterer, cfg.Contract, cfg.ABI, cfg.BlockTime, cfg.StallingTimeout, cfg.BackoffTimeout, listener.SnapshotBlockPage)

	return &batchservice.Snapshot{
		Listener:   eventListener,
		StartBlock: cfg.StartBlock,
	}, info, nil
}
