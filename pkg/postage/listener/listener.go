// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
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
	"github.com/ethersphere/bee/v2/pkg/safe"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/ethersphere/bee/v2/pkg/util/syncutil"
	"github.com/prometheus/client_golang/prometheus"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "listener"

// DefaultBlockPage is the default number of blocks requested in a single
// FilterLogs call while paging.
const DefaultBlockPage = uint64(1000)

const (
	tailSize           = 4         // how many blocks to tail from the tip of the chain
	defaultBatchFactor = uint64(5) // minimal number of blocks to sync at once
)

// for testing, set externally
var batchFactorOverridePublic = "5"

var (
	ErrPostageSyncingStalled = errors.New("postage syncing stalled")
	ErrPostagePaused         = errors.New("postage contract is paused")
)

// SyncTarget returns the highest block Listen syncs up to when the backend
// reports blockNumber as its head: the head minus the reorg-safety tail, rounded
// down to the batch factor. It reports false while the head is still within the
// tail, in which case Listen makes no progress.
func SyncTarget(blockNumber uint64) (uint64, bool) {
	if blockNumber < tailSize {
		return 0, false
	}
	bf := batchFactor()
	return (blockNumber - tailSize) / bf * bf, true
}

// batchFactor returns the effective batch factor: the build-time override when it
// parses to a positive value, the default otherwise.
func batchFactor() uint64 {
	bf, err := strconv.ParseUint(batchFactorOverridePublic, 10, 64)
	if err != nil || bf == 0 {
		return defaultBatchFactor
	}
	return bf
}

type BlockHeightContractFilterer interface {
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
	BlockNumber(context.Context) (uint64, error)
}

// ErrInvalidSegments is returned when the segment list is empty, does not start
// at block 0 or is not strictly increasing by FromBlock.
var ErrInvalidSegments = errors.New("listener: invalid segments")

// Segment describes the postage stamp contract that is active from FromBlock
// (inclusive) until the FromBlock of the next segment.
type Segment struct {
	Address   common.Address
	ABI       abi.ABI
	FromBlock uint64
}

// segment is a Segment with its cached event topics.
type segment struct {
	Segment

	batchCreatedTopic       common.Hash
	batchTopUpTopic         common.Hash
	batchDepthIncreaseTopic common.Hash
	priceUpdateTopic        common.Hash
	pausedTopic             common.Hash
}

func newSegment(s Segment) segment {
	return segment{
		Segment:                 s,
		batchCreatedTopic:       s.ABI.Events["BatchCreated"].ID,
		batchTopUpTopic:         s.ABI.Events["BatchTopUp"].ID,
		batchDepthIncreaseTopic: s.ABI.Events["BatchDepthIncrease"].ID,
		priceUpdateTopic:        s.ABI.Events["PriceUpdate"].ID,
		pausedTopic:             s.ABI.Events["Paused"].ID,
	}
}

type listener struct {
	logger    log.Logger
	ev        BlockHeightContractFilterer
	blockTime time.Duration

	segments        []segment
	quit            chan struct{}
	wg              sync.WaitGroup
	metrics         metrics
	stallingTimeout time.Duration
	backoffTime     time.Duration
	blockPage       uint64
	syncingStopped  *syncutil.Signaler
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
	blockPage uint64,
) postage.Listener {
	// A single segment starting at block 0 is always valid.
	l, _ := NewSegmented(syncingStopped, logger, ev, []Segment{{
		Address:   postageStampContractAddress,
		ABI:       postageStampContractABI,
		FromBlock: 0,
	}}, blockTime, stallingTimeout, backoffTime, blockPage)
	return l
}

// NewSegmented creates a listener that follows the postage stamp contract
// across version cutovers. segments must be non-empty, start at block 0 and be
// strictly increasing by FromBlock. No single FilterLogs query spans two
// segments.
func NewSegmented(
	syncingStopped *syncutil.Signaler,
	logger log.Logger,
	ev BlockHeightContractFilterer,
	segments []Segment,
	blockTime time.Duration,
	stallingTimeout time.Duration,
	backoffTime time.Duration,
	blockPage uint64,
) (postage.Listener, error) {
	if len(segments) == 0 {
		return nil, fmt.Errorf("%w: no segments", ErrInvalidSegments)
	}
	if segments[0].FromBlock != 0 {
		return nil, fmt.Errorf("%w: first segment must start at block 0", ErrInvalidSegments)
	}
	segs := make([]segment, 0, len(segments))
	for i, s := range segments {
		if i > 0 && s.FromBlock <= segments[i-1].FromBlock {
			return nil, fmt.Errorf("%w: segments not strictly increasing", ErrInvalidSegments)
		}
		segs = append(segs, newSegment(s))
	}
	if blockPage == 0 {
		blockPage = DefaultBlockPage
	}
	return &listener{
		syncingStopped:  syncingStopped,
		logger:          logger.WithName(loggerName).Register(),
		ev:              ev,
		blockTime:       blockTime,
		segments:        segs,
		quit:            make(chan struct{}),
		metrics:         newMetrics(),
		stallingTimeout: stallingTimeout,
		backoffTime:     backoffTime,
		blockPage:       blockPage,
	}, nil
}

// segmentAt returns the index of the segment active at block.
func (l *listener) segmentAt(block uint64) int {
	return sort.Search(len(l.segments), func(i int) bool {
		return l.segments[i].FromBlock > block
	}) - 1
}

func (l *listener) filterQuery(seg *segment, from, to *big.Int) ethereum.FilterQuery {
	return ethereum.FilterQuery{
		FromBlock: from,
		ToBlock:   to,
		Addresses: []common.Address{
			seg.Address,
		},
		Topics: [][]common.Hash{
			{
				seg.batchCreatedTopic,
				seg.batchTopUpTopic,
				seg.batchDepthIncreaseTopic,
				seg.priceUpdateTopic,
				seg.pausedTopic,
			},
		},
	}
}

func (l *listener) processEvent(seg *segment, e types.Log, updater postage.EventUpdater) error {
	defer l.metrics.EventsProcessed.Inc()
	switch e.Topics[0] {
	case seg.batchCreatedTopic:
		c := &batchCreatedEvent{}
		err := transaction.ParseEvent(&seg.ABI, "BatchCreated", c, e)
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
	case seg.batchTopUpTopic:
		c := &batchTopUpEvent{}
		err := transaction.ParseEvent(&seg.ABI, "BatchTopUp", c, e)
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
	case seg.batchDepthIncreaseTopic:
		c := &batchDepthIncreaseEvent{}
		err := transaction.ParseEvent(&seg.ABI, "BatchDepthIncrease", c, e)
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
	case seg.priceUpdateTopic:
		c := &priceUpdateEvent{}
		err := transaction.ParseEvent(&seg.ABI, "PriceUpdate", c, e)
		if err != nil {
			return err
		}
		l.metrics.PriceCounter.Inc()
		return updater.UpdatePrice(
			c.Price,
			e.TxHash,
		)
	case seg.pausedTopic:
		if seg != &l.segments[len(l.segments)-1] {
			// a superseded contract is expected to be paused during a
			// migration; keep syncing into the next segment.
			l.logger.Debug("superseded postage contract is paused", "contract_address", seg.Address)
			return nil
		}
		l.logger.Warning("Postage contract is paused.")
		return ErrPostagePaused
	default:
		l.metrics.EventErrors.Inc()
		return errors.New("unknown event")
	}
}

func (l *listener) Listen(ctx context.Context, from uint64, updater postage.EventUpdater) <-chan error {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-l.quit
		cancel()
	}()

	processEvents := func(seg *segment, events []types.Log, to uint64) error {
		if err := updater.TransactionStart(); err != nil {
			return err
		}

		for _, e := range events {
			startEv := time.Now()
			err := updater.UpdateBlockNumber(e.BlockNumber)
			if err != nil {
				return err
			}
			if err = l.processEvent(seg, e, updater); err != nil {
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

	bf := batchFactor()
	l.logger.Debug("batch factor", "value", bf)

	pageSize := l.blockPage
	l.logger.Debug("block page size", "page_size", pageSize)

	synced := make(chan error)
	closeOnce := new(sync.Once)
	paged := true

	lastProgress := time.Now()
	lastConfirmedBlock := uint64(0)

	l.wg.Add(1)
	listenf := safe.RunFunc(l.logger, "postage-listener-func", func() error {
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
				nextExpectedBatchBlock := (lastConfirmedBlock/bf + 1) * bf
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
				paged = false
				continue
			}

			target, ok := SyncTarget(to)
			if !ok {
				// in a test blockchain there might be not be enough blocks yet
				continue
			}
			lastConfirmedBlock = to - tailSize
			to = target

			if to < from {
				// if the blockNumber is actually less than what we already, it might mean the backend is not synced or some reorg scenario
				continue
			}

			// do some paging (sub-optimal)
			final := to-from < pageSize
			if !final {
				paged = true
				to = from + pageSize - 1
			}

			// never let a query span two contract segments: clamp at the
			// next segment boundary and keep paging until the real target
			idx := l.segmentAt(from)
			seg := &l.segments[idx]
			if idx+1 < len(l.segments) && l.segments[idx+1].FromBlock <= to {
				to = l.segments[idx+1].FromBlock - 1
				final = false
				paged = true
			}
			l.metrics.BackendCalls.Inc()

			events, err := l.ev.FilterLogs(ctx, l.filterQuery(seg, big.NewInt(int64(from)), big.NewInt(int64(to))))
			if err != nil {
				l.metrics.BackendErrors.Inc()
				l.logger.Warning("could not get blockchain log", "error", err)
				lastConfirmedBlock = 0
				paged = false
				continue
			}

			if err := processEvents(seg, events, to); err != nil {
				return err
			}

			from = to + 1
			lastProgress = time.Now()
			totalTimeMetric(l.metrics.PageProcessDuration, start)
			l.metrics.PagesProcessed.Inc()

			// Report the sync as done only once the final page is in the store,
			// so a failure on it reaches the caller instead of being lost after
			// an early signal.
			if final {
				closeOnce.Do(func() { synced <- nil })
			}
		}
	})

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
