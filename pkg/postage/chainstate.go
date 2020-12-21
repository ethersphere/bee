package postage

import "math/big"

// ChainState contains data the batch service reads from the chain
type ChainState struct {
	Block uint64   `json:"block"` // The block number of the last postage event
	Total *big.Int `json:"total"` // Cumulative amount paid per stamp
	Price *big.Int `json:"price"` // Bzz/chunk/block normalised price
}
