package mock

import (
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type mockStamper struct{}

func NewStamper() postage.Stamper {
	return &mockStamper{}
}

func (mockStamper) Stamp(_ swarm.Address) (*postage.Stamp, error) {
	return &postage.Stamp{}, nil
}
