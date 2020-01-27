package mock_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/janos/bee/pkg/storage"
	"github.com/janos/bee/pkg/storage/mock"
)

func TestMockStorer(t *testing.T) {

	s := mock.NewStorer()

	keyFound := []byte("exists")
	keyNotFound := []byte("not found")

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
