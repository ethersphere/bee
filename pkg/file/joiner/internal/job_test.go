package internal_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/file/joiner/internal"
	filetest "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)


func TestSimpleJoinerJob(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootChunk := filetest.GenerateTestRandomFileChunk(swarm.ZeroAddress, swarm.ChunkSize*2, swarm.SectionSize*2)

	firstAddress := swarm.NewAddress(rootChunk.Data()[:swarm.SectionSize])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, swarm.ChunkSize, swarm.ChunkSize)
	store.Put(ctx, storage.ModePutUpload, firstChunk)

	secondAddress := swarm.NewAddress(rootChunk.Data()[swarm.SectionSize:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, swarm.ChunkSize, swarm.ChunkSize)
	store.Put(ctx, storage.ModePutUpload, secondChunk)

	j := internal.NewSimpleJoinerJob(ctx, store, swarm.ChunkSize*2, rootChunk.Data()[8:])
	_ = j
}
