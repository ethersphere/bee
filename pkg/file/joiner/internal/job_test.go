package internal_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/joiner/internal"
	filetest "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/logging"
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
	store.Put(ctx, storage.ModePutUpload, rootChunk)

	j := internal.NewSimpleJoinerJob(ctx, store, rootChunk)
	b := make([]byte, swarm.SectionSize)
	_, err := j.Read(b)
	if err == nil {
		t.Fatal("expected error on Read with too small buffer")
	}

	b = make([]byte, swarm.ChunkSize+swarm.SectionSize)
	_, err = j.Read(b)
	if err == nil {
		t.Fatal("expected error on Read with too big buffer")
	}

}

// TestSimpleJoinerJobOneLevel tests the retrieval of data chunks immediately
// below the root chunk level.
func TestSimpleJoinerJobOneLevel(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	logger := logging.New(os.Stderr, 5)
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	store.Put(ctx, storage.ModePutUpload, rootChunk)
	logger.Debugf("put rootchunk %v", rootChunk)

	firstAddress := swarm.NewAddress(rootChunk.Data()[8 : swarm.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err := store.Put(ctx, storage.ModePutUpload, firstChunk)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debugf("put firstchunk %v", firstChunk)

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	_, err = store.Put(ctx, storage.ModePutUpload, secondChunk)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debugf("put secondtchunk %v", secondChunk)

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

	c, err = j.Read(outBuffer)
	if err != io.EOF {
		t.Fatalf("expected io.EOF")
	}

	c, err = j.Read(outBuffer)
	if err != io.EOF {
		t.Fatalf("expected io.EOF")
	}
}
