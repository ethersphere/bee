package postage

import (
	"math/big"
)

// EventUpdater interface definitions reflect the updates triggered by events emitted by
// the postage contract on the blockchain
type EventUpdater interface {
	Create(id []byte, owner []byte, amount *big.Int, depth uint8) error
	TopUp(id []byte, amount *big.Int) error
	UpdateDepth(id []byte, depth uint8) error
	UpdatePrice(price *big.Int) error
}

type BatchStorer interface {
	Get(id []byte) *postage.Batch
	Put(*postage.Batch) error
}

// Listener provides a blockchain event iterator
type Listener interface {
	// - it starts at block from
	// - it terminates  with no error when quit channel is closed
	// - if the update function returns an error, the call returns with that error
	Listen(from uint64, quit chan struct{}, update func(EventUpdater) error) error
}
