package localstore

import (
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

func New() *MockBatchStore {
	tbs := &MockBatchStore{}
	return tbs.WithGet(func(batchID []byte) (*postage.Batch, error) {
		return &postage.Batch{ID: batchID, Depth: swarm.MaxPO - 2}, nil
	})
}

// var _ BatchStore = (*MockBatchStore)(nil)

type MockBatchStore struct {
	getf func(batchID []byte) (*postage.Batch, error)
}

func (tbs *MockBatchStore) WithGet(getf func(batchID []byte) (*postage.Batch, error)) *MockBatchStore {
	tbs.getf = getf
	return tbs
}

func (tbs *MockBatchStore) Get(batchID []byte) (*postage.Batch, error) {
	return tbs.getf(batchID)
}

func (tbs *MockBatchStore) SubscribeToDepth() (sub <-chan uint8, cancel func()) {
	return nil, nil
}

func (tbs *MockBatchStore) SubscribeToExpiredBatches() (sub <-chan []byte, cancel func()) {
	return nil, nil
}
