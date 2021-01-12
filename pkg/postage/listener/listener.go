package listener

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/postage"
)

var (
	createdTopic       = common.HexToHash("3f6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")
	topupTopic         = common.HexToHash("af5756c62d6c0722ef9be1f82bef97ab06ea5aea7f3eb8ad348422079f01d88d")
	depthIncreaseTopic = common.HexToHash("af27998ec15e9d3809edad41aec1b5551d8412e71bd07c91611a0237ead1dc8e")
)

type BlockHeightContractFilterer interface {
	bind.ContractFilterer
	BlockHeight(context.Context) (uint64, error)
}

type listener struct {
	ev BlockHeightContractFilterer
}

func New(ev BlockHeightContractFilterer) *listener {
	return &listener{ev: ev}
}

func (l *listener) Listen(from uint64, updater postage.EventUpdater) error {
	blockHeight, err := l.ev.BlockHeight(context.Background())
	if err != nil {
		return err
	}
	go l.catchUp(from, blockHeight, updater)
	return nil
}

func (l *listener) parseEvent(a abi.ABI, eventName string, c interface{}, e types.Log) error {
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
	abi.ParseTopics(c, indexed, e.Topics[1:])
	if err != nil {
		return err
	}
	return nil
}

func (l *listener) processEvent(a abi.ABI, e types.Log, updater postage.EventUpdater) error {
	eventSig := e.Topics[0]
	if eventSig == createdTopic {
		c := &batchCreatedEvent{}
		err := l.parseEvent(a, "BatchCreated", c, e)
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
	} else if eventSig == topupTopic {
		c := &batchTopUpEvent{}
		err := l.parseEvent(a, "BatchTopUp", c, e)
		if err != nil {
			return err
		}
		return updater.TopUp(
			c.BatchId[:],
			c.TopupAmount,
			c.NormalisedBalance,
		)
	} else if eventSig == depthIncreaseTopic {
		c := &batchDepthIncreaseEvent{}
		err := l.parseEvent(a, "BatchDepthIncrease", c, e)
		if err != nil {
			return err
		}
		return updater.UpdateDepth(
			c.BatchId[:],
			c.NewDepth,
			c.NormalisedBalance,
		)
	}
	return errors.New("unknown event")
}

func (l *listener) catchUp(from, to uint64, updater postage.EventUpdater) {
	ctx := context.Background()
	a := parseABI(Abi)

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(from)),
		ToBlock:   big.NewInt(int64(to)),
	}
	events, err := l.ev.FilterLogs(ctx, query)
	if err != nil {
		panic(err)
	}

	for _, e := range events {
		if err = l.processEvent(a, e, updater); err != nil {
			panic(err)
		}
	}
}

func (l *listener) Close() error {
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
