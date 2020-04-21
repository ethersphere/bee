package internal

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// SimpleJoinerJob encapsulates a single joiner operation.
//
// If a chunk's length is greater than swarm.ChunkSize it attempts
// to retrieve chunks using the chunk sections as chunk references.
// Otherwise it blocks until io.Reader consumer reads the chunk 
// before it fetches the next chunk.
type SimpleJoinerJob struct {
	ctx        context.Context
	store      storage.Storer
	spanLength int64 // the total length of data represented by the root chunk the job was initialized with.
	levelCount int // recursion level of the chunk tree. 
	readCount  int64 // running count of chunks read by the io.Reader consumer.
	cursors    [9]int // per-level read cursor of data.
	data       [9][]byte // data of currently loaded chunk.
	dataC      chan []byte // channel to pass data chunks to the io.Reader method.
	logger     logging.Logger
}

// NewSimpleJoinerJob creates a new simpleJoinerJob.
func NewSimpleJoinerJob(ctx context.Context, store storage.Storer, rootChunk swarm.Chunk) *SimpleJoinerJob {
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data()[:8])
	levelCount := getLevelsFromLength(int64(spanLength), swarm.SectionSize, swarm.Branches)

	j := &SimpleJoinerJob{
		ctx:        ctx,
		store:      store,
		spanLength: int64(spanLength),
		levelCount: levelCount,
		dataC:      make(chan []byte),
		logger:     logging.New(os.Stderr, 5),
	}

	// startLevelIndex is the root chunk level
	// data level has index 0
	startLevelIndex := levelCount-1
	j.data[startLevelIndex] = rootChunk.Data()[8:]

	// retrieval must be asynchronous to the io.Reader()
	go func() {
		err := j.start(startLevelIndex)
		if err != nil {
			// this will only already be closed if all the chunk data has been fully read
			// in this case the error will always be nil and this will not be executed
			j.logger.Errorf("error in process: %v", err)
			close(j.dataC)
		}
	}()

	return j
}

// start processes all chunk references of the root chunk that already has been retrieved.
func (j *SimpleJoinerJob) start(level int) error {

	for j.cursors[level] < len(j.data[level]) {
		// consume the reference at the current cursor position of the chunk level data
		// and start recursive retrieval down to the underlying data chunks
		err := j.nextReference(level)
		if err != nil {
			return err
		}
	}
	return nil
}

// nextReference gets the next chunk reference from the currently loaded chunk in a level
func (j *SimpleJoinerJob) nextReference(level int) error {
	data := j.data[level]
	cursor := j.cursors[level]
	chunkAddress := swarm.NewAddress(data[cursor : cursor+swarm.SectionSize])
	err := j.descend(level-1, chunkAddress)
	if err != nil {
		return err
	}

	// move the cursor to the next reference
	j.cursors[level] += swarm.SectionSize
	return nil
}

// descend is a recursive method that retrieves data chunks by resolving references
// in intermediate chunks.
// When a data chunk is found it is passed on the dataC channel to be consumed by the
// io.Reader consumer.
// 
// TODO: goroutine leaks if context is cancelled.
func (j *SimpleJoinerJob) descend(level int, address swarm.Address) error {

	// attempt to retrieve the chunk
	j.logger.Debugf("next chunk get: %v", address)
	ch, err := j.store.Get(j.ctx, storage.ModeGetRequest, address)
	if err != nil {
		return err
	}

	// any level higher than 0 means the chunk contains references
	// which must be recursively processed
	if level > 0 {
		for j.cursors[level] < len(j.data[level]) {
			if len(j.data[level]) == j.cursors[level] {
				j.data[level] = ch.Data()[8:]
				j.cursors[level] = 0
			}
			err := j.nextReference(level)
			if err != nil {
				return err
			}
		}
	} else {
		// close the channel if we have read all data
		data := ch.Data()[8:]
		j.dataC <- data
		j.readCount += int64(len(data))
		if j.readCount == j.spanLength {
			close(j.dataC)
		}
	}
	return nil
}

// Read is called by the consumer to retrieve the joined data.
// It must be called with a buffer equal to the maximum chunk size.
func (j *SimpleJoinerJob) Read(b []byte) (n int, err error) {
	if cap(b) != swarm.ChunkSize {
		return 0, fmt.Errorf("Read must be called with a buffer of %d bytes", swarm.ChunkSize)
	}
	select {
	case data, ok := <-j.dataC:
		if !ok {
			return 0, io.EOF
		}
		copy(b, data)
		return len(b), nil
	case <-j.ctx.Done():
		return 0, j.ctx.Err()
	}
}

// getLevelsFromLength calculates the last level index which a particular data section count will result in.
// The returned level will be the level of the root hash.
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
