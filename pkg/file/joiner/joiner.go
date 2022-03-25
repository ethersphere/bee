// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implementations of the file.Joiner interface
package joiner

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/pkg/encryption/store"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

type joiner struct {
	addr      swarm.Address
	rootData  []byte
	span      int64
	off       int64
	refLength int

	ctx    context.Context
	getter storage.Getter
}

// New creates a new Joiner. A Joiner provides Read, Seek and Size functionalities.
func New(ctx context.Context, getter storage.Getter, address swarm.Address) (file.Joiner, int64, error) {
	getter = store.New(getter)
	// retrieve the root chunk to read the total data length the be retrieved
	rootChunk, err := getter.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		return nil, 0, err
	}

	var chunkData = rootChunk.Data()

	span := int64(binary.LittleEndian.Uint64(chunkData[:swarm.SpanSize]))

	j := &joiner{
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
func (j *joiner) Read(b []byte) (n int, err error) {
	read, err := j.ReadAt(b, j.off)
	if err != nil && err != io.EOF {
		return read, err
	}

	j.off += int64(read)
	return read, err
}

func (j *joiner) ReadAt(buffer []byte, off int64) (read int, err error) {
	// since offset is int64 and swarm spans are uint64 it means we cannot seek beyond int64 max value
	if off >= j.span {
		return 0, io.EOF
	}

	readLen := int64(cap(buffer))
	if readLen > j.span-off {
		readLen = j.span - off
	}
	var bytesRead int64
	var eg errgroup.Group
	j.readAtOffset(buffer, j.rootData, 0, j.span, off, 0, readLen, &bytesRead, &eg)

	err = eg.Wait()
	if err != nil {
		return 0, err
	}

	return int(atomic.LoadInt64(&bytesRead)), nil
}

var ErrMalformedTrie = errors.New("malformed tree")

func (j *joiner) readAtOffset(b, data []byte, cur, subTrieSize, off, bufferOffset, bytesToRead int64, bytesRead *int64, eg *errgroup.Group) {
	// we are at a leaf data chunk
	if subTrieSize <= int64(len(data)) {
		dataOffsetStart := off - cur
		dataOffsetEnd := dataOffsetStart + bytesToRead

		if lenDataToCopy := int64(len(data)) - dataOffsetStart; bytesToRead > lenDataToCopy {
			dataOffsetEnd = dataOffsetStart + lenDataToCopy
		}

		bs := data[dataOffsetStart:dataOffsetEnd]
		n := copy(b[bufferOffset:bufferOffset+int64(len(bs))], bs)
		atomic.AddInt64(bytesRead, int64(n))
		return
	}

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

		// if we are here it means that we are within the bounds of the data we need to read
		address := swarm.NewAddress(data[cursor : cursor+j.refLength])

		subtrieSpan := sec
		subtrieSpanLimit := sec

		currentReadSize := subtrieSpan - (off - cur) // the size of the subtrie, minus the offset from the start of the trie

		// upper bound alignments
		if currentReadSize > bytesToRead {
			currentReadSize = bytesToRead
		}
		if currentReadSize > subtrieSpan {
			currentReadSize = subtrieSpan
		}

		func(address swarm.Address, b []byte, cur, subTrieSize, off, bufferOffset, bytesToRead, subtrieSpanLimit int64) {
			eg.Go(func() error {
				ch, err := j.getter.Get(j.ctx, storage.ModeGetRequest, address)
				if err != nil {
					return err
				}

				chunkData := ch.Data()[8:]
				subtrieSpan := int64(chunkToSpan(ch.Data()))

				if subtrieSpan > subtrieSpanLimit {
					return ErrMalformedTrie
				}

				j.readAtOffset(b, chunkData, cur, subtrieSpan, off, bufferOffset, currentReadSize, bytesRead, eg)
				return nil
			})
		}(address, b, cur, subtrieSpan, off, bufferOffset, currentReadSize, subtrieSpanLimit)

		bufferOffset += currentReadSize
		bytesToRead -= currentReadSize
		cur += subtrieSpan
		off = cur
	}
}

// brute-forces the subtrie size for each of the sections in this intermediate chunk
func subtrieSection(data []byte, startIdx, refLen int, subtrieSize int64) int64 {
	// assume we have a trie of size `y` then we can assume that all of
	// the forks except for the last one on the right are of equal size
	// this is due to how the splitter wraps levels.
	// so for the branches on the left, we can assume that
	// y = (refs - 1) * x + l
	// where y is the size of the subtrie, refs are the number of references
	// x is constant (the brute forced value) and l is the size of the last subtrie
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

func (j *joiner) Seek(offset int64, whence int) (int64, error) {
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

func (j *joiner) IterateChunkAddresses(fn swarm.AddressIterFunc) error {
	// report root address
	err := fn(j.addr)
	if err != nil {
		return err
	}

	return j.processChunkAddresses(j.ctx, fn, j.rootData, j.span)
}

func (j *joiner) processChunkAddresses(ctx context.Context, fn swarm.AddressIterFunc, data []byte, subTrieSize int64) error {
	// we are at a leaf data chunk
	if subTrieSize <= int64(len(data)) {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	eg, ectx := errgroup.WithContext(ctx)

	var wg sync.WaitGroup

	for cursor := 0; cursor < len(data); cursor += j.refLength {

		address := swarm.NewAddress(data[cursor : cursor+j.refLength])

		if err := fn(address); err != nil {
			return err
		}

		sec := subtrieSection(data, cursor, j.refLength, subTrieSize)
		if sec <= 4096 {
			continue
		}

		func(address swarm.Address, eg *errgroup.Group) {
			wg.Add(1)

			eg.Go(func() error {
				defer wg.Done()

				ch, err := j.getter.Get(ectx, storage.ModeGetRequest, address)
				if err != nil {
					return err
				}

				chunkData := ch.Data()[8:]
				subtrieSpan := int64(chunkToSpan(ch.Data()))

				return j.processChunkAddresses(ectx, fn, chunkData, subtrieSpan)
			})
		}(address, eg)

		wg.Wait()
	}

	return eg.Wait()
}

func (j *joiner) Size() int64 {
	return j.span
}

func chunkToSpan(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data[:8])
}
