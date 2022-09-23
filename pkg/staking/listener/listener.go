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
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "listener"

var (
	stakingABI = parseABI(postageabi.PostageStampABIv0_3_0)
	// stakeUpdatedTopic is the staking contract's deposit event topic
	stakeUpdatedTopic = stakingABI.Events["StakeUpdated"].ID
	// stakeUpdatedTopic is the staking contract's deposit event topic
	slashedTopic = stakingABI.Events["SlashedNotRevealed"].ID
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
				slashedTopic,
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
	case slashedTopic:
		c := &slashed{}
		err := transaction.ParseEvent(&stakingABI, "SlashedNotRevealed", c, e)
		if err != nil {
			return err
		}
		l.metrics.SlashingCounter.Inc()
		return updater.SlashedNotRevealed(c.Overlay[:])
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

type slashed struct {
	Overlay [32]byte
}
