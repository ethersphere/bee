package batchstore

import (
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

var (
	batchKeyPrefix = "batchKeyPrefix"
	valueKeyPrefix = "valueKeyPrefix"
)

var _ postage.EventUpdater = (*Store)(nil)

// Store is a local store  for postage batches
type Store struct {
	store  storage.StateStorer // state store backend to persist batches
	mu     sync.Mutex          // mutex to lock statestore during atomic changes
	cancel func()              // cancel sync and wait till done
	state  *state              // the current state
	logger logging.Logger
}

// New constructs a new postage batch store
func New(store storage.StateStorer, events postage.Events, logger logging.Logger) (*Store, error) {
	// initialise state from statestore or start with 0-s
	st := &state{}
	if err := st.load(store); err != nil {
		return nil, err
	}
	s := &Store{
		store:  store,
		logger: logger,
	}
	s.cancel = events.Each(st.block, s.update)
	return s, nil
}

// Close quits the sync routine and closes the statestore
func (s *Store) Close() error {
	s.cancel()
	return s.store.Close()
}

// update wraps around the update call for the specific event and
// abstracts the process shared across events
// - lock
// - settle = update cumulative outpayment normalised
// - update specific to event
// - save state
// - unlock
// this is the function that is given  to the listener iterator when synchronising
func (s *Store) update(block uint64, ev postage.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settle(block)
	if err := ev.Update(s); err != nil {
		return err
	}
	return s.state.save(s.store)
}

// settle retrieves the current state
// - sets the cumulative outpayment normalised, cno+=price*period
// - sets the new block number
// caller holds the store mutex
func (s *Store) settle(block uint64) {
	period := int64(block - s.state.block)
	s.state.block = block
	s.state.total.Add(s.state.total, new(big.Int).Mul(s.state.price, big.NewInt(period)))
}

//
func (s *Store) balance(b *postage.Batch, add *big.Int) (*big.Int, error) {
	return nil, nil
}

// batchKey returns the index key for the batch ID used in the by-ID batch index
func batchKey(id []byte) string {
	return batchKeyPrefix + string(id)
}

// valueKey returns the index key for the batch value used in the by-value (priority) batch index
func valueKey(v *big.Int) string {
	key := make([]byte, 32)
	value := v.Bytes()
	copy(key[32-len(value):], value)
	return valueKeyPrefix + string(key)
}

func (s *Store) get(id []byte) (*postage.Batch, error) {
	b := &postage.Batch{}
	err := s.store.Get(batchKey(id), b)
	return b, err
}

func (s *Store) put(b *postage.Batch) error {
	return s.store.Put(batchKey(b.ID), b)
}

func (s *Store) replace(id []byte, oldValue, newValue *big.Int) error {
	err := s.store.Delete(valueKey(oldValue))
	if err != nil {
		return err
	}
	return s.store.Put(valueKey(newValue), id)
}

func (s *Store) Create(id []byte, owner []byte, amount *big.Int, depth uint8) error {
	b := &postage.Batch{
		ID:    id,
		Start: s.state.block,
		Owner: owner,
		Depth: depth,
	}
	value, err := s.balance(b, amount)
	if err != nil {
		return err
	}
	err = s.replace(id, b.Value, value)
	if err != nil {
		return err
	}
	return s.put(b)
}

func (s *Store) TopUp(id []byte, amount *big.Int) error {
	b, err := s.get(id)
	if err != nil {
		return err
	}
	value, err := s.balance(b, amount)
	if err != nil {
		return err
	}
	err = s.replace(id, b.Value, value)
	if err != nil {
		return err
	}
	return s.put(b)
}

func (s *Store) UpdateDepth(id []byte, depth uint8) error {
	b, err := s.get(id)
	if err != nil {
		return err
	}
	b.Depth = depth
	return s.put(b)
}

func (s *Store) UpdatePrice(price *big.Int) error {
	s.state.price = price
	return nil
}
