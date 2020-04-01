package mem_test

import (
	"bytes"
	"context"
	"math/rand"
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

	// Check if a non existing key is found or not.
	if yes, _ := s.Has(ctx, keyNotFound); yes {
		t.Fatalf("expected false but got true")
	}

	// Check if an existing key is found.
	if yes, _ := s.Has(ctx, keyFound); !yes {
		t.Fatalf("expected true but got false")
	}

	// Try deleting a non existing key.
	if err := s.Delete(ctx, keyNotFound); err != nil{
		t.Fatalf("expected no error but got: %v", err)
	}

	// Delete a existing key.
	if err := s.Delete(ctx, keyFound); err != nil{
		t.Fatalf("expected no error but got: %v", err)
	}
}

func TestCountAndIterator(t *testing.T) {
	s, err := memstore.NewMemStorer()
	if err != nil {
		t.Fatal(err)
	}

	expectedMap := make(map[string][]byte)

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		ch := GenerateRandomChunk()
		expectedMap[ch.Address().String()] = ch.Data()
		err := s.Put(ctx, ch)
		if err != nil {
			t.Fatalf("%v", err)
		}
	}

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// check count match.
	if count != 100 {
		t.Fatalf("expected %v but got: %v", 100, count)
	}

	// check for iteration correctness.
	err = s.Iterate(func(chunk swarm.Chunk) (stop bool, err error) {
		data, ok := expectedMap[chunk.Address().ByteString()]
		if !ok {
			t.Fatalf("key %v not present", chunk.Address().ByteString())
		}

		if !bytes.Equal(data, chunk.Data()) {
			t.Fatalf("data mismatch for key %v", chunk.Address().ByteString())
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		t.Fatalf("error in iteration")
	}


}

func GenerateRandomChunk() swarm.Chunk {
	data := make([]byte, swarm.DefaultChunkSize)
	rand.Read(data)
	key := make([]byte, swarm.DefaultAddressLength)
	rand.Read(key)
	return swarm.NewChunk(swarm.NewAddress(key), data)
}