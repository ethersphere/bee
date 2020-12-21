package mock

import "github.com/ethersphere/bee/pkg/postage"

// BatchStore is a mock BatchStorer
type BatchStore struct {
}

// New creates a new mock BatchStore
func New() postage.BatchStorer {
	return &BatchStore{}
}

// Get mocks the Get method from the BatchStore
func (bs *BatchStore) Get(id []byte) (*postage.Batch, error) {
	panic("not implemented")
}

// Put mocks the Put method from the BatchStore
func (bs *BatchStore) Put(*postage.Batch) error {
	panic("not implemented")
}

// PutChainState mocks the PutChainState method from the BatchStore
func (bs *BatchStore) PutChainState(*postage.ChainState) error {
	panic("not implemented")
}

// GetChainState mocks the GetChainState method from the BatchStore
func (bs *BatchStore) GetChainState() *postage.ChainState {
	panic("not implemented")
}
