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

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/encryption"
	"github.com/ethersphere/bee/v2/pkg/encryption/store"
	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/replicas"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

	ctx         context.Context
	decoders    *decoderCache
	chunkToSpan func(data []byte) (redundancy.Level, int64) // returns parity and span value from chunkData
}

// decoderCache is cache of decoders for intermediate chunks
type decoderCache struct {
	fetcher storage.Getter            // network retrieval interface to fetch chunks
	putter  storage.Putter            // interface to local storage to save reconstructed chunks
	mu      sync.Mutex                // mutex to protect cache
	cache   map[string]storage.Getter // map from chunk address to RS getter
	config  getter.Config             // getter configuration
}

// NewDecoderCache creates a new decoder cache
func NewDecoderCache(g storage.Getter, p storage.Putter, conf getter.Config) *decoderCache {
	return &decoderCache{
		fetcher: g,
		putter:  p,
		cache:   make(map[string]storage.Getter),
		config:  conf,
	}
}

func fingerprint(addrs []swarm.Address) string {
	h := swarm.NewHasher()
	for _, addr := range addrs {
		_, _ = h.Write(addr.Bytes())
	}
	return string(h.Sum(nil))
}

// createRemoveCallback returns a function that handles the cleanup after a recovery attempt
func (g *decoderCache) createRemoveCallback(key string) func(error) {
	return func(err error) {
		g.mu.Lock()
		defer g.mu.Unlock()
		if err != nil {
			// signals that a new getter is needed to reattempt to recover the data
			delete(g.cache, key)
		} else {
			// signals that the chunks were fetched/recovered/cached so a future getter is not needed
			// The nil value indicates a successful recovery
			g.cache[key] = nil
		}
	}
}

// getFromCache safely retrieves a decoder from cache with lock
func (g *decoderCache) getFromCache(key string) (storage.Getter, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	d, ok := g.cache[key]
	return d, ok
}

// GetOrCreate returns a decoder for the given chunk address
func (g *decoderCache) GetOrCreate(addrs []swarm.Address, shardCnt int) storage.Getter {
	// since a recovery decoder is not allowed, simply return the underlying netstore
	if g.config.Strict && g.config.Strategy == getter.NONE {
		return g.fetcher
	}

	if len(addrs) == shardCnt {
		return g.fetcher
	}

	key := fingerprint(addrs)
	d, ok := g.getFromCache(key)

	if ok {
		if d == nil {
			// The nil value indicates a previous successful recovery
			// Create a new decoder but only use it as fallback if network fetch fails
			decoderCallback := g.createRemoveCallback(key)

			// Create a factory function that will instantiate the decoder only when needed
			recovery := func() storage.Getter {
				g.config.Logger.Debug("lazy-creating recovery decoder after fetch failed", "key", key)

				if d, ok := g.getFromCache(key); ok && d != nil {
					return d
				}

				newGetter := getter.New(addrs, shardCnt, g.fetcher, g.putter, decoderCallback, g.config)

				g.mu.Lock()
				defer g.mu.Unlock()
				if d, ok := g.cache[key]; ok && d != nil {
					return d
				}
				g.cache[key] = newGetter
				return newGetter
			}

			return getter.NewReDecoder(g.fetcher, recovery, g.config.Logger)
		}
		return d
	}

	removeCallback := g.createRemoveCallback(key)
	newGetter := getter.New(addrs, shardCnt, g.fetcher, g.putter, removeCallback, g.config)

	// ensure no other goroutine created the same getter
	g.mu.Lock()
	defer g.mu.Unlock()
	if d, ok := g.cache[key]; ok {
		return d
	}
	g.cache[key] = newGetter
	return newGetter
}

// New creates a new Joiner. A Joiner provides Read, Seek and Size functionalities.
func New(ctx context.Context, g storage.Getter, putter storage.Putter, address swarm.Address, rLevel redundancy.Level) (file.Joiner, int64, error) {
	// retrieve the root chunk to read the total data length the be retrieved
	rootChunkGetter := store.New(g)
	if rLevel != redundancy.NONE {
		rootChunkGetter = store.New(replicas.NewGetter(g, rLevel))
	}
	rootChunk, err := rootChunkGetter.Get(ctx, address)
	if err != nil {
		return nil, 0, err
	}

	return NewJoiner(ctx, g, putter, address, rootChunk)
}

// NewJoiner creates a new Joiner with the already fetched root chunk.
// A Joiner provides Read, Seek and Size functionalities.
func NewJoiner(ctx context.Context, g storage.Getter, putter storage.Putter, address swarm.Address, rootChunk swarm.Chunk) (file.Joiner, int64, error) {
	chunkData := rootChunk.Data()
	rootData := chunkData[swarm.SpanSize:]
	refLength := len(address.Bytes())
	encryption := refLength == encryption.ReferenceSize
	rLevel, span := chunkToSpan(chunkData)
	rootParity := 0
	maxBranching := swarm.ChunkSize / refLength
	spanFn := func(data []byte) (redundancy.Level, int64) {
		return 0, int64(bmt.LengthFromSpan(data[:swarm.SpanSize]))
	}
	conf, err := getter.NewConfigFromContext(ctx, getter.DefaultConfig)
	if err != nil {
		return nil, 0, err
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
	} else {
		// if root chunk has no redundancy, strategy is ignored and set to DATA and strict is set to true
		conf.Strategy = getter.DATA
		conf.Strict = true
	}

	j := &joiner{
		addr:         rootChunk.Address(),
		refLength:    refLength,
		ctx:          ctx,
		decoders:     NewDecoderCache(g, putter, conf),
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

	readLen := min(int64(cap(buffer)), j.span-off)
	var bytesRead int64
	eg, ectx := errgroup.WithContext(j.ctx)
	j.readAtOffset(buffer, j.rootData, 0, j.span, off, 0, readLen, &bytesRead, j.rootParity, eg, ectx)

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
	ectx context.Context,
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

	addrs, shardCnt := file.ChunkAddresses(data[:pSize], parity, j.refLength)
	g := store.New(j.decoders.GetOrCreate(addrs, shardCnt))
	for cursor := 0; cursor < len(data); cursor += j.refLength {
		if bytesToRead == 0 {
			break
		}

		// fast forward the cursor
		sec := j.subtrieSection(cursor, pSize, parity, subTrieSize)
		if cur+sec <= off {
			cur += sec
			continue
		}

		// if we are here it means that we are within the bounds of the data we need to read
		addr := swarm.NewAddress(data[cursor : cursor+j.refLength])

		subtrieSpan := sec
		subtrieSpanLimit := sec

		currentReadSize := subtrieSpan - (off - cur) // the size of the subtrie, minus the offset from the start of the trie
		// upper bound alignments
		currentReadSize = min(currentReadSize, bytesToRead)
		currentReadSize = min(currentReadSize, subtrieSpan)

		func(address swarm.Address, b []byte, cur, subTrieSize, off, bufferOffset, bytesToRead, subtrieSpanLimit int64) {
			eg.Go(func() error {
				select {
				case <-ectx.Done():
					return ectx.Err()
				default:
				}
				ch, err := g.Get(ectx, addr)
				if err != nil {
					return err
				}

				chunkData := ch.Data()[8:]
				subtrieLevel, subtrieSpan := j.chunkToSpan(ch.Data())
				_, subtrieParity := file.ReferenceCount(uint64(subtrieSpan), subtrieLevel, j.refLength == encryption.ReferenceSize)

				if subtrieSpan > subtrieSpanLimit {
					return ErrMalformedTrie
				}

				j.readAtOffset(b, chunkData, cur, subtrieSpan, off, bufferOffset, currentReadSize, bytesRead, subtrieParity, eg, ectx)
				return nil
			})
		}(addr, b, cur, subtrieSpan, off, bufferOffset, currentReadSize, subtrieSpanLimit)

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
func (j *joiner) subtrieSection(startIdx, payloadSize, parities int, subtrieSize int64) int64 {
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

	eSize, err := file.ChunkPayloadSize(data)
	if err != nil {
		return err
	}
	addrs, shardCnt := file.ChunkAddresses(data[:eSize], parity, j.refLength)
	g := store.New(j.decoders.GetOrCreate(addrs, shardCnt))
	for i, addr := range addrs {
		if err := fn(addr); err != nil {
			return err
		}
		cursor := i * swarm.HashSize
		if j.refLength == encryption.ReferenceSize {
			cursor += swarm.HashSize * min(i, shardCnt)
		}
		sec := j.subtrieSection(cursor, eSize, parity, subTrieSize)
		if sec <= swarm.ChunkSize {
			continue
		}

		if j.refLength == encryption.ReferenceSize && i < shardCnt {
			addr = swarm.NewAddress(data[cursor : cursor+swarm.HashSize*2])
		}

		// not a shard
		if i >= shardCnt {
			continue
		}

		ch, err := g.Get(ctx, addr)
		if err != nil {
			return err
		}

		chunkData := ch.Data()[8:]
		subtrieLevel, subtrieSpan := j.chunkToSpan(ch.Data())
		_, parities := file.ReferenceCount(uint64(subtrieSpan), subtrieLevel, j.refLength != swarm.HashSize)

		err = j.processChunkAddresses(ctx, fn, chunkData, subtrieSpan, parities)
		if err != nil {
			return err
		}
	}

	return nil
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
