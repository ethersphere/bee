package internal_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/joiner/internal"
	filetest "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestSimpleJoinerJobBlocksize checks that only Read() calls with exact
// chunk size buffer capacity is allowed.
func TestSimpleJoinerJobBlocksize(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

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
	b := make([]byte, swarm.SectionSize)
	_, err = j.Read(b)
	if err == nil {
		t.Fatal("expected error on Read with too small buffer")
	}

	b = make([]byte, swarm.ChunkSize+swarm.SectionSize)
	_, err = j.Read(b)
	if err == nil {
		t.Fatal("expected error on Read with too big buffer")
	}

	b = make([]byte, swarm.ChunkSize)
	_, err = j.Read(b)
	if err != nil {
		t.Fatal(err)
	}
	b = make([]byte, swarm.ChunkSize)
}

// TestSimpleJoinerJobOneLevel tests the retrieval of data chunks immediately
// below the root chunk level.
func TestSimpleJoinerJobOneLevel(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

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

	outBuffer := make([]byte, 4096) // arbitrary non power of 2 number

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

	_, err = j.Read(outBuffer)
	if err != io.EOF {
		t.Fatalf("expected io.EOF")
	}

	_, err = j.Read(outBuffer)
	if err != io.EOF {
		t.Fatalf("expected io.EOF")
	}
}

func TestSimpleJoinerJobTwoLevelsAcrossChunk(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

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
