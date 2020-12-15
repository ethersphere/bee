package events

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore"
)

type BatchService struct {
	total *state
}

func New(lis postage.Listener, st postage.BatchStorer, logger logging.Logger) (*Events, error) {
	store, err := batchstore.New(st)
}

func (s *BatchService) Create(id []byte, owner []byte, amount *big.Int, depth uint8) error {
	batch := &postage.Batch{
		ID: id,
		....
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

func (e *Events) Close() error {
	close(e.quit)
}

// Settle retrieves the current state
// - sets the cumulative outpayment normalised, cno+=price*period
// - sets the new block number
func (s *Events) Settle(block uint64) error {
	updatePeriod := int64(block - s.block)
	s.block = block
	s.total.Add(s.total, new(big.Int).Mul(s.price, big.NewInt(updatePeriod)))

	return s.store.Put(stateKey, s)
}

func TestTopUp() {

	mockBatchStore := newMockBatchStore()
	batchService := New()

	batchService.TopUp()
}
