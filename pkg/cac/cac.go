package cac

import (
	"encoding/binary"

	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/swarm"
)

func New(data []byte) (swarm.Chunk, error) {
	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	_, err := hasher.Write(data)
	if err != nil {
		return nil, err
	}
	span := make([]byte, 8)
	binary.LittleEndian.PutUint64(span, uint64(len(data)))
	err = hasher.SetSpanBytes(span)
	if err != nil {
		return nil, err
	}

	return swarm.NewChunk(swarm.NewAddress(hasher.Sum(nil)), append(span, data...)), nil
}
