// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type SimpleJoiner struct {
	addr      swarm.Address
	rootData  []byte
	span      int64
	off       int64
	refLength int

	ctx    context.Context
	getter storage.Getter
}

// NewSimpleJoiner creates a new SimpleJoiner.
func NewSimpleJoiner(ctx context.Context, getter storage.Getter, address swarm.Address) (*SimpleJoiner, int64, error) {
	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := getter.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		return nil, 0, err
	}

	var chunkData = rootChunk.Data()

	span := int64(binary.LittleEndian.Uint64(chunkData[:swarm.SpanSize]))

	j := &SimpleJoiner{
		addr:      rootChunk.Address(),
		refLength: len(address.Bytes()),
		ctx:       ctx,
		getter:    getter,
		span:      span,
		rootData:  chunkData[swarm.SpanSize:],
	}

	return j, span, nil
}

// Read is called by the consumer to retrieve the joined data.
// It must be called with a buffer equal to the maximum chunk size.
func (j *SimpleJoiner) Read(b []byte) (n int, err error) {
	read, err := j.ReadAt(b, j.off)
	if err != nil && err != io.EOF {
		return read, err
	}

	j.off += int64(read)
	return read, err
}

func (j *SimpleJoiner) ReadAt(b []byte, off int64) (read int, err error) {
	// since offset is int64 and swarm spans are uint64 it means we cannot seek beyond int64 max value
	readLen := int64(cap(b))
	if readLen > j.span-off {
		readLen = j.span - off
	}
	var wg sync.WaitGroup
	wg.Add(1)
	return j.readAtOffset(b, j.rootData, 0, j.span, off, 0, readLen, &wg)
}

func (j *SimpleJoiner) readAtOffset(b, data []byte, cur, subTrieSize, off, bufferOffset, bytesToRead int64, wg *sync.WaitGroup) (read int, err error) {
	if off >= j.span {
		return 0, io.EOF
	}
	btr := bytesToRead

	if subTrieSize <= int64(len(data)) {
		dataOffsetStart := off - cur
		dataOffsetEnd := dataOffsetStart + bytesToRead

		if lenDataToCopy := int64(len(data)) - dataOffsetStart; bytesToRead > lenDataToCopy {
			dataOffsetEnd = dataOffsetStart + lenDataToCopy
		}

		bs := data[dataOffsetStart:dataOffsetEnd]
		n := copy(b[bufferOffset:bufferOffset+int64(len(bs))], bs)
		wg.Done()
		return n, nil
	}

	// treat the root hash case:
	// let's assume we have a trie of size x
	// then we can assume that at least all of the forks
	// except for the last one on the right are of equal size
	// this is due to how the splitter wraps levels - ie in the process
	// of chunking, it may be that the last address of each fork is not a whole level
	// so for the branches on the left, we can assume that
	// y = (branching factor - 1) * x + l
	// where y is the size of the subtrie, x is constant and l is the size of the last subtrie
	// we know how many refs we have in the current chunk

	for cursor := 0; cursor < len(data); cursor += j.refLength {
		if bytesToRead == 0 {
			break
		}
		// fast forward the cursor
		sec := subtrieSection(data, cursor, j.refLength, subTrieSize)
		if cur+sec < off {
			cur += sec
			continue
		}

		// if we are here it means that either we are within the bounds of the data we need to read
		address := swarm.NewAddress(data[cursor : cursor+j.refLength])
		subtrieSpan := sec
		currentReadSize := subtrieSpan - (off - cur) // the size of the subtrie, minus the offset from the start of the trie

		// upper bound alignments
		if currentReadSize > bytesToRead {
			currentReadSize = bytesToRead
		}
		if currentReadSize > subtrieSpan {
			currentReadSize = subtrieSpan
		}

		wg.Add(1)
		go func(address swarm.Address, b []byte, cur, subTrieSize, off, bufferOffset, bytesToRead int64) {
			ch, err := j.getter.Get(j.ctx, storage.ModeGetRequest, address)
			if err != nil {
				return
			}

			chunkData := ch.Data()[8:]
			subtrieSpan := int64(chunkToSpan(ch.Data()))
			wg.Add(1)
			go func(b, data []byte, cur, subTrieSize, off, bufferOffset, bytesToRead int64, wg *sync.WaitGroup) {
				_, _ = j.readAtOffset(b, chunkData, cur, subtrieSpan, off, bufferOffset, currentReadSize, wg)
			}(b, chunkData, cur, subtrieSpan, off, bufferOffset, currentReadSize, wg)
			wg.Done()
		}(address, b, cur, subtrieSpan, off, bufferOffset, currentReadSize)

		bufferOffset += currentReadSize
		bytesToRead -= currentReadSize
		cur += subtrieSpan
		off = cur
	}

	wg.Done()
	wg.Wait()
	return int(btr), nil
}

// brute-forces the subtrie size for each of the sections in this intermediate chunk
func subtrieSection(data []byte, startIdx, refLen int, subtrieSize int64) int64 {
	var (
		refs       = int64(len(data) / refLen) // how many references in the intermediate chunk
		branching  = int64(4096 / refLen)      // branching factor is chunkSize divided by reference length
		branchSize = int64(4096)
	)
	for {
		whatsLeft := subtrieSize - (branchSize * (refs - 1))
		if whatsLeft <= branchSize {
			break
		}
		branchSize *= branching
	}

	// handle last branch edge case
	if startIdx == int(refs-1)*refLen {
		return subtrieSize - (refs-1)*branchSize
	}
	return branchSize
}

var errWhence = errors.New("seek: invalid whence")
var errOffset = errors.New("seek: invalid offset")

func (j *SimpleJoiner) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		offset += 0
	case 1:
		offset += j.off
	case 2:

		offset = j.span - offset
		if offset < 0 {
			return 0, io.EOF
		}
	default:
		return 0, errWhence
	}

	if offset < 0 {
		return 0, errOffset
	}
	if offset > j.span {
		return 0, io.EOF
	}
	j.off = offset
	return offset, nil

}

func (j *SimpleJoiner) Size() (int64, error) {
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
