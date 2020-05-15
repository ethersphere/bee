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
}

// NewSimpleJoinerJob creates a new simpleJoinerJob.
func NewSimpleJoinerJob(ctx context.Context, getter storage.Getter, rootChunk swarm.Chunk) *SimpleJoinerJob {
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data()[:8])
	levelCount := file.Levels(int64(spanLength), swarm.SectionSize, swarm.Branches)

	j := &SimpleJoinerJob{
		ctx:        ctx,
		getter:     getter,
		spanLength: int64(spanLength),
		dataC:      make(chan []byte),
		doneC:      make(chan struct{}),
		logger:     logging.New(ioutil.Discard, 0),
	}

	// startLevelIndex is the root chunk level
	// data level has index 0
	startLevelIndex := levelCount - 1
	j.data[startLevelIndex] = rootChunk.Data()[8:]

	// retrieval must be asynchronous to the io.Reader()
	go func() {
		err := j.start(startLevelIndex)
		if err != nil {
			// this will only already be closed if all the chunk data has been fully read
			// in this case the error will always be nil and this will not be executed
			if err != io.EOF {
				j.logger.Errorf("simple joiner chunk join job fail: %v", err)
			}
		}
		j.err = err
		close(j.dataC)
		j.closeDone()
	}()

	return j
}

// start processes all chunk references of the root chunk that already has been retrieved.
func (j *SimpleJoinerJob) start(level int) error {

	// consume the reference at the current cursor position of the chunk level data
	// and start recursive retrieval down to the underlying data chunks
	for j.cursors[level] < len(j.data[level]) {
		err := j.nextReference(level)
		if err != nil {
			return err
		}
	}
	return nil
}

// nextReference gets the next chunk reference at the cursor of the chunk currently loaded
// for the specified level.
func (j *SimpleJoinerJob) nextReference(level int) error {
	data := j.data[level]
	cursor := j.cursors[level]
	chunkAddress := swarm.NewAddress(data[cursor : cursor+swarm.SectionSize])
	err := j.nextChunk(level-1, chunkAddress)
	if err != nil {
		if err == io.EOF {
			return err
		}
		// if the last write is a "dangling chunk" the data chunk will have been moved
		// to an intermediate level. In this edge case, the error must be suppressed,
		// and the cursor manually to data length boundary to terminate the loop in
		// the calling frame.
		if j.readCount+int64(len(data)) == j.spanLength {
			j.cursors[level] = len(j.data[level])
			err = j.sendChunkToReader(data)
			return err
		}
		return fmt.Errorf("error in join for chunk %v: %v", chunkAddress, err)
	}

	// move the cursor to the next reference
	j.cursors[level] += swarm.SectionSize
	return nil
}

// nextChunk retrieves data chunks by resolving references in intermediate chunks.
// The method will be called recursively via the nextReference method when
// the current chunk is an intermediate chunk.
// When a data chunk is found it is passed on the dataC channel to be consumed by the
// io.Reader consumer.
func (j *SimpleJoinerJob) nextChunk(level int, address swarm.Address) error {

	// attempt to retrieve the chunk
	ch, err := j.getter.Get(j.ctx, storage.ModeGetRequest, address)
	if err != nil {
		return err
	}
	j.cursors[level] = 0
	j.data[level] = ch.Data()[8:]

	// any level higher than 0 means the chunk contains references
	// which must be recursively processed
	if level > 0 {
		for j.cursors[level] < len(j.data[level]) {
			if len(j.data[level]) == j.cursors[level] {
				j.data[level] = ch.Data()[8:]
				j.cursors[level] = 0
			}
			err = j.nextReference(level)
			if err != nil {
				return err
			}
		}
	} else {
		// read data and pass to reader only if session is still active
		// * context cancelled when client has disappeared, timeout etc
		// * doneC receive when gracefully terminated through Close
		data := ch.Data()[8:]
		err = j.sendChunkToReader(data)
	}
	return err
}

// sendChunkToReader handles exceptions on the part of consumer in
// the reading of data
func (j *SimpleJoinerJob) sendChunkToReader(data []byte) error {
	select {
	case <-j.ctx.Done():
		j.readCount = j.spanLength
		return j.ctx.Err()
	case <-j.doneC:
		return file.NewAbortError(errors.New("chunk read aborted"))
	case j.dataC <- data:
		j.readCount += int64(len(data))
		// when we reach the end of data to be read
		// bubble io.EOF error to the gofunc in the
		// constructor that called start()
		if j.readCount == j.spanLength {
			return io.EOF
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
	data, ok := <-j.dataC
	if !ok {
		<-j.doneC
		return 0, j.err
	}
	copy(b, data)
	return len(data), nil
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
