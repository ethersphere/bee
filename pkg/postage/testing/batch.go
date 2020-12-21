package testing

import (
	crand "crypto/rand"
	"io"
	"math/big"
	"math/rand"

	"github.com/ethersphere/bee/pkg/postage"
)

// BatchOption is an optional parameter for NewBatch
type BatchOption func(c *postage.Batch)

// NewBatch will create a new test batch. Fields that are not supplied will
// be filled with random data
func NewBatch(opts ...BatchOption) (*postage.Batch, error) {
	var b postage.Batch

	for _, opt := range opts {
		opt(&b)
	}

	if b.ID == nil {
		id := make([]byte, 32)
		_, err := io.ReadFull(crand.Reader, id)
		if err != nil {
			return nil, err
		}
		b.ID = id
	}
	if b.Value == nil {
		b.Value = (new(big.Int)).SetUint64(rand.Uint64())

	}
	if b.Value == nil {
		b.Start = rand.Uint64()
	}
	if b.Owner == nil {
		owner := make([]byte, 20)
		_, err := io.ReadFull(crand.Reader, owner)
		if err != nil {
			return nil, err
		}
		b.Owner = owner
	}
	if b.Depth == 0 {
		b.Depth = 16

	}

	return &b, nil
}

// WithOwner will set the batch owner
func WithOwner(owner []byte) BatchOption {
	return func(b *postage.Batch) {
		b.Owner = owner
	}
}
