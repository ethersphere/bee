// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// SimpleJoinerJob encapsulates a single joiner operation, providing the consumer
// with blockwise reads of data represented by a content addressed chunk tree.
//
// Every chunk has a span length, which is a 64-bit integer in little-endian encoding
// stored as a prefix in the chunk itself. This represents the length of the data
// that reference represents.
//
// If a chunk's span length is greater than swarm.ChunkSize, the chunk will be treated
// as an intermediate chunk, meaning the contents of the chunk are handled as references
// to other chunks which in turn are retrieved.
//
// Otherwise it passes the data chunk to the io.Reader and blocks until the consumer reads
// the chunk.
//
// The process is repeated until the readCount reaches the announced spanLength of the chunk.
type SimpleJoinerJob struct {
	addr          swarm.Address
	ctx           context.Context
	getter        storage.Getter
	spanLength    int64         // the total length of data represented by the root chunk the job was initialized with.
	readCount     int64         // running count of chunks read by the io.Reader consumer.
	cursors       [9]int        // per-level read cursor of data.
	data          [9][]byte     // data of currently loaded chunk.
	dataC         chan []byte   // channel to pass data chunks to the io.Reader method.
	doneC         chan struct{} // channel to signal termination of join loop
	closeDoneOnce sync.Once     // make sure done channel is closed only once
	err           error         // read by the main thread to capture error state of the job
	logger        logging.Logger

	levels   int
	off      int64
	rootData []byte
}

// NewSimpleJoinerJob creates a new simpleJoinerJob.
func NewSimpleJoinerJob(ctx context.Context, getter storage.Getter, rootChunk swarm.Chunk) *SimpleJoinerJob {
	// spanLength is the overall  size of the entire data layer for this content addressed hash
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data()[:8])
	levelCount := file.Levels(int64(spanLength), swarm.SectionSize, swarm.Branches)
	j := &SimpleJoinerJob{
		addr:       rootChunk.Address(),
		ctx:        ctx,
		getter:     getter,
		spanLength: int64(spanLength),
		dataC:      make(chan []byte),
		doneC:      make(chan struct{}),
		logger:     logging.New(ioutil.Discard, 0),
		rootData:   rootChunk.Data()[8:],
	}

	// startLevelIndex is the root chunk level
	// data level has index 0
	startLevelIndex := levelCount - 1
	j.data[startLevelIndex] = rootChunk.Data()[8:]
	j.levels = levelCount

	return j
}

// Read is called by the consumer to retrieve the joined data.
// It must be called with a buffer equal to the maximum chunk size.
func (j *SimpleJoinerJob) Read(b []byte) (n int, err error) {
	//if cap(b) != swarm.ChunkSize {
	//return 0, fmt.Errorf("Read must be called with a buffer of %d bytes", swarm.ChunkSize)
	//}

	read, err := j.ReadAt(b, j.off)
	if err != nil && err != io.EOF {
		return read, err
	}

	j.off += int64(read)
	return read, err
}

func (j *SimpleJoinerJob) ReadAt(b []byte, off int64) (read int, err error) {
	return j.readAtOffset(b, j.rootData, 0, j.spanLength, off)
}

func (j *SimpleJoinerJob) readAtOffset(b, data []byte, cur, subTrieSize, off int64) (read int, err error) {

	if subTrieSize <= int64(len(data)) {
		ca := int64(cap(b))
		ii := off - cur
		if cd := int64(len(data)) - ii; ca > cd {
			ca = cd
		}

		// we are at a leaf
		bs := data[ii : ii+ca]
		copy(b, bs)
		return len(bs), nil
	}

	for cursor := 0; cursor < len(data); cursor += swarm.SectionSize {
		address := swarm.NewAddress(data[cursor : cursor+swarm.SectionSize])
		ch, err := j.getter.Get(j.ctx, storage.ModeGetRequest, address)
		if err != nil {
			return 0, err
		}

		var chunkData []byte
		chunkData = ch.Data()[8:]
		sp := int64(chunkSize(ch.Data()))
		// we have the size of the subtrie now, if the read offset is within this chunk,
		// then we drilldown more
		if off < cur+sp {
			return j.readAtOffset(b, chunkData, cur, sp, off)

		}
		cur += sp
	}
	return 0, errOffset
}

// completely analogous to standard SectionReader implementation
var errWhence = errors.New("seek: invalid whence")
var errOffset = errors.New("seek: invalid offset")

func (j *SimpleJoinerJob) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, errWhence
	case 0:
		offset += 0
	case 1:
		offset += j.off
	case 2:

		if j.rootData == nil { //seek from the end requires rootchunk for size. call Size first
			_, err := j.Size()
			if err != nil {
				return 0, fmt.Errorf("chunk size: %w", err)
			}
		}
		offset = j.spanLength - offset
		if offset < 0 {
			return 0, io.EOF
		}
	}

	if offset < 0 {
		return 0, errOffset
	}
	if offset > j.spanLength {
		return 0, io.EOF
	}
	j.off = offset
	fmt.Println("seek", j.off)
	return offset, nil

}

func (j *SimpleJoinerJob) Size() (int64, error) {
	if j.rootData == nil {
		chunk, err := j.getter.Get(context.TODO(), storage.ModeGetRequest, j.addr)
		if err != nil {
			return 0, err
		}
		j.rootData = chunk.Data()
	}

	s := chunkSize(j.rootData)

	return int64(s), nil
}

// Close is called by the consumer to gracefully abort the data retrieval.
func (j *SimpleJoinerJob) Close() error {
	j.closeDone()
	return nil
}

// closeDone, for purpose readability, wraps the sync.Once execution of closing the doneC channel
func (j *SimpleJoinerJob) closeDone() {
	j.closeDoneOnce.Do(func() {
		close(j.doneC)
	})
}

func chunkSize(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data[:8])
}
