package internal_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/file/splitter/internal"
)

func TestSplitterJobPartialSingleChunk(t *testing.T) {
	store := mock.NewStorer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := []byte("foo")
	j := internal.NewSimpleSplitterJob(ctx, store, int64(len(data)))

	c, err := j.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if c < len(data) {
		t.Fatalf("short write %d", c)
	}

	hashResult := j.Sum(nil)
	addressResult := swarm.NewAddress(hashResult)

	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)
	if addressResult.Equal(address) {
		t.Fatalf("expected %v, got %v", address, addressResult)
	}
}
