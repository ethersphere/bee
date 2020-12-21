package mock

import (
	"bytes"
	"math/big"

	"github.com/ethersphere/bee/pkg/postage"
)

var _ postage.BatchStorer = (*BatchStore)(nil)

// BatchStore is a mock BatchStorer
type BatchStore struct {
	cs     postage.ChainState
	id     []byte
	batch  postage.Batch
	getErr error
	putErr error
}

// Option is a an option passed to New
type Option func(*BatchStore)

// New creates a new mock BatchStore
func New(opts ...Option) *BatchStore {
	var bs BatchStore

	for _, o := range opts {
		o(&bs)
	}

	bs.batch.Value = big.NewInt(0)
	bs.cs.Price = big.NewInt(0)

	return &bs
}

// WithChainState the initial ChainStore
func WithChainState(cs *postage.ChainState) Option {
	return func(bs *BatchStore) {
		bs.cs = *cs
	}
}

// Get mocks the Get method from the BatchStore
func (bs *BatchStore) Get(id []byte) (*postage.Batch, error) {
	if !bytes.Equal(bs.id, id) {
		return nil, bs.getErr
	}
	return &bs.batch, nil
}

// Put mocks the Put method from the BatchStore
func (bs *BatchStore) Put(batch *postage.Batch) error {
	bs.batch = *batch
	bs.id = batch.ID
	return bs.putErr
}

// GetChainState mocks the GetChainState method from the BatchStore
func (bs *BatchStore) GetChainState() (*postage.ChainState, error) {
	return &bs.cs, bs.getErr
}

// PutChainState mocks the PutChainState method from the BatchStore
func (bs *BatchStore) PutChainState(cs *postage.ChainState) error {
	bs.cs = *cs
	return bs.putErr
}

// SetGetError will set the error returned on the next get
func (bs *BatchStore) SetGetError(err error) {
	bs.getErr = err
}

// SetPutError will set the error returned on the next put
func (bs *BatchStore) SetPutError(err error) {
	bs.putErr = err
}
