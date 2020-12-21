package testing

import (
	"math/big"
	"math/rand"

	"github.com/ethersphere/bee/pkg/postage"
)

// NewChainState will create a new ChainState with random values
func NewChainState() *postage.ChainState {
	var cs postage.ChainState

	cs.Block = rand.Uint64()
	cs.Price = (new(big.Int)).SetUint64(rand.Uint64())
	cs.Total = (new(big.Int)).SetUint64(rand.Uint64())

	return &cs
}
