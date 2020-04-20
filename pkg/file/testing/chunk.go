package testing

import (
	"encoding/binary"
	"math/rand"

	"github.com/ethersphere/bee/pkg/swarm"
)

func GenerateTestRandomFileChunk(length int) swarm.Chunk {
	dataSize := length/swarm.Branches
	data := make([]byte, dataSize+8)
	binary.LittleEndian.PutUint64(data, uint64(length))
	_, _ = rand.Read(data[8:])
	key := make([]byte, swarm.SectionSize)
	_, _ = rand.Read(key)
	return swarm.NewChunk(swarm.NewAddress(key), data)
}
