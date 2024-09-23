// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener

import (
	"context"
	"errors"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchservice"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
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

type listener struct {
	logger    log.Logger
	ev        BlockHeightContractFilterer
	blockTime time.Duration

	postageStampContractAddress common.Address
	postageStampContractABI     abi.ABI
	quit                        chan struct{}
	wg                          sync.WaitGroup
	metrics                     metrics
	stallingTimeout             time.Duration
	backoffTime                 time.Duration
	syncingStopped              *syncutil.Signaler

	// Cached postage stamp contract event topics.
	batchCreatedTopic       common.Hash
	batchTopUpTopic         common.Hash
	batchDepthIncreaseTopic common.Hash
	priceUpdateTopic        common.Hash
	pausedTopic             common.Hash
}

func New(
	syncingStopped *syncutil.Signaler,
	logger log.Logger,
	ev BlockHeightContractFilterer,
	postageStampContractAddress common.Address,
	postageStampContractABI abi.ABI,
	blockTime time.Duration,
	stallingTimeout time.Duration,
	backoffTime time.Duration,
) postage.Listener {
	return &listener{
		syncingStopped:              syncingStopped,
		logger:                      logger.WithName(loggerName).Register(),
		ev:                          ev,
		blockTime:                   blockTime,
		postageStampContractAddress: postageStampContractAddress,
		postageStampContractABI:     postageStampContractABI,
		quit:                        make(chan struct{}),
		metrics:                     newMetrics(),
		stallingTimeout:             stallingTimeout,
		backoffTime:                 backoffTime,

		batchCreatedTopic:       postageStampContractABI.Events["BatchCreated"].ID,
		batchTopUpTopic:         postageStampContractABI.Events["BatchTopUp"].ID,
		batchDepthIncreaseTopic: postageStampContractABI.Events["BatchDepthIncrease"].ID,
		priceUpdateTopic:        postageStampContractABI.Events["PriceUpdate"].ID,
		pausedTopic:             postageStampContractABI.Events["Paused"].ID,
	}
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

func (l *listener) processEvent(e types.Log, updater postage.EventUpdater) error {
	defer l.metrics.EventsProcessed.Inc()
	switch e.Topics[0] {
	case l.batchCreatedTopic:
		c := &batchCreatedEvent{}
		err := transaction.ParseEvent(&l.postageStampContractABI, "BatchCreated", c, e)
		if err != nil {
			return err
		}
		l.metrics.CreatedCounter.Inc()
		return updater.Create(
			c.BatchId[:],
			c.Owner.Bytes(),
			c.TotalAmount,
			c.NormalisedBalance,
			c.Depth,
			c.BucketDepth,
			c.ImmutableFlag,
			e.TxHash,
		)
	case l.batchTopUpTopic:
		c := &batchTopUpEvent{}
		err := transaction.ParseEvent(&l.postageStampContractABI, "BatchTopUp", c, e)
		if err != nil {
			return err
		}
		l.metrics.TopupCounter.Inc()
		return updater.TopUp(
			c.BatchId[:],
			c.TopupAmount,
			c.NormalisedBalance,
			e.TxHash,
		)
	case l.batchDepthIncreaseTopic:
		c := &batchDepthIncreaseEvent{}
		err := transaction.ParseEvent(&l.postageStampContractABI, "BatchDepthIncrease", c, e)
		if err != nil {
			return err
		}
		l.metrics.DepthCounter.Inc()
		return updater.UpdateDepth(
			c.BatchId[:],
			c.NewDepth,
			c.NormalisedBalance,
			e.TxHash,
		)
	case l.priceUpdateTopic:
		c := &priceUpdateEvent{}
		err := transaction.ParseEvent(&l.postageStampContractABI, "PriceUpdate", c, e)
		if err != nil {
			return err
		}
		l.metrics.PriceCounter.Inc()
		return updater.UpdatePrice(
			c.Price,
			e.TxHash,
		)
	case l.pausedTopic:
		l.logger.Warning("Postage contract is paused.")
		return ErrPostagePaused
	default:
		l.metrics.EventErrors.Inc()
		return errors.New("unknown event")
	}
}

func (l *listener) Listen(ctx context.Context, from uint64, updater postage.EventUpdater, initState *postage.ChainSnapshot) <-chan error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-l.quit
		cancel()
	}()

	processEvents := func(events []types.Log, to uint64) error {
		if err := updater.TransactionStart(); err != nil {
			return err
		}

		for _, e := range events {
			startEv := time.Now()
			err := updater.UpdateBlockNumber(e.BlockNumber)
			if err != nil {
				return err
			}
			if err = l.processEvent(e, updater); err != nil {
				// if we have a zero value batch - silence & log then move on
				if !errors.Is(err, batchservice.ErrZeroValueBatch) {
					return err
				}
				l.logger.Debug("failed processing event", "error", err)
			}
			totalTimeMetric(l.metrics.EventProcessDuration, startEv)
		}

		err := updater.UpdateBlockNumber(to)
		if err != nil {
			return err
		}

		if err := updater.TransactionEnd(); err != nil {
			return err
		}

		return nil
	}

	if initState != nil {
		err := processEvents(initState.Events, initState.LastBlockNumber+1)
		if err != nil {
			l.logger.Error(err, "failed bootstrapping from initial state")
		}
	}

	batchFactor, err := strconv.ParseUint(batchFactorOverridePublic, 10, 64)
	if err != nil {
		l.logger.Warning("batch factor conversation failed", "batch_factor", batchFactor, "error", err)
		batchFactor = defaultBatchFactor
	}

	l.logger.Debug("batch factor", "value", batchFactor)

	synced := make(chan error)
	closeOnce := new(sync.Once)
	paged := true

	lastProgress := time.Now()
	lastConfirmedBlock := uint64(0)

	l.wg.Add(1)
	listenf := func() error {
		defer l.wg.Done()
		for {
			// if for whatever reason we are stuck for too long we terminate
			// this can happen because of rpc errors but also because of a stalled backend node
			// this does not catch the case were a backend node is actively syncing but not caught up
			if time.Since(lastProgress) >= l.stallingTimeout {
				return ErrPostageSyncingStalled
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// if we have a last blocknumber from the backend we can make a good estimate on when we need to requery
			// otherwise we just use the backoff time
			var expectedWaitTime time.Duration
			if lastConfirmedBlock != 0 {
				nextExpectedBatchBlock := (lastConfirmedBlock/batchFactor + 1) * batchFactor
				remainingBlocks := nextExpectedBatchBlock - lastConfirmedBlock
				expectedWaitTime = l.blockTime * time.Duration(remainingBlocks)
			} else {
				expectedWaitTime = l.backoffTime
			}

			if !paged {
				l.logger.Debug("sleeping until next block batch", "duration", expectedWaitTime)
				select {
				case <-time.After(expectedWaitTime):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			paged = false

			start := time.Now()

			l.metrics.BackendCalls.Inc()
			to, err := l.ev.BlockNumber(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}

				l.metrics.BackendErrors.Inc()
				l.logger.Warning("could not get block number", "error", err)
				lastConfirmedBlock = 0
				continue
			}

			if to < tailSize {
				// in a test blockchain there might be not be enough blocks yet
				continue
			}

			// consider to-tailSize as the "latest" block we need to sync to
			to = to - tailSize
			lastConfirmedBlock = to

			// round down to the largest multiple of batchFactor
			to = (to / batchFactor) * batchFactor

			if to < from {
				// if the blockNumber is actually less than what we already, it might mean the backend is not synced or some reorg scenario
				continue
			}

			// do some paging (sub-optimal)
			if to-from >= blockPage {
				paged = true
				to = from + blockPage - 1
			} else {
				closeOnce.Do(func() { synced <- nil })
			}
			l.metrics.BackendCalls.Inc()

			events, err := l.ev.FilterLogs(ctx, l.filterQuery(big.NewInt(int64(from)), big.NewInt(int64(to))))
			if err != nil {
				l.metrics.BackendErrors.Inc()
				l.logger.Warning("could not get blockchain log", "error", err)
				lastConfirmedBlock = 0
				continue
			}

			if err := processEvents(events, to); err != nil {
				return err
			}

			from = to + 1
			lastProgress = time.Now()
			totalTimeMetric(l.metrics.PageProcessDuration, start)
			l.metrics.PagesProcessed.Inc()
		}
	}

	go func() {
		err := listenf()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// Context cancelled is returned on shutdown, therefore we do nothing here.
				l.logger.Debug("shutting down event listener")
				return
			}
			l.logger.Error(err, "failed syncing event listener; shutting down node error")
		}
		closeOnce.Do(func() { synced <- err })
		if l.syncingStopped != nil {
			l.syncingStopped.Signal() // trigger shutdown in start.go
		}
	}()

	return synced
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
