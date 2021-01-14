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
)

const (
	blockPage = 500 // how many blocks to sync every time
	tailSize  = 100 // how many blocks to tail from the tip of the chain
)

var (
	chainUpdateInterval = 30 * time.Second
)

type BlockHeightContractFilterer interface {
	bind.ContractFilterer
	BlockNumber(context.Context) (uint64, error)
}

type listener struct {
	logger          logging.Logger
	ev              BlockHeightContractFilterer
	postageStampABI abi.ABI
	priceOracleABI  abi.ABI

	batchCreatedTopic       common.Hash
	batchTopupTopic         common.Hash
	batchDepthIncreaseTopic common.Hash
	priceUpdateTopic        common.Hash

	postageStampAddress common.Address
	priceOracleAddress  common.Address

	quit chan struct{}
	wg   sync.WaitGroup
}

func New(
	logger logging.Logger,
	ev BlockHeightContractFilterer,
	postageStampAddress,
	priceOracleAddress common.Address,
) postage.Listener {
	postageStampABI := parseABI(PostageStampABI)
	priceOracleABI := parseABI(PriceOracleABI)
	return &listener{
		logger:                  logger,
		ev:                      ev,
		postageStampABI:         postageStampABI,
		priceOracleABI:          priceOracleABI,
		batchCreatedTopic:       postageStampABI.Events["BatchCreated"].ID,
		batchTopupTopic:         postageStampABI.Events["BatchTopUp"].ID,
		batchDepthIncreaseTopic: postageStampABI.Events["BatchDepthIncrease"].ID,
		priceUpdateTopic:        priceOracleABI.Events["PriceUpdate"].ID,
	}
}

func (l *listener) Listen(from uint64, updater postage.EventUpdater) error {
	l.wg.Add(1)

	go func() {
		defer l.wg.Done()
		err := l.sync(from, updater)
		if err != nil {
			l.logger.Errorf("event listener sync: %v", err)
		}
	}()

	return nil
}

func (l *listener) filterQuery(from, to *big.Int) ethereum.FilterQuery {
	return ethereum.FilterQuery{
		FromBlock: from,
		ToBlock:   to,
		Addresses: []common.Address{
			l.postageStampAddress,
			l.priceOracleAddress,
		},
		Topics: [][]common.Hash{
			{
				l.batchCreatedTopic,
				l.batchTopupTopic,
				l.batchDepthIncreaseTopic,
				l.priceUpdateTopic,
			},
		},
	}
}

func (l *listener) parseEvent(a *abi.ABI, eventName string, c interface{}, e types.Log) error {
	err := a.Unpack(c, eventName, e.Data)
	if err != nil {
		return err
	}

	var indexed abi.Arguments
	for _, arg := range a.Events[eventName].Inputs {
		if arg.Indexed {
			indexed = append(indexed, arg)
		}
	}
	return abi.ParseTopics(c, indexed, e.Topics[1:])
}

func (l *listener) processEvent(e types.Log, updater postage.EventUpdater) error {
	eventSig := e.Topics[0]
	switch eventSig {
	case l.batchCreatedTopic:
		c := &batchCreatedEvent{}
		err := l.parseEvent(&l.postageStampABI, "BatchCreated", c, e)
		if err != nil {
			return err
		}
		return updater.Create(
			c.BatchId[:],
			c.Owner.Bytes(),
			c.TotalAmount,
			c.NormalisedBalance,
			c.Depth,
		)
	case l.batchTopupTopic:
		c := &batchTopUpEvent{}
		err := l.parseEvent(&l.postageStampABI, "BatchTopUp", c, e)
		if err != nil {
			return err
		}
		return updater.TopUp(
			c.BatchId[:],
			c.TopupAmount,
			c.NormalisedBalance,
		)
	case l.batchDepthIncreaseTopic:
		c := &batchDepthIncreaseEvent{}
		err := l.parseEvent(&l.postageStampABI, "BatchDepthIncrease", c, e)
		if err != nil {
			return err
		}
		return updater.UpdateDepth(
			c.BatchId[:],
			c.NewDepth,
			c.NormalisedBalance,
		)
	case l.priceUpdateTopic:
		c := &priceUpdateEvent{}
		err := l.parseEvent(&l.priceOracleABI, "PriceUpdate", c, e)
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

func (l *listener) sync(from uint64, updater postage.EventUpdater) error {
	ctx := context.Background()
	paged := make(chan struct{}, 1)
	paged <- struct{}{}
	for {
		select {
		case <-paged:
			// if we paged then it means there's more things to sync on
		case <-time.After(chainUpdateInterval):
		case <-l.quit:
			return nil
		}
		to, err := l.ev.BlockNumber(context.Background())
		if err != nil {
			return err
		}

		// consider to-tailSize as the "latest" block we need to sync to
		to = to - tailSize

		// do some paging (sub-optimal)
		if to-from > blockPage {
			paged <- struct{}{}
			to = from + blockPage
		}

		events, err := l.ev.FilterLogs(ctx, l.filterQuery(big.NewInt(int64(from)), big.NewInt(int64(to))))
		if err != nil {
			return err
		}

		for _, e := range events {
			if err = l.processEvent(e, updater); err != nil {
				return err
			}
		}
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
