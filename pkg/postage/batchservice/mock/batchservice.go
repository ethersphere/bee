package mock

import (
	"math/big"
)

// BatchService mocks the postage.EventUpdater interface
type BatchService struct {
}

// NewBatchService creates a new mock BatchService
func NewBatchService() *BatchService {
	return &BatchService{}
}

// Create mocks the Create function on BatchService
func (bs *BatchService) Create(id []byte, owner []byte, amount *big.Int, depth uint8) error {
	panic("not implemented")
}

// TopUp mocks the TopUp function on BatchService
func (bs *BatchService) TopUp(id []byte, amount *big.Int) error {
	panic("not implemented")
}

// UpdateDepth mocks the UpdateDepth function on BatchService
func (bs *BatchService) UpdateDepth(id []byte, depth uint8) error {
	panic("not implemented")
}

// UpdatePrice mocks the UpdatePrice function on BatchService
func (bs *BatchService) UpdatePrice(price *big.Int) error {
	panic("not implemented")
}
