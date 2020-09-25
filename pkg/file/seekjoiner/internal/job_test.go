// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/seekjoiner/internal"
	"github.com/ethersphere/bee/pkg/file/splitter"
	filetest "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSeek(t *testing.T) {
	seed := time.Now().UnixNano()

	r := mrand.New(mrand.NewSource(seed))

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
			ctx := context.Background()

			store := mock.NewStorer()
			defer store.Close()

			data, err := ioutil.ReadAll(io.LimitReader(r, tc.size))
			if err != nil {
				t.Fatal(err)
			}

			s := splitter.NewSimpleSplitter(store, storage.ModePutUpload)
			addr, err := s.Split(ctx, ioutil.NopCloser(bytes.NewReader(data)), tc.size, false)
			if err != nil {
				t.Fatal(err)
			}

			j, _, err := internal.NewSimpleJoiner(ctx, store, addr)
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
				if err != io.EOF {
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
	seed := time.Now().UnixNano()

	r := mrand.New(mrand.NewSource(seed))

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
			ctx := context.Background()

			store := mock.NewStorer()
			defer store.Close()

			data, err := ioutil.ReadAll(io.LimitReader(r, tc.size))
			if err != nil {
				t.Fatal(err)
			}

			s := splitter.NewSimpleSplitter(store, storage.ModePutUpload)
			addr, err := s.Split(ctx, ioutil.NopCloser(bytes.NewReader(data)), tc.size, false)
			if err != nil {
				t.Fatal(err)
			}

			j, _, err := internal.NewSimpleJoiner(ctx, store, addr)
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

// TestSimpleJoinerReadAt
func TestSimpleJoinerReadAt(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create root chunk with 2 references and the referenced data chunks
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	_, err := store.Put(ctx, storage.ModePutUpload, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	j, _, err := internal.NewSimpleJoiner(ctx, store, rootChunk.Address())
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

// TestSimpleJoinerOneLevel tests the retrieval of two data chunks immediately
// below the root chunk level.
func TestSimpleJoinerOneLevel(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create root chunk with 2 references and the referenced data chunks
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	_, err := store.Put(ctx, storage.ModePutUpload, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	j, _, err := internal.NewSimpleJoiner(ctx, store, rootChunk.Address())
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
	if err != io.EOF {
		t.Fatal("expected io.EOF")
	}

	_, err = j.Read(outBuffer)
	if err != io.EOF {
		t.Fatal("expected io.EOF")
	}
}

// TestSimpleJoinerTwoLevelsAcrossChunk tests the retrieval of data chunks below
// first intermediate level across two intermediate chunks.
// Last chunk has sub-chunk length.
func TestSimpleJoinerTwoLevelsAcrossChunk(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// create root chunk with 2 references and two intermediate chunks with references
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*swarm.Branches+42, swarm.SectionSize*2)
	_, err := store.Put(ctx, storage.ModePutUpload, rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize*swarm.Branches, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, 42, swarm.SectionSize)
	_, err = store.Put(ctx, storage.ModePutUpload, secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	// create 128+1 chunks for all references in the intermediate chunks
	cursor := 8
	for i := 0; i < swarm.Branches; i++ {
		chunkAddressBytes := firstChunk.Data()[cursor : cursor+swarm.SectionSize]
		chunkAddress := swarm.NewAddress(chunkAddressBytes)
		ch := filetest.GenerateTestRandomFileChunk(chunkAddress, swarm.ChunkSize, swarm.ChunkSize)
		_, err := store.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		cursor += swarm.SectionSize
	}
	chunkAddressBytes := secondChunk.Data()[8:]
	chunkAddress := swarm.NewAddress(chunkAddressBytes)
	ch := filetest.GenerateTestRandomFileChunk(chunkAddress, 42, 42)
	_, err = store.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	j, _, err := internal.NewSimpleJoiner(ctx, store, rootChunk.Address())
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
