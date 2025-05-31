// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "listener"

const (
	blockPage          = 5000      // how many blocks to sync every time we page
	tailSize           = 4         // how many blocks to tail from the tip of the chain
	defaultBatchFactor = uint64(5) // // minimal number of blocks to sync at once
)

var (
	// for testing, set externally
	batchFactorOverridePublic = "5"
)

var (
	ErrPostageSyncingStalled = errors.New("postage syncing stalled")
	ErrPostagePaused         = errors.New("postage contract is paused")
)

type BlockHeightContractFilterer interface {
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
	BlockNumber(context.Context) (uint64, error)
}

func (l *listener) filterQuery(from, to *big.Int) ethereum.FilterQuery {
	return ethereum.FilterQuery{
		FromBlock: from,
		ToBlock:   to,
		Addresses: []common.Address{
			l.postageStampContractAddress,
		},
		Topics: [][]common.Hash{
			{
				l.batchCreatedTopic,
				l.batchTopUpTopic,
				l.batchDepthIncreaseTopic,
				l.priceUpdateTopic,
				l.pausedTopic,
			},
		},
	}
}

func (l *listener) Close() error {
	close(l.quit)

	done := make(chan struct{})
	go func() {
		defer close(done)
		l.wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		return errors.New("postage listener closed with running goroutines")
	}
	return nil
}

type batchCreatedEvent struct {
	BatchId           [32]byte
	TotalAmount       *big.Int
	NormalisedBalance *big.Int
	Owner             common.Address
	Depth             uint8
	BucketDepth       uint8
	ImmutableFlag     bool
}

type batchTopUpEvent struct {
	BatchId           [32]byte
	TopupAmount       *big.Int
	NormalisedBalance *big.Int
}

type batchDepthIncreaseEvent struct {
	BatchId           [32]byte
	NewDepth          uint8
	NormalisedBalance *big.Int
}

type priceUpdateEvent struct {
	Price *big.Int
}

func totalTimeMetric(metric prometheus.Counter, start time.Time) {
	totalTime := time.Since(start)
	metric.Add(float64(totalTime))
}
