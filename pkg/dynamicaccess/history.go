package dynamicaccess

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/kvs"
)

type History interface {
	Add(timestamp int64, kvs kvs.KeyValueStore) error
	Get(timestamp int64) (kvs.KeyValueStore, error)
	Lookup(at int64) (kvs.KeyValueStore, error)
}

var _ History = (*history)(nil)

type history struct {
	history map[int64]*kvs.KeyValueStore
}

func NewHistory(topic []byte, owner common.Address) *history {
	return &history{history: make(map[int64]*kvs.KeyValueStore)}
}

func (h *history) Add(timestamp int64, kvs kvs.KeyValueStore) error {

	return nil
}

func (h *history) Lookup(at int64) (kvs.KeyValueStore, error) {
	return nil, nil
}

func (h *history) Get(timestamp int64) (kvs.KeyValueStore, error) {
	// get the feed
	return nil, nil
}
