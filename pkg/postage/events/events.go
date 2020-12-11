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
	"github.com/ethersphere/bee/pkg/postage/batchstore"
	"github.com/ethersphere/bee/pkg/storage"
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
		"BatchDepthIncrease(bytes32,uint256)",
		"PriceUpdate(uint256)",
	}
	eventNames = []string{
		"BatchCreated",
		"BatchTopUp",
		"BatchDepthIncrease",
		"PriceUpdate",
	}
)

// event BatchCreated(bytes32 indexed batchId, uint256 initialBalance, address owner, uint256 depth);
type batchCreatedEvent struct {
	batchID [32]byte
	balance *big.Int
	owner   common.Address
	depth   *big.Int
}

// event BatchTopUp(bytes32 indexed batchId, uint256 topupAmount);
type batchTopUpEvent struct {
	batchID [32]byte
	amount  *big.Int
}

// event BatchDepthIncrease(bytes32 indexed batchId, uint256 newDepth);
type batchDepthIncreaseEvent struct {
	batchID [32]byte
	depth   *big.Int
}

// event PriceUpdate(uint256 price);
type priceUpdateEvent struct {
	price *big.Int
}

func parse(log types.Log) postage.Event {
	sigdigest := log.Topics[0].Hex()
	switch signdigest {
	//batchCreatedEvent
	case contractEventDigests[0]:
		ev := &batchCreatedEvent{}
		if err := contractABI.Unpack(ev, eventNames[0], log.Data); err != nil {
			return nil
		}

	//batchTopUpEvent
	case contractEventDigests[1]:
		ev := &batchTopUpEvent{}
		if err := contractABI.Unpack(ev, eventNames[1], log.Data); err != nil {
			return nil
		}

	//batchDepthIncreaseEvent
	case contractEventDigests[2]:
		ev := &batchDepthIncreaseEvent{}
		if err := contractABI.Unpack(ev, eventNames[2], log.Data); err != nil {
			return nil
		}

	//priceUpdateEvent
	case contractEventDigests[3]:
		ev := &priceUpdateEvent{}
		if err := contractABI.Unpack(ev, eventNames[3], log.Data); err != nil {
			return nil
		}
	default:
	}
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

type Events struct {
	block uint64
	price *big.Int
	total *big.Int

	lis    postage.Listener
	store  *batchstore.Store
	logger logging.Logger

	quit chan struct{}
}

func New(lis postage.Listener, st storage.StateStorer, logger logging.Logger) (*Events, error) {
	store, err := batchstore.New(st)
	block := store.Block()
	e := &Events{
		store:  store,
		lis:    lis,
		logger: logger,
		quit:   make(chan struct{}),
	}
}

func (e *Events) Each(from uint64, f func(uint64, postage.Event) error) func() {
	update := func(ev types.Log) error {
		return f(ev.BlockNumber, parse(ev))
	}
	go e.listen(from, quit, update)
	return stop
}

func (e *Events) listen(from uint64, quit chan struct{}, update func(types.Log) error) {
	if err := e.lis.Listen(from, quit, update); err != nil {
		e.logger.Errorf("error syncing batches with the blockchain: %v", err)
	}
}

func (e *Events) Close() error {
	close(e.quit)
}

// Settle retrieves the current state
// - sets the cumulative outpayment normalised, cno+=price*period
// - sets the new block number
func (s *Events) Settle(block uint64) error {

	updatePeriod := int64(block - s.block)
	s.block = block
	s.total.Add(s.total, new(big.Int).Mul(s.price, big.NewInt(updatePeriod)))

	return s.store.Put(stateKey, s)
}
