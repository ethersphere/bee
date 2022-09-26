// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/staking"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/util"
	"github.com/ethersphere/go-storage-incentives-abi/postageabi"
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
	//TODO: set staking abi here
	stakingABI = parseABI(postageabi.PostageStampABIv0_3_0)
	// stakeUpdatedTopic is the staking contract's deposit event topic
	stakeUpdatedTopic = stakingABI.Events["StakeUpdated"].ID

	ErrSyncStopped = errors.New("sync stopped")
)

type BlockHeightContractFilterer interface {
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
	BlockNumber(context.Context) (uint64, error)
}

type listener struct {
	logger    log.Logger
	ev        BlockHeightContractFilterer
	blockTime uint64

	stakingAddress  common.Address
	quit            chan struct{}
	wg              sync.WaitGroup
	metrics         metrics
	stallingTimeout time.Duration
	backoffTime     time.Duration
	syncingStopped  *util.Signaler
}

func New(
	syncingStopped *util.Signaler,
	logger log.Logger,
	ev BlockHeightContractFilterer,
	stakingAddress common.Address,
	blockTime uint64,
	stallingTimeout time.Duration,
	backoffTime time.Duration,
) staking.Listener {
	return &listener{
		syncingStopped:  syncingStopped,
		logger:          logger.WithName(loggerName).Register(),
		ev:              ev,
		blockTime:       blockTime,
		stakingAddress:  stakingAddress,
		quit:            make(chan struct{}),
		metrics:         newMetrics(),
		stallingTimeout: stallingTimeout,
		backoffTime:     backoffTime,
	}
}

func (l *listener) filterQuery(from, to *big.Int) ethereum.FilterQuery {
	return ethereum.FilterQuery{
		FromBlock: from,
		ToBlock:   to,
		Addresses: []common.Address{
			l.stakingAddress,
		},
		Topics: [][]common.Hash{
			{
				stakeUpdatedTopic,
			},
		},
	}
}

func (l *listener) processEvent(e types.Log, updater staking.EventUpdater) error {
	defer l.metrics.EventsProcessed.Inc()
	switch e.Topics[0] {
	case stakeUpdatedTopic:
		c := &stakingDeposit{}
		err := transaction.ParseEvent(&stakingABI, "StakeUpdated", c, e)
		if err != nil {
			return err
		}
		l.metrics.DepositedCounter.Inc()
		return updater.DepositStake(
			c.Overlay[:],
			c.stakeAmount,
			c.address,
			c.lastUpdatedBlock,
		)
	default:
		l.metrics.EventErrors.Inc()
		return errors.New("unknown event")
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
		return errors.New("staking listener closed with running goroutines")
	}
	return nil
}

func (l *listener) Listen(from uint64, updater staking.EventUpdater, initState *staking.ChainSnapshot) <-chan error {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-l.quit
		cancel()
	}()
	processEvents := func(events []types.Log, to uint64) error {
		for _, e := range events {
			startEv := time.Now()
			err := updater.UpdateBlockNumber(e.BlockNumber)
			if err != nil {
				return err
			}
			if err = l.processEvent(e, updater); err != nil {
				return err
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

	synced := make(chan error)
	closeOnce := new(sync.Once)
	paged := make(chan struct{}, 1)
	paged <- struct{}{}

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
				return ErrSyncStopped
			}

			// if we have a last blocknumber from the backend we can make a good estimate on when we need to requery
			// otherwise we just use the backoff time
			var expectedWaitTime time.Duration
			if lastConfirmedBlock != 0 {
				nextExpectedBatchBlock := (lastConfirmedBlock/defaultBatchFactor + 1) * defaultBatchFactor
				remainingBlocks := nextExpectedBatchBlock - lastConfirmedBlock
				expectedWaitTime = time.Duration(l.blockTime*remainingBlocks) * time.Second
			} else {
				expectedWaitTime = l.backoffTime
			}

			select {
			case <-paged:
				// if we paged then it means there's more things to sync on
			case <-time.After(expectedWaitTime):
			case <-l.quit:
				return nil
			}
			start := time.Now()

			l.metrics.BackendCalls.Inc()
			to, err := l.ev.BlockNumber(ctx)
			if err != nil {
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
			to = (to / defaultBatchFactor) * defaultBatchFactor

			if to < from {
				// if the blockNumber is actually less than what we already, it might mean the backend is not synced or some reorg scenario
				continue
			}

			// do some paging (sub-optimal)
			if to-from >= blockPage {
				paged <- struct{}{}
				to = from + blockPage - 1
			} else {
				closeOnce.Do(func() { synced <- nil })
			}
			l.metrics.BackendCalls.Inc()

			events, err := l.ev.FilterLogs(ctx, l.filterQuery(big.NewInt(int64(from)), big.NewInt(int64(to))))
			if err != nil {
				l.metrics.BackendErrors.Inc()
				l.logger.Warning("could not get logs", "error", err)
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
				// context cancelled is returned on shutdown,
				// therefore we do nothing here
				return
			}
			l.logger.Error(err, "failed syncing event listener; shutting down node error")
		}
		closeOnce.Do(func() { synced <- err })
		l.syncingStopped.Signal() // trigger shutdown in start.go
	}()

	return synced
}

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for postage contract: %v", err))
	}
	return cabi
}

type stakingDeposit struct {
	Overlay          [32]byte
	stakeAmount      *big.Int
	address          common.Address
	lastUpdatedBlock *big.Int
}

func totalTimeMetric(metric prometheus.Counter, start time.Time) {
	totalTime := time.Since(start)
	metric.Add(float64(totalTime))
}
