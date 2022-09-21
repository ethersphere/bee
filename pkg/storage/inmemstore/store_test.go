// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestStorePutGet(t *testing.T) {
	t.Parallel()

	s := inmemstore.New()

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
	if _, err := s.Get(ctx, storage.ModeGetRequest, keyFound); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := s.Get(ctx, storage.ModeGetRequest, keyNotFound); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := s.Put(ctx, storage.ModePutUpload, swarm.NewChunk(keyFound, valueFound)); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if chunk, err := s.Get(ctx, storage.ModeGetRequest, keyFound); err != nil {
		t.Fatalf("expected no error but got: %v", err)
	} else if !bytes.Equal(chunk.Data(), valueFound) {
		t.Fatalf("expected value %s but got %s", valueFound, chunk.Data())
	}
}
