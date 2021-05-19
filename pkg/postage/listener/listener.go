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
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/settlement/swap/transaction"
	"github.com/ethersphere/go-storage-incentives-abi/postageabi"
)

const (
	blockPage = 5000 // how many blocks to sync every time we page
	tailSize  = 4    // how many blocks to tail from the tip of the chain
)

var (
	postageStampABI = parseABI(postageabi.PostageStampABIv0_2_0)
	// batchCreatedTopic is the postage contract's batch created event topic
	batchCreatedTopic = postageStampABI.Events["BatchCreated"].ID
	// batchTopupTopic is the postage contract's batch topup event topic
	batchTopupTopic = postageStampABI.Events["BatchTopUp"].ID
	// batchDepthIncreaseTopic is the postage contract's batch dilution event topic
	batchDepthIncreaseTopic = postageStampABI.Events["BatchDepthIncrease"].ID
	// priceUpdateTopic is the postage contract's price update event topic
	priceUpdateTopic = postageStampABI.Events["PriceUpdate"].ID
)

type BlockHeightContractFilterer interface {
	bind.ContractFilterer
	BlockNumber(context.Context) (uint64, error)
}

type listener struct {
	logger    logging.Logger
	ev        BlockHeightContractFilterer
	blockTime uint64

	postageStampAddress common.Address
	quit                chan struct{}
	wg                  sync.WaitGroup
}

func New(
	logger logging.Logger,
	ev BlockHeightContractFilterer,
	postageStampAddress common.Address,
	blockTime uint64,
) postage.Listener {
	return &listener{
		logger:              logger,
		ev:                  ev,
		blockTime:           blockTime,
		postageStampAddress: postageStampAddress,
		quit:                make(chan struct{}),
	}
}

func (l *listener) filterQuery(from, to *big.Int) ethereum.FilterQuery {
	return ethereum.FilterQuery{
		FromBlock: from,
		ToBlock:   to,
		Addresses: []common.Address{
			l.postageStampAddress,
		},
		Topics: [][]common.Hash{
			{
				batchCreatedTopic,
				batchTopupTopic,
				batchDepthIncreaseTopic,
				priceUpdateTopic,
			},
		},
	}
}

func (l *listener) processEvent(e types.Log, updater postage.EventUpdater) error {
	switch e.Topics[0] {
	case batchCreatedTopic:
		c := &batchCreatedEvent{}
		err := transaction.ParseEvent(&postageStampABI, "BatchCreated", c, e)
		if err != nil {
			return err
		}
		return updater.Create(
			c.BatchId[:],
			c.Owner.Bytes(),
			c.NormalisedBalance,
			c.Depth,
		)
	case batchTopupTopic:
		c := &batchTopUpEvent{}
		err := transaction.ParseEvent(&postageStampABI, "BatchTopUp", c, e)
		if err != nil {
			return err
		}
		return updater.TopUp(
			c.BatchId[:],
			c.NormalisedBalance,
		)
	case batchDepthIncreaseTopic:
		c := &batchDepthIncreaseEvent{}
		err := transaction.ParseEvent(&postageStampABI, "BatchDepthIncrease", c, e)
		if err != nil {
			return err
		}
		return updater.UpdateDepth(
			c.BatchId[:],
			c.NewDepth,
			c.NormalisedBalance,
		)
	case priceUpdateTopic:
		c := &priceUpdateEvent{}
		err := transaction.ParseEvent(&postageStampABI, "PriceUpdate", c, e)
		if err != nil {
			return err
		}
		return updater.UpdatePrice(
			c.Price,
		)
	default:
		return errors.New("unknown event")
	}
}

func (l *listener) Listen(from uint64, updater postage.EventUpdater) <-chan struct{} {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-l.quit
		cancel()
	}()

	chainUpdateInterval := (time.Duration(l.blockTime) * time.Second) / 2

	synced := make(chan struct{})
	closeOnce := new(sync.Once)
	paged := make(chan struct{}, 1)
	paged <- struct{}{}

	l.wg.Add(1)
	listenf := func() error {
		defer l.wg.Done()
		for {
			select {
			case <-paged:
				// if we paged then it means there's more things to sync on
			case <-time.After(chainUpdateInterval):
			case <-l.quit:
				return nil
			}
			to, err := l.ev.BlockNumber(ctx)
			if err != nil {
				return err
			}

			if to < tailSize {
				// in a test blockchain there might be not be enough blocks yet
				continue
			}

			// consider to-tailSize as the "latest" block we need to sync to
			to = to - tailSize

			if to < from {
				// if the blockNumber is actually less than what we already, it might mean the backend is not synced or some reorg scenario
				continue
			}

			// do some paging (sub-optimal)
			if to-from > blockPage {
				paged <- struct{}{}
				to = from + blockPage
			} else {
				closeOnce.Do(func() { close(synced) })
			}

			events, err := l.ev.FilterLogs(ctx, l.filterQuery(big.NewInt(int64(from)), big.NewInt(int64(to))))
			if err != nil {
				return err
			}

			for _, e := range events {
				err = updater.UpdateBlockNumber(e.BlockNumber)
				if err != nil {
					return err
				}
				if err = l.processEvent(e, updater); err != nil {
					return err
				}
			}

			err = updater.UpdateBlockNumber(to)
			if err != nil {
				return err
			}

			from = to + 1
		}
	}

	go func() {
		err := listenf()
		if err != nil {
			l.logger.Errorf("event listener sync: %v", err)
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

func parseABI(json string) abi.ABI {
	cabi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic(fmt.Sprintf("error creating ABI for postage contract: %v", err))
	}
	return cabi
}

type batchCreatedEvent struct {
	BatchId           [32]byte
	TotalAmount       *big.Int
	NormalisedBalance *big.Int
	Owner             common.Address
	Depth             uint8
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

var (
	GoerliPostageStampContractAddress = common.HexToAddress("0xB3B7f2eD97B735893316aEeA849235de5e8972a2")
	GoerliStartBlock                  = uint64(4818979)
)

// DiscoverAddresses returns the canonical contracts for this chainID
func DiscoverAddresses(chainID int64) (postageStamp common.Address, startBlock uint64, found bool) {
	if chainID == 5 {
		// goerli
		return GoerliPostageStampContractAddress, GoerliStartBlock, true
	}
	return common.Address{}, 0, false
}
