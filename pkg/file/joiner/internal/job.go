package internal

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type SimpleJoinerJob struct {
	ctx context.Context
	store storage.Storer
	spanLength int64
	levelCount int
	readCount int64
	cursors [9]int
	data [9][]byte
	chunkC chan []byte
}

func NewSimpleJoinerJob(ctx context.Context, store storage.Storer, rootChunk swarm.Chunk) *SimpleJoinerJob {
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data()[:8])
	levelCount := getLevelsFromLength(int64(spanLength), swarm.SectionSize, swarm.Branches)
	j := &SimpleJoinerJob{
		ctx: ctx,
		store: store,
		spanLength: int64(spanLength),
		levelCount: levelCount,
		chunkC: make(chan []byte),
	}

	// keeping the data level as 0 index matches the file hasher solution
	j.data[levelCount-1] = rootChunk.Data()[8:]
	return j
}

func (j *SimpleJoinerJob) Read(b []byte) (n int, err error) {
	select {
	case b := <-j.chunkC:
		return len(b), nil
	case <-j.ctx.Done():
		return 0, j.ctx.Err()
	}
}

// calculate the last level index which a particular data section count will result in. The returned level will be the level of the root hash
func getLevelsFromLength(l int64, sectionSize int, branches int) int {
	s := int64(sectionSize)
	b := int64(branches)
	if l == 0 {
		return 0
	} else if l <= s*b {
		return 1
	}
	c := (l - 1) / s

	return int(math.Log(float64(c))/math.Log(float64(b)) + 1)
}
