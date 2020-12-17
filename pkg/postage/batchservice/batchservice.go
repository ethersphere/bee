package batchservice

import (
	"math/big"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
)

type BatchService struct{}

func New(st postage.BatchStorer, logger logging.Logger) (*Events, error) {
	store, err := batchstore.New(st)
}

func (s *BatchService) Create(id []byte, owner []byte, amount *big.Int, depth uint8) error {
	batch := &postage.Batch{
		//ID: id,
	}

	return s.store.Save()
}

func (s *BatchService) TopUp(id []byte, amount *big.Int) error {
	panic("not implemented") // TODO: Implement
}

func (s *BatchService) UpdateDepth(id []byte, depth uint8) error {
	panic("not implemented") // TODO: Implement
}

func (s *BatchService) UpdatePrice(price *big.Int) error {
	panic("not implemented") // TODO: Implement
}
