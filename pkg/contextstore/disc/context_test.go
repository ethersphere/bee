package disc_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/contextstore/disc"
	"github.com/ethersphere/bee/pkg/storage"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestDiscContext(t *testing.T) {
	retC := make(chan swarm.Address)
	cb := func(addr swarm.Address) {
		retC <- addr
	}
	d, c := disc.New(&r{f: cb})

	addr := swarm.MustParseHexAddress("001122")
	go func() {
		_, err := d.Get(context.Background(), addr)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("expected err not found but got %v", err)
		}
	}()
	if addr2 := <-retC; !addr2.Equal(addr) {
		t.Fatalf("got address %s want %s", addr2.String(), addr.String())
	}
	var ch swarm.Chunk
	go func() {
		ch = testingc.GenerateTestRandomChunk()
		_, err := d.Put(context.Background(), ch)
		if err != nil {
			t.Error(err)
		}
	}()

	op := <-c
	if !op.Chunk.Address().Equal(ch.Address()) {
		t.Fatalf("got address %s want %s", op.Chunk.Address().String(), ch.Address().String())
	}

	if !bytes.Equal(op.Chunk.Data(), ch.Data()) {
		t.Fatalf("data mismatch")
	}
}

type r struct {
	f func(swarm.Address)
}

func (r *r) RetrieveChunk(ctx context.Context, addr swarm.Address, origin bool) (chunk swarm.Chunk, err error) {
	r.f(addr)
	return nil, storage.ErrNotFound
}
