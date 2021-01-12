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

type BlockHeightContractFilterer interface {
	bind.ContractFilterer
	BlockHeight(context.Context) (uint64, error)
}

type listener struct {
	ev                      BlockHeightContractFilterer
	abi                     abi.ABI
	batchCreatedTopic       common.Hash
	batchTopupTopic         common.Hash
	batchDepthIncreaseTopic common.Hash
}

func New(ev BlockHeightContractFilterer) *listener {
	abi := parseABI(Abi)
	return &listener{
		ev:                      ev,
		abi:                     abi,
		batchCreatedTopic:       abi.Events["BatchCreated"].ID,
		batchTopupTopic:         abi.Events["BatchTopUp"].ID,
		batchDepthIncreaseTopic: abi.Events["BatchDepthIncrease"].ID,
	}
}

func (l *listener) Listen(from uint64, updater postage.EventUpdater) error {
	blockHeight, err := l.ev.BlockHeight(context.Background())
	if err != nil {
		return err
	}
	go l.catchUp(from, blockHeight, updater)
	return nil
}

func (l *listener) parseEvent(eventName string, c interface{}, e types.Log) error {
	err := l.abi.Unpack(c, eventName, e.Data)
	if err != nil {
		return err
	}

	var indexed abi.Arguments
	for _, arg := range l.abi.Events[eventName].Inputs {
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

func (l *listener) processEvent(e types.Log, updater postage.EventUpdater) error {
	eventSig := e.Topics[0]
	switch eventSig {
	case l.batchCreatedTopic:
		c := &batchCreatedEvent{}
		err := l.parseEvent("BatchCreated", c, e)
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
		err := l.parseEvent("BatchTopUp", c, e)
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
		err := l.parseEvent("BatchDepthIncrease", c, e)
		if err != nil {
			return err
		}
		return updater.UpdateDepth(
			c.BatchId[:],
			c.NewDepth,
			c.NormalisedBalance,
		)
	default:
		return errors.New("unknown event")
	}
}

func (l *listener) catchUp(from, to uint64, updater postage.EventUpdater) {
	ctx := context.Background()

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(from)),
		ToBlock:   big.NewInt(int64(to)),
	}
	events, err := l.ev.FilterLogs(ctx, query)
	if err != nil {
		panic(err)
	}

	for _, e := range events {
		if err = l.processEvent(e, updater); err != nil {
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
