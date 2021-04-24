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
)

const (
	blockPage = 10000 // how many blocks to sync every time
	tailSize  = 4     // how many blocks to tail from the tip of the chain
)

var (
	chainUpdateInterval     = 5 * time.Second
	postageStampABI         = parseABI(PostageStampABIJSON)
	priceOracleABI          = parseABI(PriceOracleABIJSON)
	batchCreatedTopic       = postageStampABI.Events["BatchCreated"].ID
	batchTopupTopic         = postageStampABI.Events["BatchTopUp"].ID
	batchDepthIncreaseTopic = postageStampABI.Events["BatchDepthIncrease"].ID
	priceUpdateTopic        = priceOracleABI.Events["PriceUpdate"].ID
)

type BlockHeightContractFilterer interface {
	bind.ContractFilterer
	BlockNumber(context.Context) (uint64, error)
}

type listener struct {
	logger logging.Logger
	ev     BlockHeightContractFilterer

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
	return &listener{
		logger: logger,
		ev:     ev,

		postageStampAddress: postageStampAddress,
		priceOracleAddress:  priceOracleAddress,

		quit: make(chan struct{}),
	}
}

func (l *listener) Listen(from uint64, updater postage.EventUpdater) {
	l.wg.Add(1)

	go func() {
		defer l.wg.Done()
		err := l.sync(from, updater)
		if err != nil {
			l.logger.Errorf("event listener sync: %v", err)
		}
	}()
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
		err := transaction.ParseEvent(&priceOracleABI, "PriceUpdate", c, e)
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

		if to < from {
			// if the blockNumber is actually less than what we already, it might mean the backend is not synced or some reorg scenario
			continue
		}

		if to < tailSize {
			// in a test blockchain there might be not be enough blocks yet
			continue
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

		// this is called before processing the events
		// so that the eviction in batchstore gets the correct
		// block height context for the gc round. otherwise
		// expired batches might be "revived".
		err = updater.UpdateBlockNumber(to)
		if err != nil {
			return err
		}

		for _, e := range events {
			if err = l.processEvent(e, updater); err != nil {
				return err
			}
		}

		from = to + 1
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

// DiscoverAddresses returns the canonical contracts for this chainID
func DiscoverAddresses(chainID int64) (postageStamp, priceOracle common.Address, found bool) {
	if chainID == 5 {
		// goerli
		return common.HexToAddress("0xF7a041E7e2B79ccA1975852Eb6D4c6cE52986b4a"), common.HexToAddress("0x1044534090de6f4014ece6d036C699130Bd5Df43"), true
	}
	return common.Address{}, common.Address{}, false
}
