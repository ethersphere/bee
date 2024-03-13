package dynamicaccess

import (
	"github.com/ethereum/go-ethereum/common"
)

type History interface {
	Add(timestamp int64, act Act) error
	Get(timestamp int64) (Act, error)
	Lookup(at int64) (Act, error)
}

var _ History = (*history)(nil)

type history struct {
	history map[int64]*Act
}

func NewHistory(topic []byte, owner common.Address) *history {
	return &history{history: make(map[int64]*Act)}
}

func (h *history) Add(timestamp int64, act Act) error {

	return nil
}

func (h *history) Lookup(at int64) (Act, error) {
	return nil, nil
}

func (h *history) Get(timestamp int64) (Act, error) {
	// get the feed
	return nil, nil
}
