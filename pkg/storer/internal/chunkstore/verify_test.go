// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestVerify(t *testing.T) {
	t.Parallel()

	cac := chunktest.GenerateTestRandomChunk()
	soc := chunktest.GenerateTestRandomSoChunk(t, chunktest.GenerateTestRandomChunk())
	errLoad := errors.New("read failed")

	tcs := []struct {
		name     string
		chunk    swarm.Chunk
		loadErr  error
		wantType swarm.ChunkType
		wantErr  error
	}{
		{"content addressed", cac, nil, swarm.ChunkTypeContentAddressed, nil},
		{"single owner", soc, nil, swarm.ChunkTypeSingleOwner, nil},
		{"misaddressed single owner", swarm.NewChunk(swarm.RandAddress(t), soc.Data()), nil, swarm.ChunkTypeUnspecified, storage.ErrInvalidChunk},
		{"invalid data", chunktest.GenerateTestRandomInvalidChunk(), nil, swarm.ChunkTypeUnspecified, storage.ErrInvalidChunk},
		{"load error", cac, errLoad, swarm.ChunkTypeUnspecified, errLoad},
		{"not found", nil, storage.ErrNotFound, swarm.ChunkTypeUnspecified, storage.ErrNotFound},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			chunkType, err := chunkstore.Verify(tc.chunk, tc.loadErr)
			if chunkType != tc.wantType {
				t.Fatalf("chunk type: want %v, got %v", tc.wantType, chunkType)
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, chunkstore.ErrCorrupted) || !errors.Is(err, tc.wantErr) {
				t.Fatalf("want error wrapping %v and %v, got %v", chunkstore.ErrCorrupted, tc.wantErr, err)
			}
		})
	}
}
