package testing

import (
	"encoding/binary"
	"math/rand"

	"github.com/ethersphere/bee/pkg/swarm"
)

func GenerateTestRandomFileChunk(address swarm.Address, spanLength int, dataSize int) swarm.Chunk {
	data := make([]byte, dataSize+8)
	binary.LittleEndian.PutUint64(data, uint64(spanLength))
	_, _ = rand.Read(data[8:]) // # skipcq: GSC-G404
	key := make([]byte, swarm.SectionSize)
	if address.IsZero() {
		_, _ = rand.Read(key) // # skipcq: GSC-G404
	} else {
		copy(key, address.Bytes())
	}
	return swarm.NewChunk(swarm.NewAddress(key), data)
}
