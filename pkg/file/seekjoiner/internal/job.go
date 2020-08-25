// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"encoding/binary"
	"errors"
	"io"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type SimpleJoinerJob struct {
	addr       swarm.Address
	rootData   []byte
	spanLength int64
	off        int64
	levels     int
	refLength  int

	ctx    context.Context
	getter storage.Getter
}

// NewSimpleJoinerJob creates a new simpleJoinerJob.
func NewSimpleJoinerJob(ctx context.Context, getter storage.Getter, refLength int, rootChunk swarm.Chunk) *SimpleJoinerJob {
	// spanLength is the overall  size of the entire data layer for this content addressed hash
	spanLength := binary.LittleEndian.Uint64(rootChunk.Data()[:swarm.SpanSize])
	levelCount := file.Levels(int64(spanLength), swarm.SectionSize, swarm.Branches)
	j := &SimpleJoinerJob{
		addr:       rootChunk.Address(),
		refLength:  refLength,
		ctx:        ctx,
		getter:     getter,
		spanLength: int64(spanLength),
		rootData:   rootChunk.Data()[swarm.SpanSize:],
		levels:     levelCount,
	}

	return j
}

// Read is called by the consumer to retrieve the joined data.
// It must be called with a buffer equal to the maximum chunk size.
func (j *SimpleJoinerJob) Read(b []byte) (n int, err error) {
	read, err := j.ReadAt(b, j.off)
	if err != nil && err != io.EOF {
		return read, err
	}

	j.off += int64(read)
	return read, err
}

func (j *SimpleJoinerJob) ReadAt(b []byte, off int64) (read int, err error) {
	// since offset is int64 and swarm spans are uint64 it means we cannot seek beyond int64 max value
	return j.readAtOffset(b, j.rootData, 0, j.spanLength, off)
}

func (j *SimpleJoinerJob) readAtOffset(b, data []byte, cur, subTrieSize, off int64) (read int, err error) {
	if off >= j.spanLength {
		return 0, io.EOF
	}

	if subTrieSize <= int64(len(data)) {
		capacity := int64(cap(b))
		dataOffsetStart := off - cur
		dataOffsetEnd := dataOffsetStart + capacity

		if lenDataToCopy := int64(len(data)) - dataOffsetStart; capacity > lenDataToCopy {
			dataOffsetEnd = dataOffsetStart + lenDataToCopy
		}

		bs := data[dataOffsetStart:dataOffsetEnd]
		n := copy(b, bs)
		return n, nil
	}

	for cursor := 0; cursor < len(data); cursor += j.refLength {
		address := swarm.NewAddress(data[cursor : cursor+j.refLength])
		ch, err := j.getter.Get(j.ctx, storage.ModeGetRequest, address)
		if err != nil {
			return 0, err
		}

		chunkData := ch.Data()[8:]
		subtrieSpan := int64(chunkToSpan(ch.Data()))

		// we have the size of the subtrie now, if the read offset is within this chunk,
		// then we drilldown more
		if off < cur+subtrieSpan {
			return j.readAtOffset(b, chunkData, cur, subtrieSpan, off)

		}
		cur += subtrieSpan
	}

	return 0, errOffset
}

var errWhence = errors.New("seek: invalid whence")
var errOffset = errors.New("seek: invalid offset")

func (j *SimpleJoinerJob) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		offset += 0
	case 1:
		offset += j.off
	case 2:

		offset = j.spanLength - offset
		if offset < 0 {
			return 0, io.EOF
		}
	default:
		return 0, errWhence
	}

	if offset < 0 {
		return 0, errOffset
	}
	if offset > j.spanLength {
		return 0, io.EOF
	}
	j.off = offset
	return offset, nil

}

func (j *SimpleJoinerJob) Size() (int64, error) {
	if j.rootData == nil {
		chunk, err := j.getter.Get(j.ctx, storage.ModeGetRequest, j.addr)
		if err != nil {
			return 0, err
		}
		j.rootData = chunk.Data()
	}

	s := chunkToSpan(j.rootData)

	return int64(s), nil
}

func chunkToSpan(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data[:8])
}
