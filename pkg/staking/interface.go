package staking

import (
	"context"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Listener provides a blockchain event iterator.
type Listener interface {
	io.Closer
	Listen(from uint64, updater EventUpdater, initState *ChainSnapshot) <-chan error
}

// ChainSnapshot represents the snapshot of all the staking events between the
// FirstBlockNumber and LastBlockNumber. The timestamp stores the time at which the
// snapshot was generated. This snapshot can be used to sync the staking package
// to prevent large no. of chain backend calls.
type ChainSnapshot struct {
	Events           []types.Log `json:"events"`
	LastBlockNumber  uint64      `json:"lastBlockNumber"`
	FirstBlockNumber uint64      `json:"firstBlockNumber"`
	Timestamp        int64       `json:"timestamp"`
}

type EventUpdater interface {
	DepositStake(overlay []byte, stakeAmount *big.Int, addr common.Address, lastUpdatedBlock *big.Int) error
	GetStake(ctx context.Context, overlay []byte) (*big.Int, error)
	UpdateBlockNumber(blockNumber uint64) error

	TransactionStart() error
	TransactionEnd() error
}
