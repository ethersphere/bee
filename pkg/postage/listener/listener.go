package listener

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/postage"
)

var eventDigests = []string{
	"e5b2abc48304a47f30897defa3174e96a7290de11c52e8c27229f85bcc929be3",
	"a8c128cf3a23d40c5ad64da7f5a25e4db463e2384fd4a5a1688f944920e19f12",
	"a9f8a6e6923e0cc412b0bc9fa5a70b1ae52f0216f301d28a2d0f66b91c8252b0",
	"ae46785019700e30375a5d7b4f91e32f8060ef085111f896ebf889450aa2ab5a",
}

var createdTopic = common.HexToHash("3f6ec1ed9250a6952fabac07c6eb103550dc65175373eea432fd115ce8bb2246")
var functionSigs = []string{
	"BatchCreated(bytes32,uint256,uint256,address,uint8)",
	"BatchTopUp(bytes32,uint256)",
	"BatchDepthIncrease(bytes32,uint256)",
	"PriceUpdate(uint256)",
}

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
	go l.catchUp(from, blockHeight)
	return nil
}

func (l *listener) catchUp(from, to uint64) {
	ctx := context.Background()
	a := parseABI(Abi)
	for {
		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(int64(from)),
			ToBlock:   big.NewInt(int64(to)),
		}
		events, err := l.ev.FilterLogs(ctx, query)
		if err != nil {
			panic(err)
		}
		for _, e := range events {
			if bytes.Equal(e.Topics[0][:], createdTopic[:]) {
				c := &batchCreatedEvent{}
				err := a.Unpack(c, "BatchCreated", e.Data)
				var indexed abi.Arguments
				for _, arg := range a.Events["BatchCreated"].Inputs {
					if arg.Indexed {
						indexed = append(indexed, arg)
					}
				}
				abi.ParseTopics(c, indexed, e.Topics[1:])
				if err != nil {
					panic(err)
				}
			}
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
	BatchID           [32]byte
	TotalAmount       *big.Int
	NormalisedBalance *big.Int
	Owner             common.Address
	Depth             uint8
}

type batchTopUpEvent struct {
	batchID [32]byte
	amount  *big.Int
}

type batchDepthIncreaseEvent struct {
	batchID [32]byte
	depth   *big.Int
}

type priceUpdateEvent struct {
	price *big.Int
}
