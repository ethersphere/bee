package mem_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	memstore "github.com/ethersphere/bee/pkg/storage/mem"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestMockStorer(t *testing.T) {
	s, err := memstore.NewMemStorer()
	if err != nil {
		t.Fatal(err)
	}
	keyFound, err := swarm.ParseHexAddress("aabbcc")
	if err != nil {
		t.Fatal(err)
	}
	keyNotFound, err := swarm.ParseHexAddress("bbccdd")
	if err != nil {
		t.Fatal(err)
	}

	valueFound := []byte("data data data")

	ctx := context.Background()
	if _, err := s.Get(ctx, keyFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := s.Get(ctx, keyNotFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	chunk := swarm.NewChunk(keyFound, valueFound)
	if err := s.Put(ctx, chunk); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if gotChunk, err := s.Get(ctx, keyFound); err != nil {
		t.Fatalf("expected not error but got: %v", err)

	} else {
		if !bytes.Equal(chunk.Data(), valueFound) {
			t.Fatalf("expected value %s but got %s", valueFound, gotChunk.Data())
		}
	}

	if yes, _ := s.Has(ctx, keyNotFound); yes {
		t.Fatalf("expected false but got true")
	}

	if yes, _ := s.Has(ctx, keyFound); !yes {
		t.Fatalf("expected true but got false")
	}
}
