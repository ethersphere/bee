// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// fuzzGetter serves child chunks out of a fuzz-controlled byte pool. It exists
// so that the recursive trie traversal in the joiner is driven entirely by
// fuzzed bytes. The window chosen for an address is deterministic (derived from
// the address bytes) and always materialised into a full swarm.ChunkWithSpanSize
// buffer, so that every chunk it hands back has a valid on-wire length. This
// mirrors what a real validated chunk store guarantees on retrieval and ensures
// a short-buffer panic inside the joiner is attributable to the joiner and not
// to a malformed mock chunk. The getter only ever returns (chunk, nil) or
// (nil, err) and never panics.
type fuzzGetter struct {
	pool []byte
}

func (g *fuzzGetter) Get(_ context.Context, addr swarm.Address) (swarm.Chunk, error) {
	if len(g.pool) == 0 {
		return nil, storage.ErrNotFound
	}
	b := addr.Bytes()
	var seed uint64
	if len(b) >= 8 {
		seed = binary.BigEndian.Uint64(b[:8])
	}
	start := int(seed % uint64(len(g.pool)))

	data := make([]byte, swarm.ChunkWithSpanSize)
	copy(data, g.pool[start:])
	return swarm.NewChunk(addr, data), nil
}

// FuzzJoinerReadAt drives the real joiner parsing/traversal paths with a fuzzed
// root chunk and a fuzzed pool of child-chunk bytes. NewJoiner slices the root
// chunk data at swarm.SpanSize and decodes the span with no length check, and
// readAtOffset recursively parses spans and reference lists out of peer-controlled
// bytes. The root chunk here plays the role of a chunk fetched from a network
// getter in New(), so any input must be tolerated: the joiner must never panic
// and (thanks to the timeout context and read cap) must never hang.
func FuzzJoinerReadAt(f *testing.F) {
	// helper: assemble a fuzz input = rootChunkData || pool
	span := func(n uint64) []byte {
		s := make([]byte, swarm.SpanSize)
		binary.LittleEndian.PutUint64(s, n)
		return s
	}

	// seed 1: a small leaf root chunk (span + payload, no children).
	leaf := append(span(5), []byte("hello")...)
	f.Add(leaf)

	// seed 2: an intermediate root chunk advertising a span larger than a single
	// chunk, with two 32-byte child references, plus a pool to serve children from.
	child1 := make([]byte, swarm.HashSize)
	child2 := make([]byte, swarm.HashSize)
	for i := range child1 {
		child1[i] = 0x01
		child2[i] = 0x02
	}
	inter := append(span(uint64(swarm.ChunkSize)*2), child1...)
	inter = append(inter, child2...)
	// A zero-filled pool yields children that advertise span 0, so this seed
	// descends into the trie and terminates quickly. (Non-zero child spans are
	// left for the fuzzer to explore under its per-input timeout.)
	pool := make([]byte, swarm.ChunkWithSpanSize*2)
	f.Add(append(inter, pool...))

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Split the fuzz input: up to one full chunk of root data, remainder is
		// the child byte pool.
		split := len(data)
		if split > swarm.ChunkWithSpanSize {
			split = swarm.ChunkWithSpanSize
		}
		rootData := data[:split]
		g := &fuzzGetter{pool: data[split:]}

		// Fixed 32-byte root address selects the non-encrypted (refLength == 32) path.
		rootAddr := swarm.NewAddress(make([]byte, swarm.HashSize))
		rootChunk := swarm.NewChunk(rootAddr, rootData)

		// NewJoiner must tolerate any root chunk bytes (including < SpanSize).
		j, spanLen, err := joiner.NewJoiner(ctx, g, inmemchunkstore.New(), rootAddr, rootChunk)
		if err != nil {
			return
		}
		if j == nil {
			t.Fatal("NewJoiner returned nil joiner with nil error")
		}
		if j.Size() != spanLen {
			t.Fatalf("reported span %d != joiner size %d", spanLen, j.Size())
		}

		// Bound total work: read in chunk-sized steps until EOF, an error, or a
		// hard cap so a malformed/cyclic trie cannot exhaust memory or time.
		const maxTotal = 1 << 20
		buf := make([]byte, swarm.ChunkSize)
		var total int64
		for total < maxTotal {
			n, rerr := j.Read(buf)
			if n > len(buf) {
				t.Fatalf("Read returned n=%d exceeding buffer len %d", n, len(buf))
			}
			total += int64(n)
			if rerr != nil {
				break
			}
			if n == 0 {
				break
			}
		}

		// Exercise ReadAt at a couple of fuzzed-derived offsets.
		for _, off := range []int64{0, spanLen / 2, spanLen, spanLen + 1} {
			if off < 0 {
				continue
			}
			n, rerr := j.ReadAt(buf, off)
			if n > len(buf) {
				t.Fatalf("ReadAt(%d) returned n=%d exceeding buffer len %d", off, n, len(buf))
			}
			if rerr != nil && !errors.Is(rerr, io.EOF) {
				// propagated parse/fetch errors are expected, not failures
				_ = rerr
			}
		}

		// Exercise Seek with a few whences/offsets.
		for _, whence := range []int{0, 1, 2} {
			_, _ = j.Seek(spanLen/2, whence)
		}

		// Exercise the address-iteration traversal under the same bounded context.
		_ = j.IterateChunkAddresses(func(swarm.Address) error { return nil })
	})
}
