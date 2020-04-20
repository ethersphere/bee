package internal_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/joiner/internal"
	filetest "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)


func TestSimpleJoinerJob(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)

	firstAddress := swarm.NewAddress(rootChunk.Data()[:swarm.SectionSize])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	store.Put(ctx, storage.ModePutUpload, firstChunk)

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	store.Put(ctx, storage.ModePutUpload, secondChunk)

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
