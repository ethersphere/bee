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
	block := store.Block()
	e := &Events{
		store:  store,
		lis:    lis,
		logger: logger,
		quit:   make(chan struct{}),
	}
}

func (svc *BatchService) listen(from uint64, quit chan struct{}, update func(types.Log) error) {
	if err := e.lis.Listen(from, quit, svc); err != nil {
		e.logger.Errorf("error syncing batches with the blockchain: %v", err)
	}
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
