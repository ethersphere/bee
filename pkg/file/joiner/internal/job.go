package internal

import (
	"context"
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

func NewSimpleJoinerJob(ctx context.Context, store storage.Storer, spanLength int64, rootData []byte) *SimpleJoinerJob {
	levelCount := getLevelsFromLength(spanLength, swarm.SectionSize, swarm.Branches)
	j := &SimpleJoinerJob{
		ctx: ctx,
		store: store,
		spanLength: spanLength,
		levelCount: levelCount,
	}
	j.data[levelCount-1] = rootData
	return j
}


func (j *SimpleJoinerJob) Read(b []byte) (n int, err error) {
	return 0, nil
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
