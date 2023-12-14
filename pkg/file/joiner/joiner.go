// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package joiner provides implementations of the file.Joiner interface
package joiner

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/encryption/store"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/redundancy"
	"github.com/ethersphere/bee/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/pkg/replicas"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

type joiner struct {
	addr         swarm.Address
	rootData     []byte
	span         int64
	off          int64
	refLength    int
	rootParity   int
	maxBranching int // maximum branching in an intermediate chunk

	ctx    context.Context
	getter storage.Getter
	putter storage.Putter // required to save recovered data

	chunkToSpan func(data []byte) (redundancy.Level, int64) // returns parity and span value from chunkData
}

// New creates a new Joiner. A Joiner provides Read, Seek and Size functionalities.
func New(ctx context.Context, getter storage.Getter, putter storage.Putter, address swarm.Address) (file.Joiner, int64, error) {
	// retrieve the root chunk to read the total data length the be retrieved
	rLevel := replicas.GetLevelFromContext(ctx)
	rootChunkGetter := store.New(getter)
	if rLevel != redundancy.NONE {
		rootChunkGetter = store.New(replicas.NewGetter(getter, rLevel))
	}
	rootChunk, err := rootChunkGetter.Get(ctx, address)
	if err != nil {
		return nil, 0, err
	}

	chunkData := rootChunk.Data()
	rootData := chunkData[swarm.SpanSize:]
	refLength := len(address.Bytes())
	encryption := refLength != swarm.HashSize
	rLevel, span := chunkToSpan(chunkData)
	rootParity := 0
	maxBranching := swarm.ChunkSize / refLength
	spanFn := func(data []byte) (redundancy.Level, int64) {
		return 0, int64(bmt.LengthFromSpan(data[:swarm.SpanSize]))
	}
	// override stuff if root chunk has redundancy
	if rLevel != redundancy.NONE {
		_, parities := file.ReferenceCount(uint64(span), rLevel, encryption)
		rootParity = parities
		spanFn = chunkToSpan
		if encryption {
			maxBranching = rLevel.GetMaxEncShards()
		} else {
			maxBranching = rLevel.GetMaxShards()
		}
	}

	j := &joiner{
		addr:         rootChunk.Address(),
		refLength:    refLength,
		ctx:          ctx,
		getter:       getter,
		putter:       putter,
		span:         span,
		rootData:     rootData,
		rootParity:   rootParity,
		maxBranching: maxBranching,
		chunkToSpan:  spanFn,
	}

	return j, span, nil
}

// Read is called by the consumer to retrieve the joined data.
// It must be called with a buffer equal to the maximum chunk size.
func (j *joiner) Read(b []byte) (n int, err error) {
	read, err := j.ReadAt(b, j.off)
	if err != nil && !errors.Is(err, io.EOF) {
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
	j.readAtOffset(buffer, j.rootData, 0, j.span, off, 0, readLen, &bytesRead, j.rootParity, &eg)

	err = eg.Wait()
	if err != nil {
		return 0, err
	}

	return int(atomic.LoadInt64(&bytesRead)), nil
}

var ErrMalformedTrie = errors.New("malformed tree")

func (j *joiner) readAtOffset(
	b, data []byte,
	cur, subTrieSize, off, bufferOffset, bytesToRead int64,
	bytesRead *int64,
	parity int,
	eg *errgroup.Group,
) {
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

	pSize, err := file.ChunkPayloadSize(data)
	if err != nil {
		eg.Go(func() error {
			return err
		})
		return
	}
	sAddresses, pAddresses := file.ChunkAddresses(data[:pSize], parity, j.refLength)
	getter := store.New(getter.New(sAddresses, pAddresses, j.getter, j.putter))
	for cursor := 0; cursor < len(data); cursor += j.refLength {
		if bytesToRead == 0 {
			break
		}

		// fast forward the cursor
		sec := j.subtrieSection(data, cursor, pSize, parity, subTrieSize)
		if cur+sec <= off {
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
				ch, err := getter.Get(j.ctx, address)
				if err != nil {
					return err
				}

				chunkData := ch.Data()[8:]
				subtrieLevel, subtrieSpan := j.chunkToSpan(ch.Data())
				_, subtrieParity := file.ReferenceCount(uint64(subtrieSpan), subtrieLevel, j.refLength != swarm.HashSize)

				if subtrieSpan > subtrieSpanLimit {
					return ErrMalformedTrie
				}

				j.readAtOffset(b, chunkData, cur, subtrieSpan, off, bufferOffset, currentReadSize, bytesRead, subtrieParity, eg)
				return nil
			})
		}(address, b, cur, subtrieSpan, off, bufferOffset, currentReadSize, subtrieSpanLimit)

		bufferOffset += currentReadSize
		bytesToRead -= currentReadSize
		cur += subtrieSpan
		off = cur
	}
}

// getShards returns the effective reference number respective to the intermediate chunk payload length and its parities
func (j *joiner) getShards(payloadSize, parities int) int {
	return (payloadSize - parities*swarm.HashSize) / j.refLength
}

// brute-forces the subtrie size for each of the sections in this intermediate chunk
func (j *joiner) subtrieSection(data []byte, startIdx, payloadSize, parities int, subtrieSize int64) int64 {
	// assume we have a trie of size `y` then we can assume that all of
	// the forks except for the last one on the right are of equal size
	// this is due to how the splitter wraps levels.
	// so for the branches on the left, we can assume that
	// y = (refs - 1) * x + l
	// where y is the size of the subtrie, refs are the number of references
	// x is constant (the brute forced value) and l is the size of the last subtrie
	var (
		refs       = int64(j.getShards(payloadSize, parities)) // how many effective references in the intermediate chunk
		branching  = int64(j.maxBranching)                     // branching factor is chunkSize divided by reference length
		branchSize = int64(swarm.ChunkSize)
	)
	for {
		whatsLeft := subtrieSize - (branchSize * (refs - 1))
		if whatsLeft <= branchSize {
			break
		}
		branchSize *= branching
	}

	// handle last branch edge case
	if startIdx == int(refs-1)*j.refLength {
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

	return j.processChunkAddresses(j.ctx, fn, j.rootData, j.span, j.rootParity)
}

func (j *joiner) processChunkAddresses(ctx context.Context, fn swarm.AddressIterFunc, data []byte, subTrieSize int64, parity int) error {
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

	eSize, err := file.ChunkPayloadSize(data)
	if err != nil {
		return err
	}
	sAddresses, pAddresses := file.ChunkAddresses(data[:eSize], parity, j.refLength)
	getter := getter.New(sAddresses, pAddresses, j.getter, j.putter)
	for cursor := 0; cursor < len(data); cursor += j.refLength {
		ref := data[cursor : cursor+j.refLength]
		var reportAddr swarm.Address
		address := swarm.NewAddress(ref)
		if len(ref) == encryption.ReferenceSize {
			reportAddr = swarm.NewAddress(ref[:swarm.HashSize])
		} else {
			reportAddr = swarm.NewAddress(ref)
		}

		if err := fn(reportAddr); err != nil {
			return err
		}

		sec := j.subtrieSection(data, cursor, eSize, parity, subTrieSize)
		if sec <= swarm.ChunkSize {
			continue
		}

		func(address swarm.Address, eg *errgroup.Group) {
			wg.Add(1)

			eg.Go(func() error {
				defer wg.Done()

				ch, err := getter.Get(ectx, address)
				if err != nil {
					return err
				}

				chunkData := ch.Data()[8:]
				subtrieLevel, subtrieSpan := j.chunkToSpan(ch.Data())
				_, parities := file.ReferenceCount(uint64(subtrieSpan), subtrieLevel, j.refLength != swarm.HashSize)

				return j.processChunkAddresses(ectx, fn, chunkData, subtrieSpan, parities)
			})
		}(address, eg)

		wg.Wait()
	}

	return eg.Wait()
}

func (j *joiner) Size() int64 {
	return j.span
}

// chunkToSpan returns redundancy level and span value
// in the types that the package uses
func chunkToSpan(data []byte) (redundancy.Level, int64) {
	level, spanBytes := redundancy.DecodeSpan(data[:swarm.SpanSize])
	return level, int64(bmt.LengthFromSpan(spanBytes))
}
