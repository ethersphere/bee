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

	firstChunk := filetest.GenerateTestRandomFileChunk(swarm.ChunkSize*2)
	store.Put(ctx, storage.ModePutUpload, firstChunk)

	j := internal.NewSimpleJoinerJob(ctx, store, 4096, []byte{})
	_ = j
}
