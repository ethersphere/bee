package node

import (
	"context"
	"math/big"
)

type mockContract struct {
}

func (mockContract) CreateBatch(context.Context, *big.Int, uint8, bool, string) ([]byte, error) {
	return []byte("0x344"), nil
}
