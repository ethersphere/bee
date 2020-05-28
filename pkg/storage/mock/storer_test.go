package mock_test

import (
	"bytes"
	"context"
	"github.com/ethersphere/bee/pkg/tags"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestMockStorer(t *testing.T) {
	s := mock.NewStorer()

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
	if _, err := s.Get(ctx, storage.ModeGetRequest, keyFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := s.Get(ctx, storage.ModeGetRequest, keyNotFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := s.Put(ctx, storage.ModePutUpload, swarm.NewChunk(keyFound, valueFound)); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if chunk, err := s.Get(ctx, storage.ModeGetRequest, keyFound); err != nil {
		t.Fatalf("expected no error but got: %v", err)
	} else {
		if !bytes.Equal(chunk.Data(), valueFound) {
			t.Fatalf("expected value %s but got %s", valueFound, chunk.Data())
		}
	}
	has, err := s.Has(ctx, keyFound)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected mock store to have key")
	}
}

func TestMockValidatingStorer(t *testing.T) {
	validAddressHex := "aabbcc"
	invalidAddressHex := "bbccdd"

	validAddress := swarm.MustParseHexAddress(validAddressHex)
	invalidAddress := swarm.MustParseHexAddress(invalidAddressHex)

	validContent := []byte("bbaatt")
	invalidContent := []byte("bbaattss")

	s := mock.NewValidatingStorer(validator.NewMockValidator(validAddress, validContent), tags.NewTags())

	ctx := context.Background()

	if _, err := s.Put(ctx, storage.ModePutUpload, swarm.NewChunk(validAddress, validContent)); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if _, err := s.Put(ctx, storage.ModePutUpload, swarm.NewChunk(invalidAddress, validContent)); err == nil {
		t.Fatalf("expected error but got none")
	}

	if _, err := s.Put(ctx, storage.ModePutUpload, swarm.NewChunk(invalidAddress, invalidContent)); err == nil {
		t.Fatalf("expected error but got none")
	}

	if chunk, err := s.Get(ctx, storage.ModeGetRequest, validAddress); err != nil {
		t.Fatalf("got error on get but expected none: %v", err)
	} else {
		if !bytes.Equal(chunk.Data(), validContent) {
			t.Fatal("stored content not identical to input data")
		}
	}

	if _, err := s.Get(ctx, storage.ModeGetRequest, invalidAddress); err == nil {
		t.Fatal("got no error on get but expected one")
	}

}
