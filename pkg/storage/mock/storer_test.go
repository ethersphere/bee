package mock_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
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
	if _, err := s.Get(ctx, keyFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := s.Get(ctx, keyNotFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := s.Put(ctx, keyFound, valueFound); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if data, err := s.Get(ctx, keyFound); err != nil {
		t.Fatalf("expected not error but got: %v", err)

	} else {
		if !bytes.Equal(data, valueFound) {
			t.Fatalf("expected value %s but got %s", valueFound, data)
		}
	}
}

func TestMockValidatingStorer(t *testing.T) {
	validAddr := "aabbcc"
	invalidAddr := "bbccdd"

	keyValid, err := swarm.ParseHexAddress(validAddr)
	if err != nil {
		t.Fatal(err)
	}
	keyInvalid, err := swarm.ParseHexAddress(invalidAddr)
	if err != nil {
		t.Fatal(err)
	}

	validContent := []byte("bbaatt")
	invalidContent := []byte("bbaattss")

	validatorF := func(addr swarm.Address, data []byte) bool {
		if !addr.Equal(keyValid) {
			return false

		}
		if !bytes.Equal(data, validContent) {
			return false
		}
		return true
	}

	s := mock.NewValidatingStorer(validatorF)

	ctx := context.Background()

	if err := s.Put(ctx, keyValid, validContent); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if err := s.Put(ctx, keyInvalid, validContent); err == nil {
		t.Fatalf("expected error but got none")
	}

	if err := s.Put(ctx, keyInvalid, invalidContent); err == nil {
		t.Fatalf("expected error but got none")
	}

	if data, err := s.Get(ctx, keyValid); err != nil {
		t.Fatalf("got error on get but expected none: %v", err)
	} else {
		if !bytes.Equal(data, validContent) {
			t.Fatal("stored content not identical to input data")
		}
	}

	if _, err := s.Get(ctx, keyInvalid); err == nil {
		t.Fatal("got no error on get but expected one")
	}

}
