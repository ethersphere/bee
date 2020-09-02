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

	if subTrieSize <= int64(len(data)) {
		dataOffsetStart := off - cur
		dataOffsetEnd := dataOffsetStart + bytesToRead

		if lenDataToCopy := int64(len(data)) - dataOffsetStart; bytesToRead > lenDataToCopy {
			dataOffsetEnd = dataOffsetStart + lenDataToCopy
		}

		bs := data[dataOffsetStart:dataOffsetEnd]
		n := copy(b[bufferOffset:], bs)
		wg.Done()
		return n, nil
	}

	btr := bytesToRead
	fmt.Println("btr start", btr)
	for cursor := 0; cursor < len(data); cursor += j.refLength {
		if btr == 0 {
			break
		}
		address := swarm.NewAddress(data[cursor : cursor+j.refLength])
		ch, err := j.getter.Get(j.ctx, storage.ModeGetRequest, address)
		if err != nil {
			return 0, err
		}

		chunkData := ch.Data()[8:]
		subtrieSpan := int64(chunkToSpan(ch.Data()))
		fmt.Println("offset", off, "btr", btr, "current subtrie size", subtrieSpan, "cur", cur)

		// if requested offset is within this subtrie
		if off < cur+subtrieSpan {
			//subtrieSpan is how many bytes we CAN read
			// due to the fact we are launching a new goroutine(s), we lose
			// the original offset since we need to fetch multiple parts
			// simultaneously. therefore we need to call the relative offset now
			// with the new goroutine.
			//how many bytes should we ask for with this call?
			fmt.Println("howmany = span - (off - cur)", subtrieSpan, off, cur)
			howMany := subtrieSpan - (off - cur)

			// less than subtrieSpan
			if howMany > btr {
				howMany = btr
			}
			if howMany > subtrieSpan {
				howMany = subtrieSpan
			}

			fmt.Println("btr read another", btr, "bytes", "how many now?", howMany, "bufferOffset", bufferOffset)
			wg.Add(1)
			go j.readAtOffset(b, chunkData, cur, subtrieSpan, off, bufferOffset, howMany, wg)
			bufferOffset += howMany
			btr -= howMany //how many left
			cur += subtrieSpan
			off = cur
			continue
		}
		cur += subtrieSpan
	}
	wg.Done()
	wg.Wait()
	return int(bytesToRead), nil
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
