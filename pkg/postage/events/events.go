package events

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
)

var (
	contractAddress      = common.HexToAddress("0x147B8eb97fD247D06C4006D269c90C1908Fb5D54")
	contractABI          = parseABI("{}")
	contractEventDigests = digestSig(eventSignatures)
)

var (
	eventSignatures = []string{
		"BatchCreated(bytes32,uint256,address,uint256)",
		"BatchTopUp(bytes32,uint256)",
		"BatchDepthIncrease(bytes32,uint256",
		"PriceUpdate(uint256)",
	}
	eventNames = []string{
		"BatchCreated",
		"BatchTopUp",
		"BatchDepthIncrease",
		"PriceUpdate",
	}
)

func eventTypes(i int) postage.Event {
	switch i {
	case 0:
		return &batchCreatedEvent{}
	case 1:
		return &batchTopUpEvent{}
	case 2:
		return &batchDepthIncreaseEvent{}
	case 3:
		return &priceUpdateEvent{}
	default:
		return nil
	}
}

// event BatchCreated(bytes32 indexed batchId, uint256 initialBalance, address owner, uint256 depth);
type batchCreatedEvent struct {
	batchID [32]byte
	balance *big.Int
	owner   common.Address
	depth   *big.Int
}

func (e *batchCreatedEvent) Update(s postage.EventUpdater) error {
	return s.Create(e.batchID[:], e.owner[:], e.balance, uint8(e.depth.Uint64()))
}

// event BatchTopUp(bytes32 indexed batchId, uint256 topupAmount);
type batchTopUpEvent struct {
	batchID [32]byte
	amount  *big.Int
}

func (e *batchTopUpEvent) Update(s postage.EventUpdater) error {
	return s.TopUp(e.batchID[:], e.amount)
}

// event BatchDepthIncrease(bytes32 indexed batchId, uint256 newDepth);
type batchDepthIncreaseEvent struct {
	batchID [32]byte
	depth   *big.Int
}

func (e *batchDepthIncreaseEvent) Update(s postage.EventUpdater) error {
	return s.UpdateDepth(e.batchID[:], uint8(e.depth.Uint64()))

}

// event PriceUpdate(uint256 price);
type priceUpdateEvent struct {
	price *big.Int
}

func (e *priceUpdateEvent) Update(s postage.EventUpdater) error {
	return s.UpdatePrice(e.price)
}

// parse reifies the event log type as a struct
//  weakly unsafe in that if there is another event, nil is returned
func parse(log types.Log) postage.Event {
	sigdigest := log.Topics[0].Hex()
	for i, digest := range contractEventDigests {
		if digest == sigdigest {
			ev := eventTypes(i)
			if err := contractABI.Unpack(ev, eventNames[i], log.Data); err != nil {
				return nil
			}
			return ev
		}
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

func digestSig(sigs []string) []string {
	digests := make([]string, 4)
	for i, s := range sigs {
		h, err := crypto.LegacyKeccak256([]byte(s))
		if err != nil {
			panic(fmt.Sprintf("error digesting signatures: %v", err))
		}
		digests[i] = hex.EncodeToString(h)
	}
	return digests
}

var _ postage.Events = (*Events)(nil)

// Events provides an iterator listening to postage events
type Events struct {
	lis    postage.Listener
	logger logging.Logger
}

// New is constructor for Events
func New(lis postage.Listener, logger logging.Logger) *Events {
	return &Events{lis, logger}
}

// Each starts the forever loop that keeps the batch Store in sync with the blockchain
// takes a postage.Listener interface as argument and uses it as an iterator
func (e Events) Each(from uint64, f func(uint64, postage.Event) error) func() {
	update := func(ev types.Log) error {
		return f(ev.BlockNumber, parse(ev))
	}
	quit := make(chan struct{})
	stop := func() { close(quit) }
	// Listener Listen call is the forever loop listening to blockchain events
	go e.listen(from, quit, update)
	return stop
}

func (e Events) listen(from uint64, quit chan struct{}, update func(types.Log) error) {
	if err := e.lis.Listen(from, quit, update); err != nil {
		e.logger.Errorf("error syncing batches with the blockchain: %v", err)
	}
}
