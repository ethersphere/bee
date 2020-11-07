package postage

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
)

// EventUpdater interface definitions reflect the updates triggered by events emitted by
// the postage contract on the blockchain
type EventUpdater interface {
	Create(id []byte, owner []byte, amount *big.Int, depth uint8) error
	TopUp(id []byte, amount *big.Int) error
	UpdateDepth(id []byte, depth uint8) error
	UpdatePrice(price *big.Int) error
}

// Event is the interface subsuming all postage contract blockchain events
//
// postage contract event  | golang Event              | Update call on EventUpdater
// ------------------------+---------------------------+---------------------------
// BatchCreated            | batchCreatedEvent         | Create
// BatchTopUp              | batchTopUpEvent           | TopUp
// BatchDepthIncrease      | batchDepthIncreaseEvent   | UpdateDepth
// PriceUpdate             | priceUpdateEvent          | UpdatePrice
type Event interface {
	Update(s EventUpdater) error
}

// Events provides an iterator for postage events
type Events interface {
	Each(from uint64, update func(block uint64, ev Event) error) func()
}

// Listener provides a blockchain event iterator
type Listener interface {
	// - it starts at block from
	// - it terminates  with no error when quit channel is closed
	// - if the update function returns an error, the call returns with that error
	Listen(from uint64, quit chan struct{}, update func(types.Log) error) error
}
