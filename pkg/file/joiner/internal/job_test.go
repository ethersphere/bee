package internal_test

import (
	"context"
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

func TestSimpleJoinerJobRead(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	logger := logging.New(os.Stderr, 5)
	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)
	store.Put(ctx, storage.ModePutUpload, rootChunk)
	logger.Debugf("put rootchunk %v", rootChunk)

	firstAddress := swarm.NewAddress(rootChunk.Data()[8:swarm.SectionSize+8])
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

	outBuffer := make([]byte, 42) // arbitrary non power of 2 number

	c, err := j.Read(outBuffer)
	if err != nil {
		t.Fatal(err)
	}
	if c != 42 {
		t.Fatalf("expected read count %d, got %d", 42, c)
	}
}
