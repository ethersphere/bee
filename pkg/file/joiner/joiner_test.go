// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package joiner_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/joiner"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy/getter"
	"github.com/ethersphere/bee/v2/pkg/file/splitter"
	filetest "github.com/ethersphere/bee/v2/pkg/file/testing"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/ethersphere/bee/v2/pkg/util/testutil/pseudorand"
	"gitlab.com/nolash/go-mockbytes"
	"golang.org/x/sync/errgroup"
)

// nolint:paralleltest,tparallel,thelper

func TestJoiner_ErrReferenceLength(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()
	_, _, err := joiner.New(context.Background(), store, store, swarm.ZeroAddress)

	if !errors.Is(err, storage.ErrReferenceLength) {
		t.Fatalf("expected ErrReferenceLength %x but got %v", swarm.ZeroAddress, err)
	}
}

// TestJoinerSingleChunk verifies that a newly created joiner returns the data stored
// in the store when the reference is one single chunk.
func TestJoinerSingleChunk(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create the chunk to
	mockAddrHex := fmt.Sprintf("%064s", "2a")
	mockAddr := swarm.MustParseHexAddress(mockAddrHex)
	mockData := []byte("foo")
	mockDataLengthBytes := make([]byte, 8)
	mockDataLengthBytes[0] = 0x03
	mockChunk := swarm.NewChunk(mockAddr, append(mockDataLengthBytes, mockData...))
	err := store.Put(ctx, mockChunk)
	if err != nil {
		t.Fatal(err)
	}

	// read back data and compare
	joinReader, l, err := joiner.New(ctx, store, store, mockAddr)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(len(mockData)) {
		t.Fatalf("expected join data length %d, got %d", len(mockData), l)
	}
	joinData, err := io.ReadAll(joinReader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(joinData, mockData) {
		t.Fatalf("retrieved data '%x' not like original data '%x'", joinData, mockData)
	}
}

// TestJoinerDecryptingStore_NormalChunk verifies the mock store that uses
// the decrypting store manages to retrieve a normal chunk which is not encrypted
func TestJoinerDecryptingStore_NormalChunk(t *testing.T) {
	t.Parallel()

	st := inmemchunkstore.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create the chunk to
	mockAddrHex := fmt.Sprintf("%064s", "2a")
	mockAddr := swarm.MustParseHexAddress(mockAddrHex)
	mockData := []byte("foo")
	mockDataLengthBytes := make([]byte, 8)
	mockDataLengthBytes[0] = 0x03
	mockChunk := swarm.NewChunk(mockAddr, append(mockDataLengthBytes, mockData...))
	err := st.Put(ctx, mockChunk)
	if err != nil {
		t.Fatal(err)
	}

	// read back data and compare
	joinReader, l, err := joiner.New(ctx, st, st, mockAddr)
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(len(mockData)) {
		t.Fatalf("expected join data length %d, got %d", len(mockData), l)
	}
	joinData, err := io.ReadAll(joinReader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(joinData, mockData) {
		t.Fatalf("retrieved data '%x' not like original data '%x'", joinData, mockData)
	}
}

// TestJoinerWithReference verifies that a chunk reference is correctly resolved
// and the underlying data is returned.
func TestJoinerWithReference(t *testing.T) {
	t.Parallel()

	st := inmemchunkstore.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create root chunk and two data chunks referenced in the root chunk
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	err := st.Put(ctx, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = st.Put(ctx, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = st.Put(ctx, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	// read back data and compare
	joinReader, l, err := joiner.New(ctx, st, st, rootChunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	if l != int64(swarm.ChunkSize*2) {
		t.Fatalf("expected join data length %d, got %d", swarm.ChunkSize*2, l)
	}

	resultBuffer := make([]byte, swarm.ChunkSize)
	n, err := joinReader.Read(resultBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(resultBuffer) {
		t.Fatalf("expected read count %d, got %d", len(resultBuffer), n)
	}
	if !bytes.Equal(resultBuffer, firstChunk.Data()[8:]) {
		t.Fatalf("expected resultbuffer %v, got %v", resultBuffer, firstChunk.Data()[:len(resultBuffer)])
	}
}

func TestJoinerMalformed(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subTrie := []byte{8085: 1}
	pb := builder.NewPipelineBuilder(ctx, store, false, 0)
	c1addr, _ := builder.FeedPipeline(ctx, pb, bytes.NewReader(subTrie))

	chunk2 := testingc.GenerateTestRandomChunk()
	err := store.Put(ctx, chunk2)
	if err != nil {
		t.Fatal(err)
	}

	// root chunk
	rootChunkData := make([]byte, 8+64)
	binary.LittleEndian.PutUint64(rootChunkData[:8], uint64(swarm.ChunkSize*2))

	copy(rootChunkData[8:], c1addr.Bytes())
	copy(rootChunkData[8+32:], chunk2.Address().Bytes())

	rootChunk, err := cac.NewWithDataSpan(rootChunkData)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Put(ctx, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	joinReader, _, err := joiner.New(ctx, store, store, rootChunk.Address())
	if err != nil {
		t.Fatal(err)
	}

	resultBuffer := make([]byte, swarm.ChunkSize)
	_, err = joinReader.Read(resultBuffer)
	if !errors.Is(err, joiner.ErrMalformedTrie) {
		t.Fatalf("expected %v, got %v", joiner.ErrMalformedTrie, err)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		chunkLength int
	}{
		{10},
		{100},
		{1000},
		{4095},
		{4096},
		{4097},
		{1000000},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Encrypt %d bytes", tt.chunkLength), func(t *testing.T) {
			t.Parallel()

			store := inmemchunkstore.New()

			g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
			testData, err := g.SequentialBytes(tt.chunkLength)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pipe := builder.NewPipelineBuilder(ctx, store, true, 0)
			testDataReader := bytes.NewReader(testData)
			resultAddress, err := builder.FeedPipeline(ctx, pipe, testDataReader)
			if err != nil {
				t.Fatal(err)
			}
			reader, l, err := joiner.New(context.Background(), store, store, resultAddress)
			if err != nil {
				t.Fatal(err)
			}

			if l != int64(len(testData)) {
				t.Fatalf("expected join data length %d, got %d", len(testData), l)
			}

			totalGot := make([]byte, tt.chunkLength)
			index := 0
			resultBuffer := make([]byte, swarm.ChunkSize)

			for index < tt.chunkLength {
				n, err := reader.Read(resultBuffer)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Fatal(err)
				}
				copy(totalGot[index:], resultBuffer[:n])
				index += n
			}

			if !bytes.Equal(testData, totalGot) {
				t.Fatal("input data and output data does not match")
			}
		})
	}
}

func TestSeek(t *testing.T) {
	t.Parallel()

	seed := time.Now().UnixNano()

	for _, tc := range []struct {
		name string
		size int64
	}{
		{
			name: "one byte",
			size: 1,
		},
		{
			name: "a few bytes",
			size: 10,
		},
		{
			name: "a few bytes more",
			size: 65,
		},
		{
			name: "almost a chunk",
			size: 4095,
		},
		{
			name: "one chunk",
			size: swarm.ChunkSize,
		},
		{
			name: "a few chunks",
			size: 10 * swarm.ChunkSize,
		},
		{
			name: "a few chunks and a change",
			size: 10*swarm.ChunkSize + 84,
		},
		{
			name: "a few chunks more",
			size: 2*swarm.ChunkSize*swarm.ChunkSize + 1000,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			store := inmemchunkstore.New()
			testutil.CleanupCloser(t, store)

			data := testutil.RandBytesWithSeed(t, int(tc.size), seed)
			s := splitter.NewSimpleSplitter(store)
			addr, err := s.Split(ctx, io.NopCloser(bytes.NewReader(data)), tc.size, false)
			if err != nil {
				t.Fatal(err)
			}

			j, _, err := joiner.New(ctx, store, store, addr)
			if err != nil {
				t.Fatal(err)
			}

			validateRead := func(t *testing.T, name string, i int) {
				t.Helper()

				got := make([]byte, swarm.ChunkSize)
				count, err := j.Read(got)
				if err != nil {
					t.Fatal(err)
				}
				if count == 0 {
					t.Errorf("read with seek from %s to %v: got count 0", name, i)
				}
				got = got[:count]
				want := data[i : i+count]
				if !bytes.Equal(got, want) {
					t.Fatal("data mismatch")
				}
			}

			// seek to 10 random locations
			for i := int64(0); i < 10 && i < tc.size; i++ {
				exp := mrand.Int63n(tc.size)
				n, err := j.Seek(exp, io.SeekStart)
				if err != nil {
					t.Fatal(err)
				}
				if n != exp {
					t.Errorf("seek to %v from start, want %v", n, exp)
				}

				validateRead(t, "start", int(n))
			}
			if _, err := j.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}

			// seek to all possible locations from current position
			for i := int64(1); i < 10 && i < tc.size; i++ {
				exp := mrand.Int63n(tc.size)
				n, err := j.Seek(exp, io.SeekCurrent)
				if err != nil {
					t.Fatal(err)
				}
				if n != exp {
					t.Errorf("seek to %v from current, want %v", n, exp)
				}

				validateRead(t, "current", int(n))
				if _, err := j.Seek(0, io.SeekStart); err != nil {
					t.Fatal(err)
				}

			}
			if _, err := j.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}

			// seek to 10 random locations from end
			for i := int64(1); i < 10; i++ {
				exp := mrand.Int63n(tc.size)
				if exp == 0 {
					exp = 1
				}
				n, err := j.Seek(exp, io.SeekEnd)
				if err != nil {
					t.Fatalf("seek from end, exp %d size %d error: %v", exp, tc.size, err)
				}
				want := tc.size - exp
				if n != want {
					t.Errorf("seek to %v from end, want %v, size %v, exp %v", n, want, tc.size, exp)
				}

				validateRead(t, "end", int(n))
			}
			if _, err := j.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			// seek overflow for a few bytes
			for i := int64(1); i < 5; i++ {
				n, err := j.Seek(tc.size+i, io.SeekStart)
				if !errors.Is(err, io.EOF) {
					t.Errorf("seek overflow to %v: got error %v, want %v", i, err, io.EOF)
				}

				if n != 0 {
					t.Errorf("seek overflow to %v: got %v, want 0", i, n)
				}
			}
		})
	}
}

// TestPrefetch tests that prefetching chunks is made to fill up the read buffer
func TestPrefetch(t *testing.T) {
	t.Parallel()

	seed := time.Now().UnixNano()

	for _, tc := range []struct {
		name       string
		size       int64
		bufferSize int
		readOffset int64
		expRead    int
	}{
		{
			name:       "one byte",
			size:       1,
			bufferSize: 1,
			readOffset: 0,
			expRead:    1,
		},
		{
			name:       "one byte",
			size:       1,
			bufferSize: 10,
			readOffset: 0,
			expRead:    1,
		},
		{
			name:       "ten bytes",
			size:       10,
			bufferSize: 5,
			readOffset: 0,
			expRead:    5,
		},
		{
			name:       "thousand bytes",
			size:       1000,
			bufferSize: 100,
			readOffset: 0,
			expRead:    100,
		},
		{
			name:       "thousand bytes",
			size:       1000,
			bufferSize: 100,
			readOffset: 900,
			expRead:    100,
		},
		{
			name:       "thousand bytes",
			size:       1000,
			bufferSize: 100,
			readOffset: 800,
			expRead:    100,
		},
		{
			name:       "one chunk",
			size:       4096,
			bufferSize: 4096,
			readOffset: 0,
			expRead:    4096,
		},
		{
			name:       "one chunk minus a few",
			size:       4096,
			bufferSize: 4093,
			readOffset: 0,
			expRead:    4093,
		},
		{
			name:       "one chunk minus a few",
			size:       4096,
			bufferSize: 4093,
			readOffset: 3,
			expRead:    4093,
		},
		{
			name:       "one byte at the end",
			size:       4096,
			bufferSize: 1,
			readOffset: 4095,
			expRead:    1,
		},
		{
			name:       "one byte at the end",
			size:       8192,
			bufferSize: 1,
			readOffset: 8191,
			expRead:    1,
		},
		{
			name:       "one byte at the end",
			size:       8192,
			bufferSize: 1,
			readOffset: 8190,
			expRead:    1,
		},
		{
			name:       "one byte at the end",
			size:       100000,
			bufferSize: 1,
			readOffset: 99999,
			expRead:    1,
		},
		{
			name:       "10kb",
			size:       10000,
			bufferSize: 5,
			readOffset: 5,
			expRead:    5,
		},

		{
			name:       "10kb",
			size:       10000,
			bufferSize: 1500,
			readOffset: 5,
			expRead:    1500,
		},

		{
			name:       "100kb",
			size:       100000,
			bufferSize: 8000,
			readOffset: 100,
			expRead:    8000,
		},

		{
			name:       "100kb",
			size:       100000,
			bufferSize: 80000,
			readOffset: 100,
			expRead:    80000,
		},

		{
			name:       "10megs",
			size:       10000000,
			bufferSize: 8000,
			readOffset: 990000,
			expRead:    8000,
		},
		{
			name:       "10megs",
			size:       10000000,
			bufferSize: 80000,
			readOffset: 900000,
			expRead:    80000,
		},
		{
			name:       "10megs",
			size:       10000000,
			bufferSize: 8000000,
			readOffset: 900000,
			expRead:    8000000,
		},
		{
			name:       "10megs",
			size:       1000000,
			bufferSize: 2000000,
			readOffset: 900000,
			expRead:    100000,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			store := inmemchunkstore.New()
			testutil.CleanupCloser(t, store)

			data := testutil.RandBytesWithSeed(t, int(tc.size), seed)
			s := splitter.NewSimpleSplitter(store)
			addr, err := s.Split(ctx, io.NopCloser(bytes.NewReader(data)), tc.size, false)
			if err != nil {
				t.Fatal(err)
			}

			j, _, err := joiner.New(ctx, store, store, addr)
			if err != nil {
				t.Fatal(err)
			}
			b := make([]byte, tc.bufferSize)
			n, err := j.ReadAt(b, tc.readOffset)
			if err != nil {
				t.Fatal(err)
			}
			if n != tc.expRead {
				t.Errorf("read %d bytes out of %d", n, tc.expRead)
			}
			ro := int(tc.readOffset)
			if !bytes.Equal(b[:n], data[ro:ro+n]) {
				t.Error("buffer does not match generated data")
			}
		})
	}
}

func TestJoinerReadAt(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create root chunk with 2 references and the referenced data chunks
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	err := store.Put(ctx, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = store.Put(ctx, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = store.Put(ctx, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	j, _, err := joiner.New(ctx, store, store, rootChunk.Address())
	if err != nil {
		t.Fatal(err)
	}

	b := make([]byte, swarm.ChunkSize)
	_, err = j.ReadAt(b, swarm.ChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(b, secondChunk.Data()[8:]) {
		t.Fatal("data read at offset not equal to expected chunk")
	}
}

// TestJoinerOneLevel tests the retrieval of two data chunks immediately
// below the root chunk level.
func TestJoinerOneLevel(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create root chunk with 2 references and the referenced data chunks
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	err := store.Put(ctx, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = store.Put(ctx, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = store.Put(ctx, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	j, _, err := joiner.New(ctx, store, store, rootChunk.Address())
	if err != nil {
		t.Fatal(err)
	}

	// verify first chunk content
	outBuffer := make([]byte, swarm.ChunkSize)
	c, err := j.Read(outBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if c != swarm.ChunkSize {
		t.Fatalf("expected firstchunk read count %d, got %d", swarm.ChunkSize, c)
	}
	if !bytes.Equal(outBuffer, firstChunk.Data()[8:]) {
		t.Fatalf("firstchunk data mismatch, expected %x, got %x", outBuffer, firstChunk.Data()[8:])
	}

	// verify second chunk content
	c, err = j.Read(outBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if c != swarm.ChunkSize {
		t.Fatalf("expected secondchunk read count %d, got %d", swarm.ChunkSize, c)
	}
	if !bytes.Equal(outBuffer, secondChunk.Data()[8:]) {
		t.Fatalf("secondchunk data mismatch, expected %x, got %x", outBuffer, secondChunk.Data()[8:])
	}

	// verify EOF is returned also after first time it is returned
	_, err = j.Read(outBuffer)
	if !errors.Is(err, io.EOF) {
		t.Fatal("expected io.EOF")
	}

	_, err = j.Read(outBuffer)
	if !errors.Is(err, io.EOF) {
		t.Fatal("expected io.EOF")
	}
}

// TestJoinerTwoLevelsAcrossChunk tests the retrieval of data chunks below
// first intermediate level across two intermediate chunks.
// Last chunk has sub-chunk length.
func TestJoinerTwoLevelsAcrossChunk(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create root chunk with 2 references and two intermediate chunks with references
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*swarm.Branches+42, swarm.SectionSize*2)
	err := store.Put(ctx, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize*swarm.Branches, swarm.ChunkSize)
	err = store.Put(ctx, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, 42, swarm.SectionSize)
	err = store.Put(ctx, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	// create 128+1 chunks for all references in the intermediate chunks
	cursor := 8
	for i := 0; i < swarm.Branches; i++ {
		chunkAddressBytes := firstChunk.Data()[cursor : cursor+swarm.SectionSize]
		chunkAddress := swarm.NewAddress(chunkAddressBytes)
		ch := filetest.GenerateTestRandomFileChunk(chunkAddress, swarm.ChunkSize, swarm.ChunkSize)
		err := store.Put(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
		cursor += swarm.SectionSize
	}
	chunkAddressBytes := secondChunk.Data()[8:]
	chunkAddress := swarm.NewAddress(chunkAddressBytes)
	ch := filetest.GenerateTestRandomFileChunk(chunkAddress, 42, 42)
	err = store.Put(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}

	j, _, err := joiner.New(ctx, store, store, rootChunk.Address())
	if err != nil {
		t.Fatal(err)
	}

	// read back all the chunks and verify
	b := make([]byte, swarm.ChunkSize)
	for i := 0; i < swarm.Branches; i++ {
		c, err := j.Read(b)
		if err != nil {
			t.Fatal(err)
		}
		if c != swarm.ChunkSize {
			t.Fatalf("chunk %d expected read %d bytes; got %d", i, swarm.ChunkSize, c)
		}
	}
	c, err := j.Read(b)
	if err != nil {
		t.Fatal(err)
	}
	if c != 42 {
		t.Fatalf("last chunk expected read %d bytes; got %d", 42, c)
	}
}

func TestJoinerIterateChunkAddresses(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create root chunk with 2 references and the referenced data chunks
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	err := store.Put(ctx, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = store.Put(ctx, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	err = store.Put(ctx, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	createdAddresses := []swarm.Address{rootChunk.Address(), firstAddress, secondAddress}

	j, _, err := joiner.New(ctx, store, store, rootChunk.Address())
	if err != nil {
		t.Fatal(err)
	}

	foundAddresses := make(map[string]struct{})
	var foundAddressesMu sync.Mutex

	err = j.IterateChunkAddresses(func(addr swarm.Address) error {
		foundAddressesMu.Lock()
		defer foundAddressesMu.Unlock()

		foundAddresses[addr.String()] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(createdAddresses) != len(foundAddresses) {
		t.Fatalf("expected to find %d addresses, got %d", len(createdAddresses), len(foundAddresses))
	}

	checkAddressFound := func(t *testing.T, foundAddresses map[string]struct{}, address swarm.Address) {
		t.Helper()

		if _, ok := foundAddresses[address.String()]; !ok {
			t.Fatalf("expected address %s not found", address.String())
		}
	}

	for _, createdAddress := range createdAddresses {
		checkAddressFound(t, foundAddresses, createdAddress)
	}
}

func TestJoinerIterateChunkAddresses_Encrypted(t *testing.T) {
	t.Parallel()

	store := inmemchunkstore.New()

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	testData, err := g.SequentialBytes(10000)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pipe := builder.NewPipelineBuilder(ctx, store, true, 0)
	testDataReader := bytes.NewReader(testData)
	resultAddress, err := builder.FeedPipeline(ctx, pipe, testDataReader)
	if err != nil {
		t.Fatal(err)
	}
	j, l, err := joiner.New(context.Background(), store, store, resultAddress)
	if err != nil {
		t.Fatal(err)
	}

	if l != int64(len(testData)) {
		t.Fatalf("expected join data length %d, got %d", len(testData), l)
	}

	foundAddresses := make(map[string]struct{})
	var foundAddressesMu sync.Mutex

	err = j.IterateChunkAddresses(func(addr swarm.Address) error {
		foundAddressesMu.Lock()
		defer foundAddressesMu.Unlock()

		foundAddresses[addr.String()] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if l := len(foundAddresses); l != 4 {
		t.Fatalf("got %d addresses, want 4", l)
	}

	for v := range foundAddresses {
		// this is 64 because 32 bytes hex is 64 chars
		if len(v) != 64 {
			t.Fatalf("got wrong ref size %d, %s", len(v), v)
		}
	}
}

type mockPutter struct {
	storage.ChunkStore
	shards, parities chan swarm.Chunk
	done             chan struct{}
	mu               sync.Mutex
}

func newMockPutter(store storage.ChunkStore, shardCnt, parityCnt int) *mockPutter {
	return &mockPutter{
		ChunkStore: store,
		done:       make(chan struct{}, 1),
		shards:     make(chan swarm.Chunk, shardCnt),
		parities:   make(chan swarm.Chunk, parityCnt),
	}
}

func (m *mockPutter) Put(ctx context.Context, ch swarm.Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.shards) < cap(m.shards) {
		m.shards <- ch
		return nil
	}
	if len(m.parities) < cap(m.parities) {
		m.parities <- ch
		return nil
	}
	err := m.ChunkStore.Put(ctx, ch) // use passed context
	select {
	case m.done <- struct{}{}:
	default:
	}
	return err
}

func (m *mockPutter) wait(ctx context.Context) {
	select {
	case <-m.done:
	case <-ctx.Done():
	}
	m.mu.Lock()
	close(m.parities)
	close(m.shards)
	m.mu.Unlock()
}

func (m *mockPutter) store(cnt int) error {
	n := 0
	m.mu.Lock()
	defer m.mu.Unlock()
	for ch := range m.parities {
		if err := m.ChunkStore.Put(context.Background(), ch); err != nil {
			return err
		}
		n++
		if n == cnt {
			return nil
		}
	}
	for ch := range m.shards {
		if err := m.ChunkStore.Put(context.Background(), ch); err != nil {
			return err
		}
		n++
		if n == cnt {
			break
		}
	}
	return nil
}

// nolint:thelper
func TestJoinerRedundancy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rLevel       redundancy.Level
		encryptChunk bool
	}{
		{
			redundancy.MEDIUM,
			false,
		},
		{
			redundancy.MEDIUM,
			true,
		},
		{
			redundancy.STRONG,
			false,
		},
		{
			redundancy.STRONG,
			true,
		},
		{
			redundancy.INSANE,
			false,
		},
		{
			redundancy.INSANE,
			true,
		},
		{
			redundancy.PARANOID,
			false,
		},
		{
			redundancy.PARANOID,
			true,
		},
	} {
		t.Run(fmt.Sprintf("redundancy=%d encryption=%t", tc.rLevel, tc.encryptChunk), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			shardCnt := tc.rLevel.GetMaxShards()
			parityCnt := tc.rLevel.GetParities(shardCnt)
			if tc.encryptChunk {
				shardCnt = tc.rLevel.GetMaxEncShards()
				parityCnt = tc.rLevel.GetEncParities(shardCnt)
			}
			store := inmemchunkstore.New()
			putter := newMockPutter(store, shardCnt, parityCnt)
			pipe := builder.NewPipelineBuilder(ctx, putter, tc.encryptChunk, tc.rLevel)
			dataChunks := make([]swarm.Chunk, shardCnt)
			chunkSize := swarm.ChunkSize
			size := chunkSize
			for i := 0; i < shardCnt; i++ {
				if i == shardCnt-1 {
					size = 5
				}
				chunkData := make([]byte, size)
				_, err := io.ReadFull(rand.Reader, chunkData)
				if err != nil {
					t.Fatal(err)
				}
				dataChunks[i], err = cac.New(chunkData)
				if err != nil {
					t.Fatal(err)
				}
				_, err = pipe.Write(chunkData)
				if err != nil {
					t.Fatal(err)
				}
			}

			// reader init
			sum, err := pipe.Sum()
			if err != nil {
				t.Fatal(err)
			}
			swarmAddr := swarm.NewAddress(sum)
			putter.wait(ctx)
			_, err = store.Get(ctx, swarm.NewAddress(sum[:swarm.HashSize]))
			if err != nil {
				t.Fatal(err)
			}
			// all data can be read back
			readCheck := func(t *testing.T, expErr error) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				decodeTimeoutStr := time.Second.String()
				fallback := true
				s := getter.RACE

				ctx, err := getter.SetConfigInContext(ctx, &s, &fallback, &decodeTimeoutStr, log.Noop)
				if err != nil {
					t.Fatal(err)
				}

				joinReader, rootSpan, err := joiner.New(ctx, store, store, swarmAddr)
				if err != nil {
					t.Fatal(err)
				}
				// sanity checks
				expectedRootSpan := chunkSize*(shardCnt-1) + 5
				if int64(expectedRootSpan) != rootSpan {
					t.Fatalf("Expected root span %d. Got: %d", expectedRootSpan, rootSpan)
				}
				i := 0
				eg, ectx := errgroup.WithContext(ctx)

			scnt:
				for ; i < shardCnt; i++ {
					select {
					case <-ectx.Done():
						break scnt
					default:
					}
					i := i
					eg.Go(func() error {
						chunkData := make([]byte, chunkSize)
						n, err := joinReader.ReadAt(chunkData, int64(i*chunkSize))
						if err != nil {
							return err
						}
						select {
						case <-ectx.Done():
							return ectx.Err()
						default:
						}
						expectedChunkData := dataChunks[i].Data()[swarm.SpanSize:]
						if !bytes.Equal(expectedChunkData, chunkData[:n]) {
							return fmt.Errorf("data mismatch on chunk position %d", i)
						}
						return nil
					})
				}
				err = eg.Wait()

				if !errors.Is(err, expErr) {
					t.Fatalf("unexpected error reading chunkdata at chunk position %d: expected %v. got %v", i, expErr, err)
				}
			}
			t.Run("no recovery possible with no chunk stored", func(t *testing.T) {
				readCheck(t, storage.ErrNotFound)
			})

			if err := putter.store(shardCnt - 1); err != nil {
				t.Fatal(err)
			}
			t.Run("no recovery possible with 1 short of shardCnt chunks stored", func(t *testing.T) {
				readCheck(t, storage.ErrNotFound)
			})

			if err := putter.store(1); err != nil {
				t.Fatal(err)
			}
			t.Run("recovery given shardCnt chunks stored", func(t *testing.T) {
				readCheck(t, nil)
			})

			if err := putter.store(shardCnt + parityCnt); err != nil {
				t.Fatal(err)
			}
			t.Run("success given shardCnt data chunks stored, no need for recovery", func(t *testing.T) {
				readCheck(t, nil)
			})
			// success after rootChunk deleted using replicas given shardCnt data chunks stored, no need for recovery
			if err := store.Delete(ctx, swarm.NewAddress(swarmAddr.Bytes()[:swarm.HashSize])); err != nil {
				t.Fatal(err)
			}
			t.Run("recover from replica if root deleted", func(t *testing.T) {
				readCheck(t, nil)
			})
		})
	}
}

// TestJoinerRedundancyMultilevel tests the joiner with all combinations of
// redundancy level, encryption and size (levels, i.e., the	height of the swarm hash tree).
//
// The test cases have the following structure:
//
//  1. upload a file with a given redundancy level and encryption
//
//  2. [positive test] download the file by the reference returned by the upload API response
//     This uses range queries to target specific (number of) chunks of the file structure
//     During path traversal in the swarm hash tree, the underlying mocksore (forgetting)
//     is in 'recording' mode, flagging all the retrieved chunks as chunks to forget.
//     This is to simulate the scenario where some of the chunks are not available/lost
//
//  3. [negative test] attempt at downloading the file using once again the same root hash
//     and a no-redundancy strategy to find the file inaccessible after forgetting.
//     3a. [negative test] download file using NONE without fallback and fail
//     3b. [negative test] download file using DATA without fallback and fail
//
//  4. [positive test] download file using DATA with fallback to allow for
//     reconstruction via erasure coding and succeed.
//
//  5. [positive test] after recovery chunks are saved, so forgetting no longer
//     repeat  3a/3b but this time succeed
//
// nolint:thelper
func TestJoinerRedundancyMultilevel(t *testing.T) {
	t.Parallel()
	test := func(t *testing.T, rLevel redundancy.Level, encrypt bool, size int) {
		t.Helper()
		store := mockstorer.NewForgettingStore(newChunkStore())
		seed, err := pseudorand.NewSeed()
		if err != nil {
			t.Fatal(err)
		}
		dataReader := pseudorand.NewReader(seed, size*swarm.ChunkSize)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// ctx = redundancy.SetLevelInContext(ctx, rLevel)
		ctx = redundancy.SetLevelInContext(ctx, redundancy.NONE)
		pipe := builder.NewPipelineBuilder(ctx, store, encrypt, rLevel)
		addr, err := builder.FeedPipeline(ctx, pipe, dataReader)
		if err != nil {
			t.Fatal(err)
		}
		expRead := swarm.ChunkSize
		buf := make([]byte, expRead)
		offset := mrand.Intn(size) * expRead
		canReadRange := func(t *testing.T, s getter.Strategy, fallback bool, canRead bool) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			decodingTimeoutStr := time.Second.String()

			ctx, err := getter.SetConfigInContext(ctx, &s, &fallback, &decodingTimeoutStr, log.Noop)
			if err != nil {
				t.Fatal(err)
			}

			j, _, err := joiner.New(ctx, store, store, addr)
			if err != nil {
				t.Fatal(err)
			}
			n, err := j.ReadAt(buf, int64(offset))
			if !canRead {
				if !errors.Is(err, storage.ErrNotFound) && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected error %v or %v. got %v", storage.ErrNotFound, context.DeadlineExceeded, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if n != expRead {
				t.Errorf("read %d bytes out of %d", n, expRead)
			}
			_, err = dataReader.Seek(int64(offset), io.SeekStart)
			if err != nil {
				t.Fatal(err)
			}
			ok, err := dataReader.Match(bytes.NewBuffer(buf), expRead)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Error("content mismatch")
			}
		}

		// first sanity check and recover a range
		t.Run("NONE w/o fallback CAN retrieve", func(t *testing.T) {
			store.Record()
			defer store.Unrecord()
			canReadRange(t, getter.NONE, false, true)
		})

		// do not forget the root chunk
		store.Unmiss(swarm.NewAddress(addr.Bytes()[:swarm.HashSize]))
		// after we forget the chunks on the way to the range, we should not be able to retrieve
		t.Run("NONE w/o fallback CANNOT retrieve", func(t *testing.T) {
			canReadRange(t, getter.NONE, false, false)
		})

		// we lost a data chunk, we cannot recover using DATA only strategy with no fallback
		t.Run("DATA w/o fallback CANNOT retrieve", func(t *testing.T) {
			canReadRange(t, getter.DATA, false, false)
		})

		if rLevel == 0 {
			// allowing fallback mode will not help if no redundancy used for upload
			t.Run("DATA w fallback CANNOT retrieve", func(t *testing.T) {
				canReadRange(t, getter.DATA, true, false)
			})
			return
		}
		// allowing fallback mode will make the range retrievable using erasure decoding
		t.Run("DATA w fallback CAN retrieve", func(t *testing.T) {
			canReadRange(t, getter.DATA, true, true)
		})
		// after the reconstructed data is stored, we can retrieve the range using DATA only mode
		t.Run("after recovery, NONE w/o fallback CAN retrieve", func(t *testing.T) {
			canReadRange(t, getter.NONE, false, true)
		})
	}
	r2level := []int{2, 1, 2, 3, 2}
	encryptChunk := []bool{false, false, true, true, true}
	for _, rLevel := range []redundancy.Level{0, 1, 2, 3, 4} {
		// speeding up tests by skipping some of them
		t.Run(fmt.Sprintf("rLevel=%v", rLevel), func(t *testing.T) {
			t.Parallel()
			for _, encrypt := range []bool{false, true} {
				shardCnt := rLevel.GetMaxShards()
				if encrypt {
					shardCnt = rLevel.GetMaxEncShards()
				}
				for _, levels := range []int{1, 2, 3} {
					chunkCnt := 1
					switch levels {
					case 1:
						chunkCnt = 2
					case 2:
						chunkCnt = shardCnt + 1
					case 3:
						chunkCnt = shardCnt*shardCnt + 1
					}
					t.Run(fmt.Sprintf("encrypt=%v levels=%d chunks=%d incomplete", encrypt, levels, chunkCnt), func(t *testing.T) {
						if r2level[rLevel] != levels || encrypt != encryptChunk[rLevel] {
							t.Skip("skipping to save time")
						}
						test(t, rLevel, encrypt, chunkCnt)
					})
					switch levels {
					case 1:
						chunkCnt = shardCnt
					case 2:
						chunkCnt = shardCnt * shardCnt
					case 3:
						continue
					}
					t.Run(fmt.Sprintf("encrypt=%v levels=%d chunks=%d full", encrypt, levels, chunkCnt), func(t *testing.T) {
						test(t, rLevel, encrypt, chunkCnt)
					})
				}
			}
		})
	}
}

type chunkStore struct {
	mu     sync.Mutex
	chunks map[string]swarm.Chunk
}

func newChunkStore() *chunkStore {
	return &chunkStore{
		chunks: make(map[string]swarm.Chunk),
	}
}

func (c *chunkStore) Get(_ context.Context, addr swarm.Address) (swarm.Chunk, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	chunk, ok := c.chunks[addr.ByteString()]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return chunk, nil
}

func (c *chunkStore) Put(_ context.Context, ch swarm.Chunk) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chunks[ch.Address().ByteString()] = swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(ch.Stamp())
	return nil
}

func (c *chunkStore) Replace(_ context.Context, ch swarm.Chunk, emplace bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chunks[ch.Address().ByteString()] = swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(ch.Stamp())
	return nil
}

func (c *chunkStore) Has(_ context.Context, addr swarm.Address) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.chunks[addr.ByteString()]

	return exists, nil
}

func (c *chunkStore) Delete(_ context.Context, addr swarm.Address) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.chunks, addr.ByteString())
	return nil
}

func (c *chunkStore) Iterate(_ context.Context, fn storage.IterateChunkFn) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, c := range c.chunks {
		stop, err := fn(c)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}

	return nil
}

func (c *chunkStore) Close() error {
	return nil
}
