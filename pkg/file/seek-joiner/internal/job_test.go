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

	"github.com/ethersphere/bee/pkg/file/seek-joiner/internal"
	"github.com/ethersphere/bee/pkg/file/splitter"
	filetest "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSeek(t *testing.T) {
	seed := time.Now().UnixNano()
	t.Log("seed", seed)

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
			size: 4096,
		},
		{
			name: "a few chunks",
			size: 10 * 4096,
		},
		{
			name: "a few chunks and a change",
			size: 10*4096 + 84,
		},
		{
			name: "a few chunks more",
			size: 2*4096*4096 + 1000,
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

			s := splitter.NewSimpleSplitter(store)
			addr, err := s.Split(ctx, ioutil.NopCloser(bytes.NewReader(data)), tc.size, false)
			if err != nil {
				t.Fatal(err)
			}
			rootChunk, err := store.Get(ctx, storage.ModeGetLookup, addr)
			if err != nil {
				t.Fatal(err)
			}

			j := internal.NewSimpleJoinerJob(ctx, store, rootChunk)
			defer j.Close()

			validateRead := func(t *testing.T, name string, i int) {
				t.Helper()

				got := make([]byte, 4096)
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
					t.Errorf("read on seek to %v from %v: got data %x, want %s", name, i, got, want)
				}
			}

			// seek to all possible locations from start
			for i := int64(0); i < tc.size; i++ {
				n, err := j.Seek(i, io.SeekStart)
				if err != nil {
					t.Fatal(err)
				}
				if n != i {
					t.Errorf("seek to %v from start, want %v", n, i)
				}

				validateRead(t, "start", int(n))
			}
			if _, err := j.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}

			// seek to all possible locations from current position
			for i := int64(1); i < tc.size; i++ {
				n, err := j.Seek(1, io.SeekCurrent)
				if err != nil {
					t.Fatal(err)
				}
				if n != i {
					t.Errorf("seek to %v from current, want %v", n, i)
				}

				validateRead(t, "current", int(n))
			}
			if _, err := j.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}

			// seek to all possible locations from end
			for i := int64(1); i < tc.size; i++ {
				n, err := j.Seek(i, io.SeekEnd)
				if err != nil {
					t.Fatal(err)
				}
				want := tc.size - i
				if n != want {
					t.Errorf("seek to %v from end, want %v", n, want)
				}

				validateRead(t, "end", int(n))
			}
			if _, err := j.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}

			// seek overflow for a few bytes
			for i := int64(0); i < 5; i++ {
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
	t.Log("root", rootChunk.Address().String())

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, firstChunk)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("first", firstChunk.Address().String())

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, secondChunk)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("second", secondChunk.Address().String())

	j := internal.NewSimpleJoinerJob(ctx, store, rootChunk)

	b := make([]byte, swarm.ChunkSize)
	_, err = j.ReadAt(b, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, secondChunk.Data()[8:]) {
		t.Fatal("not equal")
	}
}

// TestSimpleJoinerJobBlocksize checks that only Read() calls with exact
// chunk size buffer capacity is allowed.
func TestSimpleJoinerJobBlocksize(t *testing.T) {
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

	// this buffer is too small
	j := internal.NewSimpleJoinerJob(ctx, store, rootChunk)
	b := make([]byte, swarm.SectionSize)
	_, err = j.Read(b)
	if err == nil {
		t.Fatal("expected error on Read with too small buffer")
	}

	// this buffer is too big
	b = make([]byte, swarm.ChunkSize+swarm.SectionSize)
	_, err = j.Read(b)
	if err == nil {
		t.Fatal("expected error on Read with too big buffer")
	}

	// this buffer is juuuuuust right
	b = make([]byte, swarm.ChunkSize)
	_, err = j.Read(b)
	if err != nil {
		t.Fatal(err)
	}
}

// TestSimpleJoinerJobOneLevel tests the retrieval of two data chunks immediately
// below the root chunk level.
func TestSimpleJoinerJobOneLevel(t *testing.T) {
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

	j := internal.NewSimpleJoinerJob(ctx, store, rootChunk)

	// verify first chunk content
	outBuffer := make([]byte, 4096)
	c, err := j.Read(outBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if c != 4096 {
		t.Fatalf("expected firstchunk read count %d, got %d", 4096, c)
	}
	if !bytes.Equal(outBuffer, firstChunk.Data()[8:]) {
		t.Fatalf("firstchunk data mismatch, expected %x, got %x", outBuffer, firstChunk.Data()[8:])
	}

	// verify second chunk content
	c, err = j.Read(outBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if c != 4096 {
		t.Fatalf("expected secondchunk read count %d, got %d", 4096, c)
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

// TestSimpleJoinerJobTwoLevelsAcrossChunk tests the retrieval of data chunks below
// first intermediate level across two intermediate chunks.
// Last chunk has sub-chunk length.
func TestSimpleJoinerJobTwoLevelsAcrossChunk(t *testing.T) {
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

	j := internal.NewSimpleJoinerJob(ctx, store, rootChunk)

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
